package postgres

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-api-boot/odm/internal/core"
	"github.com/jackc/pgx/v5"
)

// The query builder and executor behind Table's methods. Errors are plain
// causes; the public methods wrap them.

func (t Table[T]) qualified() string { return pgx.Identifier{t.schema, t.name}.Sanitize() }

type builtQuery struct {
	sql  string
	args []any
}

// build renders SELECT <selectList> FROM table WHERE ... ORDER BY ... LIMIT
// ... OFFSET ....
func (t Table[T]) build(selectList string) (builtQuery, error) {
	if t.err != nil {
		return builtQuery{}, t.err
	}
	where, args := t.whereSQL(nil)
	return builtQuery{"SELECT " + selectList + " FROM " + t.qualified() + where + t.pagingSQL(), args}, nil
}

// whereSQL renders the conditions after prefix arguments (numbered from
// len(prefix)+1), returning " WHERE ..." (or "") and all arguments.
func (t Table[T]) whereSQL(prefix []any) (string, []any) {
	args := slices.Clone(prefix)
	if len(t.where) == 0 {
		return "", args
	}
	parts := make([]string, len(t.where))
	for i, cond := range t.where {
		parts[i] = "(" + bindPlaceholders(cond, len(args)) + ")"
		args = append(args, t.args[i]...)
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

// andWhere is where with one more condition, already in SQL.
func andWhere(where, condition string) string {
	if where == "" {
		return " WHERE " + condition
	}
	return where + " AND " + condition
}

func (t Table[T]) pagingSQL() string {
	sql := ""
	if len(t.order) > 0 {
		sql += " ORDER BY " + strings.Join(t.order, ", ")
	}
	if t.limit >= 0 {
		sql += " LIMIT " + strconv.Itoa(t.limit)
	}
	if t.offset > 0 {
		sql += " OFFSET " + strconv.Itoa(t.offset)
	}
	return sql
}

// paged reports whether the query has OrderBy, Limit or Offset.
func (t Table[T]) paged() bool { return len(t.order) > 0 || t.limit >= 0 || t.offset > 0 }

func (t Table[T]) find(ctx context.Context) ([]T, error) {
	s, err := schemaOf[T]()
	if err != nil {
		return nil, err
	}
	q, err := t.build(s.selectList())
	if err != nil {
		return nil, err
	}
	rows, err := t.db.pool.Query(ctx, q.sql, q.args...)
	if err != nil {
		return emptyIfMissing[T](queryErr(err, q.sql))
	}
	out, err := scanRows[T](rows, s, nil)
	if err != nil {
		return emptyIfMissing[T](queryErr(err, q.sql))
	}
	return out, nil
}

// emptyIfMissing turns a query of a missing table into no rows.
func emptyIfMissing[T any](err error) ([]T, error) {
	if isMissing(err) {
		return []T{}, nil
	}
	return nil, err
}

func (t Table[T]) first(ctx context.Context) (*T, error) {
	rows, err := t.Limit(1).find(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, odm.ErrNotFound
	}
	return &rows[0], nil
}

func (t Table[T]) count(ctx context.Context) (int64, error) {
	q, err := t.build("1")
	if err != nil {
		return 0, err
	}
	var n int64
	err = t.db.pool.QueryRow(ctx, "SELECT count(*) FROM ("+q.sql+") AS q", q.args...).Scan(&n)
	if isMissing(err) {
		return 0, nil
	}
	return n, queryErr(err, q.sql)
}

func (t Table[T]) exists(ctx context.Context) (bool, error) {
	q, err := t.build("1")
	if err != nil {
		return false, err
	}
	var ok bool
	err = t.db.pool.QueryRow(ctx, "SELECT EXISTS ("+q.sql+")", q.args...).Scan(&ok)
	if isMissing(err) {
		return false, nil
	}
	return ok, queryErr(err, q.sql)
}

func (t Table[T]) delete(ctx context.Context) (int64, error) {
	if t.err != nil {
		return 0, t.err
	}
	if len(t.where) == 0 {
		return 0, errors.New(`Delete needs a Where; use Where("true") to delete every row`)
	}
	if t.paged() {
		return 0, errors.New("Delete doesn't take OrderBy, Limit or Offset")
	}
	where, args := t.whereSQL(nil)
	sql := "DELETE FROM " + t.qualified() + where
	tag, err := t.db.pool.Exec(ctx, sql, args...)
	if isMissing(err) {
		return 0, nil
	}
	return tag.RowsAffected(), queryErr(err, sql)
}

// save upserts model; Save documents the statement.
func (t Table[T]) save(ctx context.Context, model T) error {
	s, err := schemaOf[T]()
	if err != nil {
		return err
	}
	id := model.Id()
	if err := core.CheckID(id); err != nil {
		return err
	}

	v := reflect.ValueOf(model)
	now := t.db.timer.Now()
	names := make([]string, len(s.columns))
	values := make([]string, len(s.columns))
	args := make([]any, len(s.columns))
	var updates []string
	for i, c := range s.columns {
		cast := ""
		switch c.name {
		case idColumn:
			args[i] = id
		case createdOnColumn, updatedOnColumn:
			args[i] = now
		default:
			if args[i], cast, err = c.arg(v); err != nil {
				return err
			}
		}
		names[i] = c.quoted()
		values[i] = "$" + strconv.Itoa(i+1) + cast
		if c.name != idColumn && c.name != createdOnColumn {
			updates = append(updates, c.quoted()+" = EXCLUDED."+c.quoted())
		}
	}

	_, err = t.db.pool.Exec(ctx, "INSERT INTO "+t.qualified()+
		" ("+strings.Join(names, ", ")+") VALUES ("+strings.Join(values, ", ")+
		") ON CONFLICT ("+idColumn+") DO UPDATE SET "+strings.Join(updates, ", "), args...)
	return writeErr(err)
}

// searchOnly rejects paging on a search, which orders by relevance itself.
func (t Table[T]) searchOnly(method string) error {
	if t.err != nil {
		return t.err
	}
	if t.paged() {
		return fmt.Errorf("%s orders by relevance and takes its own limit; drop OrderBy, Limit and Offset", method)
	}
	return nil
}

// querier is a pool or a transaction.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// queryHits runs a search whose rows are the model's columns plus a distance
// or score, which score turns into the hit's score.
func (t Table[T]) queryHits(ctx context.Context, q querier, s *tableSchema, sql string, args []any, score func(float64) float64) ([]odm.SearchHit[T], error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return emptyIfMissing[odm.SearchHit[T]](queryErr(err, sql))
	}
	var raw []float64
	models, err := scanRows[T](rows, s, &raw)
	if err != nil {
		return nil, queryErr(err, sql)
	}
	hits := make([]odm.SearchHit[T], len(models))
	for i, m := range models {
		hits[i] = odm.SearchHit[T]{Score: score(raw[i]), Doc: m}
	}
	return hits, nil
}

// scanRows decodes rows into T. With scores non-nil, each row has one more
// float8 column, appended to scores.
func scanRows[T odm.Model](rows pgx.Rows, s *tableSchema, scores *[]float64) ([]T, error) {
	defer rows.Close()
	fields := s.fields()
	out := []T{}
	for rows.Next() {
		var model T
		v := reflect.ValueOf(&model).Elem()
		targets := make([]any, 0, len(fields)+1)
		var finish []func() error
		for _, c := range fields {
			target, done := c.scanTarget(v)
			targets = append(targets, target)
			if done != nil {
				finish = append(finish, done)
			}
		}
		var score float64
		if scores != nil {
			targets = append(targets, &score)
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, err
		}
		for _, done := range finish {
			if err := done(); err != nil {
				return nil, err
			}
		}
		out = append(out, model)
		if scores != nil {
			*scores = append(*scores, score)
		}
	}
	return out, rows.Err()
}
