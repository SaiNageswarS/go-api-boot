package odm

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/jackc/pgx/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ErrNoDocuments is returned by FindOne/FindOneByID when nothing matches, by
// both the MongoDB and the Postgres collections.
var ErrNoDocuments = mongo.ErrNoDocuments

type pgCollection[T DbModel] struct {
	db     PgClient
	schema string
	table  string
	timer  Timer
}

// PgCollection is a Postgres collection: the portable OdmCollectionInterface,
// plus SQL for queries the bson filters can't express. Pass it anywhere an
// OdmCollectionInterface is expected; reach FindSql by holding a PgCollection
// or with a type assertion.
type PgCollection[T DbModel] interface {
	OdmCollectionInterface[T]

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

var _ PgCollection[DbModel] = (*pgCollection[DbModel])(nil)

// PgCollectionOf is CollectionOf for Postgres, backed by the table
// "<tenant>"."<CollectionName()>".
func PgCollectionOf[T DbModel](client PgClient, tenant string) PgCollection[T] {
	var zero T
	return &pgCollection[T]{
		db:     client,
		schema: tenant,
		table:  zero.CollectionName(),
		timer:  DefaultTimer{},
	}
}

func (c *pgCollection[T]) qualified() string {
	return pgx.Identifier{c.schema, c.table}.Sanitize()
}

func (c *pgCollection[T]) ensure(ctx context.Context) error {
	return ensurePgTable(ctx, c.db, c.schema, c.table)
}

// Save upserts the model under model.Id(), merging top-level fields into an
// existing document like MongoDB's $set, and stamps createdOn / updatedOn.
func (c *pgCollection[T]) Save(ctx context.Context, model T) <-chan async.Result[struct{}] {
	return async.Go(func() (struct{}, error) {
		if err := c.ensure(ctx); err != nil {
			return struct{}{}, err
		}

		doc, err := bsonToJSON(model)
		if err != nil {
			return struct{}{}, err
		}

		_, err = c.db.Exec(ctx,
			"INSERT INTO "+c.qualified()+" AS t (id, doc) "+
				"VALUES ($1, $2::jsonb || jsonb_build_object('_id', $1::text, 'createdOn', $3::bigint)) "+
				"ON CONFLICT (id) DO UPDATE SET doc = t.doc || $2::jsonb || jsonb_build_object('_id', $1::text, 'updatedOn', $3::bigint)",
			model.Id(), doc, c.timer.Now())
		return struct{}{}, err
	})
}

func (c *pgCollection[T]) FindOneByID(ctx context.Context, id string) <-chan async.Result[*T] {
	return c.FindOne(ctx, bson.M{"_id": id})
}

func (c *pgCollection[T]) FindOne(ctx context.Context, filters bson.M) <-chan async.Result[*T] {
	return async.Go(func() (*T, error) {
		if filters == nil {
			return nil, errors.New("filters cannot be nil for FindOne")
		}
		if err := c.ensure(ctx); err != nil {
			return nil, err
		}

		q := &pgQuery{}
		where, err := q.where(filters)
		if err != nil {
			return nil, err
		}

		var doc []byte
		err = c.db.QueryRow(ctx, "SELECT doc::text FROM "+c.qualified()+" WHERE "+where+" LIMIT 1", q.args...).Scan(&doc)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoDocuments
		}
		if err != nil {
			return nil, err
		}
		return jsonToModel[T](doc)
	})
}

func (c *pgCollection[T]) Find(ctx context.Context, filters bson.M, sort bson.D, limit, skip int64) <-chan async.Result[[]T] {
	return async.Go(func() ([]T, error) {
		if err := c.ensure(ctx); err != nil {
			return nil, err
		}

		q := &pgQuery{}
		where, err := q.where(filters)
		if err != nil {
			return nil, err
		}
		order, err := orderBy(sort)
		if err != nil {
			return nil, err
		}

		sql := "SELECT doc::text FROM " + c.qualified() + " WHERE " + where + order + limitOffset(limit, skip)
		return c.queryModels(ctx, sql, q.args)
	})
}

