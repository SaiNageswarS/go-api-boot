package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/jackc/pgx/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Collection is a Postgres collection: the portable odm.Collection plus SQL
// for queries filters can't express.
type Collection[T odm.Model] interface {
	odm.Collection[T]

	// FindSql runs "SELECT doc FROM <tenant>.<collection> <clause>" and decodes
	// each row into T. The clause may hold WHERE, ORDER BY, LIMIT and OFFSET
	// over the columns id (text) and doc (jsonb, keyed by bson field names),
	// with $1, $2, ... bound to args:
	//
	//	FindSql(ctx, "WHERE (doc->>'year')::int >= $1 ORDER BY doc->'year' DESC LIMIT 10", 2020)
	//
	// The clause is executed as SQL: it must be trusted code, never built from
	// user input. Pass user values through args.
	FindSql(ctx context.Context, clause string, args ...any) <-chan async.Result[[]T]
}

var _ Collection[odm.Model] = (*collection[odm.Model])(nil)

type collection[T odm.Model] struct {
	db     *DB
	schema string
	table  string
}

// CollectionOf returns the collection of T in tenant (a Postgres schema).
func CollectionOf[T odm.Model](db *DB, tenant string) Collection[T] {
	var zero T
	return &collection[T]{db: db, schema: tenant, table: zero.CollectionName()}
}

func (c *collection[T]) qualified() string {
	return pgx.Identifier{c.schema, c.table}.Sanitize()
}

// Save upserts in one statement: model's top-level fields replace the stored
// ones, createdOn is set on insert, updatedOn on every save.
func (c *collection[T]) Save(ctx context.Context, model T) <-chan async.Result[struct{}] {
	return async.Go(func() (struct{}, error) {
		id := model.Id()
		if err := odm.CheckID(id); err != nil {
			return struct{}{}, err
		}
		doc, err := bsonToJSON(model)
		if err != nil {
			return struct{}{}, err
		}

		const fields = "($2::jsonb - 'createdOn' - 'updatedOn')"
		_, err = c.db.pool.Exec(ctx,
			"INSERT INTO "+c.qualified()+" AS t (id, doc) VALUES ($1, "+fields+
				" || jsonb_build_object('_id', $1::text, 'createdOn', $3::bigint, 'updatedOn', $3::bigint)) "+
				"ON CONFLICT (id) DO UPDATE SET doc = t.doc || "+fields+
				" || jsonb_build_object('_id', $1::text, 'updatedOn', $3::bigint)",
			id, doc, c.db.timer.Now())
		return struct{}{}, writeErr(err, c.schema, c.table)
	})
}

func (c *collection[T]) FindByID(ctx context.Context, id string) <-chan async.Result[*T] {
	return c.FindOne(ctx, odm.M{"_id": id})
}

func (c *collection[T]) FindOne(ctx context.Context, filter odm.M) <-chan async.Result[*T] {
	return async.Go(func() (*T, error) {
		q := &pgQuery{}
		where, err := q.where(filter)
		if err != nil {
			return nil, err
		}
		var doc []byte
		err = c.db.pool.QueryRow(ctx, "SELECT doc::text FROM "+c.qualified()+" WHERE "+where+" LIMIT 1", q.args...).Scan(&doc)
		if errors.Is(err, pgx.ErrNoRows) || isMissing(err) {
			return nil, odm.ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		return jsonToModel[T](doc)
	})
}

func (c *collection[T]) Find(ctx context.Context, filter odm.M, opts ...odm.FindOption) <-chan async.Result[[]T] {
	return async.Go(func() ([]T, error) {
		o, err := odm.ResolveFindOptions(opts...)
		if err != nil {
			return nil, err
		}
		q := &pgQuery{}
		where, err := q.where(filter)
		if err != nil {
			return nil, err
		}
		sql := "SELECT doc::text FROM " + c.qualified() + " WHERE " + where + orderBy(o.Sort) + limitOffset(o.Limit, o.Skip)
		return queryDocs[T](ctx, c.db.pool, sql, q.args)
	})
}

func (c *collection[T]) FindSql(ctx context.Context, clause string, args ...any) <-chan async.Result[[]T] {
	return async.Go(func() ([]T, error) {
		return queryDocs[T](ctx, c.db.pool, "SELECT doc::text FROM "+c.qualified()+" "+clause, args)
	})
}

func (c *collection[T]) Count(ctx context.Context, filter odm.M) <-chan async.Result[int64] {
	return async.Go(func() (int64, error) {
		q := &pgQuery{}
		where, err := q.where(filter)
		if err != nil {
			return 0, err
		}
		var n int64
		err = c.db.pool.QueryRow(ctx, "SELECT count(*) FROM "+c.qualified()+" WHERE "+where, q.args...).Scan(&n)
		if isMissing(err) {
			return 0, nil
		}
		return n, err
	})
}

func (c *collection[T]) Exists(ctx context.Context, id string) <-chan async.Result[bool] {
	return async.Go(func() (bool, error) {
		var ok bool
		err := c.db.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM "+c.qualified()+" WHERE id = $1)", id).Scan(&ok)
		if isMissing(err) {
			return false, nil
		}
		return ok, err
	})
}