func (c *pgCollection[T]) FindSql(ctx context.Context, clause string, args ...any) <-chan async.Result[[]T] {
	return async.Go(func() ([]T, error) {
		if err := c.ensure(ctx); err != nil {
			return nil, err
		}
		return c.queryModels(ctx, "SELECT doc::text FROM "+c.qualified()+" "+clause, args)
	})
}

func (c *pgCollection[T]) DeleteByID(ctx context.Context, id string) <-chan async.Result[struct{}] {
	return c.DeleteOne(ctx, bson.M{"_id": id})
}

func (c *pgCollection[T]) DeleteOne(ctx context.Context, filters bson.M) <-chan async.Result[struct{}] {
	return async.Go(func() (struct{}, error) {
		if filters == nil {
			return struct{}{}, errors.New("filters cannot be nil for DeleteOne")
		}
		if err := c.ensure(ctx); err != nil {
			return struct{}{}, err
		}

		q := &pgQuery{}
		where, err := q.where(filters)
		if err != nil {
			return struct{}{}, err
		}

		_, err = c.db.Exec(ctx,
			"DELETE FROM "+c.qualified()+" WHERE id = (SELECT id FROM "+c.qualified()+" WHERE "+where+" LIMIT 1)",
			q.args...)
		return struct{}{}, err
	})
}

func (c *pgCollection[T]) Count(ctx context.Context, filters bson.M) <-chan async.Result[int64] {
	return async.Go(func() (int64, error) {
		if err := c.ensure(ctx); err != nil {
			return 0, err
		}

		q := &pgQuery{}
		where, err := q.where(filters)
		if err != nil {
			return 0, err
		}

		var count int64
		err = c.db.QueryRow(ctx, "SELECT count(*) FROM "+c.qualified()+" WHERE "+where, q.args...).Scan(&count)
		return count, err
	})
}

// DistinctInto decodes the distinct values of field into out (a pointer to a
// slice). Like MongoDB, array values contribute each of their elements.
func (c *pgCollection[T]) DistinctInto(ctx context.Context, field string, filters bson.D, out any) error {
	if out == nil {
		return errors.New("output slice cannot be nil")
	}
	if err := c.ensure(ctx); err != nil {
		return err
	}

	q := &pgQuery{}
	where, err := q.where(filters)
	if err != nil {
		return err
	}

	path := strings.Split(field, ".")
	p := jsonbExpr(path)
	sql := fmt.Sprintf(
		"SELECT COALESCE(jsonb_agg(DISTINCT v)::text, '[]') FROM %s, "+
			"LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(%s) = 'array' THEN %s ELSE jsonb_build_array(%s) END) AS v "+
			"WHERE %s IS NOT NULL AND %s",
		c.qualified(), p, p, p, p, where)

	var values string
	if err := c.db.QueryRow(ctx, sql, q.args...).Scan(&values); err != nil {
		return err
	}

	var wrapper struct {
		V bson.RawValue `bson:"v"`
	}
	if err := bson.UnmarshalExtJSON([]byte(`{"v":`+values+`}`), false, &wrapper); err != nil {
		return err
	}
	return wrapper.V.Unmarshal(out)
}

// Aggregate supports pipelines made of $match, $sort, $skip and $limit stages.
// Other stages reshape documents, which cannot be decoded back into T anyway.
func (c *pgCollection[T]) Aggregate(ctx context.Context, pipeline mongo.Pipeline) <-chan async.Result[[]T] {
	return async.Go(func() ([]T, error) {
		if err := c.ensure(ctx); err != nil {
			return nil, err
		}

		q := &pgQuery{}
		sql, err := c.pipelineSQL(q, pipeline)
		if err != nil {
			return nil, err
		}
		return c.queryModels(ctx, sql, q.args)
	})
}