// Distinct unwinds array values, like MongoDB.
func (c *collection[T]) Distinct(ctx context.Context, field string, filter odm.M) <-chan async.Result[[]any] {
	return async.Go(func() ([]any, error) {
		q := &pgQuery{}
		where, err := q.where(filter)
		if err != nil {
			return nil, err
		}
		p := jsonbExpr(strings.Split(field, "."))
		sql := fmt.Sprintf(
			"SELECT COALESCE(jsonb_agg(DISTINCT v)::text, '[]') FROM %s, "+
				"LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(%s) = 'array' THEN %s ELSE jsonb_build_array(%s) END) AS v "+
				"WHERE %s IS NOT NULL AND %s",
			c.qualified(), p, p, p, p, where)

		var values string
		err = c.db.pool.QueryRow(ctx, sql, q.args...).Scan(&values)
		if isMissing(err) {
			return []any{}, nil
		}
		if err != nil {
			return nil, err
		}
		var out struct {
			V []any `bson:"v"`
		}
		if err := bson.UnmarshalExtJSON([]byte(`{"v":`+values+`}`), false, &out); err != nil {
			return nil, err
		}
		return out.V, nil
	})
}

func (c *collection[T]) DeleteByID(ctx context.Context, id string) <-chan async.Result[struct{}] {
	return c.DeleteOne(ctx, odm.M{"_id": id})
}

func (c *collection[T]) DeleteOne(ctx context.Context, filter odm.M) <-chan async.Result[struct{}] {
	return async.Go(func() (struct{}, error) {
		if filter == nil {
			return struct{}{}, errors.New("odm/postgres: DeleteOne needs a filter")
		}
		q := &pgQuery{}
		where, err := q.where(filter)
		if err != nil {
			return struct{}{}, err
		}
		_, err = c.db.pool.Exec(ctx,
			"DELETE FROM "+c.qualified()+" WHERE id = (SELECT id FROM "+c.qualified()+" WHERE "+where+" LIMIT 1)",
			q.args...)
		if isMissing(err) {
			return struct{}{}, nil
		}
		return struct{}{}, err
	})
}

/* ── helpers ── */

func limitOffset(limit, skip int64) string {
	s := ""
	if limit > 0 {
		s += " LIMIT " + strconv.FormatInt(limit, 10)
	}
	if skip > 0 {
		s += " OFFSET " + strconv.FormatInt(skip, 10)
	}
	return s
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// queryDocs runs a query whose first column is a document; a missing table
// yields no documents.
func queryDocs[T odm.Model](ctx context.Context, db querier, sql string, args []any) ([]T, error) {
	hits, err := queryHits[T](ctx, db, sql, args, nil)
	if err != nil {
		return nil, err
	}
	docs := make([]T, len(hits))
	for i, h := range hits {
		docs[i] = h.Doc
	}
	return docs, nil
}

// queryHits runs a query returning (doc, score) rows, or only doc when score
// is nil. score maps the raw second column to a hit's score.
func queryHits[T odm.Model](ctx context.Context, db querier, sql string, args []any, score func(float64) float64) ([]odm.SearchHit[T], error) {
	rows, err := db.Query(ctx, sql, args...)
	if err == nil {
		defer rows.Close()
	}
	hits := []odm.SearchHit[T]{}
	for err == nil && rows.Next() {
		var (
			doc []byte
			raw float64
		)
		if score != nil {
			err = rows.Scan(&doc, &raw)
		} else {
			err = rows.Scan(&doc)
		}
		if err != nil {
			break
		}
		var model *T
		if model, err = jsonToModel[T](doc); err != nil {
			break
		}
		hit := odm.SearchHit[T]{Doc: *model}
		if score != nil {
			hit.Score = score(raw)
		}
		hits = append(hits, hit)
	}
	if err == nil {
		err = rows.Err()
	}
	if isMissing(err) {
		return []odm.SearchHit[T]{}, nil
	}
	return hits, err
}