func (c *pgCollection[T]) pipelineSQL(q *pgQuery, pipeline mongo.Pipeline) (string, error) {
	// One SELECT holds WHERE, ORDER BY, OFFSET, LIMIT, which SQL applies in that
	// order. A stage that would have to run after a later clause starts a new
	// SELECT over the previous one.
	type level struct {
		where       []string
		order       string
		limit, skip int64
	}
	from := c.qualified()
	cur := level{}
	render := func(l level) string {
		where := "TRUE"
		if len(l.where) > 0 {
			where = strings.Join(l.where, " AND ")
		}
		return "SELECT id, doc FROM " + from + " WHERE " + where + l.order + limitOffset(l.limit, l.skip)
	}
	wrap := func() {
		from = "(" + render(cur) + ") AS s"
		cur = level{order: cur.order}
	}

	for _, stage := range pipeline {
		if len(stage) != 1 {
			return "", errors.New("odm/postgres: each pipeline stage must have exactly one operator")
		}
		op, arg := stage[0].Key, stage[0].Value
		switch op {
		case "$match":
			if cur.limit > 0 || cur.skip > 0 {
				wrap()
			}
			clause, err := q.where(arg)
			if err != nil {
				return "", err
			}
			cur.where = append(cur.where, clause)
		case "$sort":
			if cur.limit > 0 || cur.skip > 0 {
				wrap()
			}
			order, err := orderBy(arg)
			if err != nil {
				return "", err
			}
			cur.order = order
		case "$skip":
			n, ok := toInt64(arg)
			if !ok || n < 0 {
				return "", errors.New("odm/postgres: $skip needs a non-negative number")
			}
			cur.skip += n
			if cur.limit > 0 {
				cur.limit -= n
				if cur.limit <= 0 {
					return "SELECT doc::text FROM " + c.qualified() + " WHERE FALSE", nil
				}
			}
		case "$limit":
			n, ok := toInt64(arg)
			if !ok || n <= 0 {
				return "", errors.New("odm/postgres: $limit needs a positive number")
			}
			if cur.limit == 0 || n < cur.limit {
				cur.limit = n
			}
		default:
			return "", errUnsupported("aggregation stage %s", op)
		}
	}

	// The outer ORDER BY keeps the final order; "doc" there resolves to r.doc.
	return "SELECT doc::text FROM (" + render(cur) + ") AS r" + cur.order, nil
}

func (c *pgCollection[T]) Exists(ctx context.Context, id string) <-chan async.Result[bool] {
	return async.Go(func() (bool, error) {
		if err := c.ensure(ctx); err != nil {
			return false, err
		}
		var exists bool
		err := c.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM "+c.qualified()+" WHERE id = $1)", id).Scan(&exists)
		return exists, err
	})
}

// VectorSearch runs a pgvector nearest-neighbour search. params.IndexName names
// one of the model's VectorIndexSpecs, which supplies the similarity (cosine by
// default). Scores follow Atlas: cosine and dotProduct map to (1 + similarity) / 2,
// euclidean to 1 / (1 + distance).
func (c *pgCollection[T]) VectorSearch(ctx context.Context, embedding []float32, params VectorSearchParams) <-chan async.Result[[]SearchHit[T]] {
	return async.Go(func() ([]SearchHit[T], error) {
		if len(embedding) == 0 || params.IndexName == "" || params.Path == "" || params.K <= 0 {
			return nil, errors.New("invalid input - embedding, index name, path, and K must be provided")
		}

		spec := findVectorSpec[T](params.IndexName)
		if spec != nil && spec.NumDimensions > 0 && spec.NumDimensions != len(embedding) {
			return nil, fmt.Errorf("odm/postgres: index %s has %d dimensions, query has %d", spec.Name, spec.NumDimensions, len(embedding))
		}
		vec := pgVectorFor(spec, strings.Split(params.Path, "."), len(embedding))

		if err := c.ensure(ctx); err != nil {
			return nil, err
		}

		q := &pgQuery{}
		where, err := q.where(params.Filter)
		if err != nil {
			return nil, err
		}
		query := q.arg(vectorLiteral(embedding)) + "::" + vec.sqlType
		distance := vec.expr + " " + vec.operator + " " + query

		sql := "SELECT doc::text, " + distance + " FROM " + c.qualified() +
			" WHERE " + vec.expr + " IS NOT NULL AND " + where +
			" ORDER BY " + distance + " LIMIT " + strconv.Itoa(params.K)

		tx, err := c.db.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if params.NumCandidates > 0 {
			ef := min(max(params.NumCandidates, params.K), 1000)
			if _, err := tx.Exec(ctx, "SET LOCAL hnsw.ef_search = "+strconv.Itoa(ef)); err != nil {
				return nil, err
			}
		}

		rows, err := tx.Query(ctx, sql, q.args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		hits := []SearchHit[T]{}
		for rows.Next() {
			var (
				doc  []byte
				dist float64
			)
			if err := rows.Scan(&doc, &dist); err != nil {
				return nil, err
			}
			model, err := jsonToModel[T](doc)
			if err != nil {
				return nil, err
			}
			hits = append(hits, SearchHit[T]{Score: vec.score(dist), Doc: *model})
		}
		return hits, rows.Err()
	})
}

// TermSearch runs a ParadeDB BM25 search over params.Path, matching any of the
// query's words. Every path must be in the model's TermSearchIndexSpecs and
// indexed by EnsurePgIndexes. Needs the pg_search extension.
func (c *pgCollection[T]) TermSearch(ctx context.Context, query string, params TermSearchParams) <-chan async.Result[[]SearchHit[T]] {
	return async.Go(func() ([]SearchHit[T], error) {
		if query == "" || params.IndexName == "" || len(params.Path) == 0 || params.Limit <= 0 {
			return nil, errors.New("invalid input - query, index name, path, and limit must be provided")
		}
		if err := requirePgSearch(ctx, c.db); err != nil {
			return nil, err
		}
		if err := c.ensure(ctx); err != nil {
			return nil, err
		}

		q := &pgQuery{}
		sql, err := termSearchSQL(c.qualified(), q, query, params.Path, params.Filter, params.Limit)
		if err != nil {
			return nil, err
		}

		rows, err := c.db.Query(ctx, sql, q.args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		hits := []SearchHit[T]{}
		for rows.Next() {
			var (
				doc   []byte
				score float64
			)
			if err := rows.Scan(&doc, &score); err != nil {
				return nil, err
			}
			model, err := jsonToModel[T](doc)
			if err != nil {
				return nil, err
			}
			hits = append(hits, SearchHit[T]{Score: score, Doc: *model})
		}
		return hits, rows.Err()
	})
}

func (c *pgCollection[T]) queryModels(ctx context.Context, sql string, args []any) ([]T, error) {
	rows, err := c.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []T{}
	for rows.Next() {
		var doc []byte
		if err := rows.Scan(&doc); err != nil {
			return nil, err
		}
		model, err := jsonToModel[T](doc)
		if err != nil {
			return nil, err
		}
		result = append(result, *model)
	}
	return result, rows.Err()
}

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

/* ── vector & text search expressions, shared with index creation ── */

type pgVector struct {
	expr     string // ((doc #>> '{path}')::vector(N))
	sqlType  string // vector(N) or halfvec(N)
	operator string // <=>, <#>, <->
	opClass  string // HNSW operator class
	score    func(distance float64) float64
}

// pgVectorFor describes how a vector field is cast, compared and indexed.
// Scalar quantization, or more than 2000 dimensions (the HNSW limit for
// vector), stores the index as halfvec.
func pgVectorFor(spec *VectorIndexSpec, path []string, dims int) pgVector {
	base := "vector"
	if spec != nil && (spec.Quantization == "scalar" || spec.NumDimensions > 2000) {
		base = "halfvec"
	}
	v := pgVector{sqlType: fmt.Sprintf("%s(%d)", base, dims)}
	v.expr = "(" + textExpr(path) + "::" + v.sqlType + ")"

	similarity := "cosine"
	if spec != nil && spec.Similarity != "" {
		similarity = spec.Similarity
	}
	switch similarity {
	case "dotProduct":
		// <#> is the negative inner product.
		v.operator, v.opClass = "<#>", base+"_ip_ops"
		v.score = func(d float64) float64 { return (1 - d) / 2 }
	case "euclidean":
		v.operator, v.opClass = "<->", base+"_l2_ops"
		v.score = func(d float64) float64 { return 1 / (1 + d) }
	default: // cosine; <=> is 1 - cosine similarity.
		v.operator, v.opClass = "<=>", base+"_cosine_ops"
		v.score = func(d float64) float64 { return 1 - d/2 }
	}
	return v
}

func findVectorSpec[T DbModel](name string) *VectorIndexSpec {
	var zero T
	vx, ok := any(zero).(VectorIndexed)
	if !ok {
		return nil
	}
	for _, spec := range vx.VectorIndexSpecs() {
		if spec.Name == name {
			return &spec
		}
	}
	return nil
}

func vectorLiteral(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
