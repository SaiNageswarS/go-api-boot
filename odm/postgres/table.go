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
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/jackc/pgx/v5"
)

// Table is a typed query over the table of T in one tenant. Build a query by
// chaining Where, OrderBy, Limit and Offset, then run it with a terminal
// method (Find, First, Count, Exists, Delete, VectorSearch, TermSearch):
//
//	top, err := async.Await(profiles.
//	    Where("score > ?", 89).
//	    Where("city = ?", city).
//	    OrderBy("score DESC").
//	    Limit(10).
//	    Find(ctx))
//
// Conditions and orderings are SQL over the table's columns. Each ? is bound
// to the next argument, so values never become part of the SQL text; write ??
// for a literal ? (the jsonb key-exists operator). Pass a slice to match a
// list: Where("city = ANY(?)", cities).
//
// The SQL in Where and OrderBy is executed as written: it must come from your
// code, never from user input. Pass user values as arguments.
//
// A Table is an immutable value: each builder method returns a new Table, so
// a base query can be shared and extended. Terminal methods run
// asynchronously and deliver one async.Result.
type Table[T odm.Model] struct {
	db     *DB
	schema string
	name   string

	where  []string // conditions, with ? placeholders
	args   [][]any  // arguments of each condition
	order  []string
	limit  int
	offset int
	err    error // the first builder error, reported by the terminal method
}

// TableOf returns the table of T in tenant (a Postgres schema).
func TableOf[T odm.Model](db *DB, tenant string) Table[T] {
	var zero T
	return Table[T]{db: db, schema: tenant, name: zero.CollectionName(), limit: -1}
}

// Where adds a condition; conditions are combined with AND.
func (t Table[T]) Where(condition string, args ...any) Table[T] {
	if n := countPlaceholders(condition); n != len(args) && t.err == nil {
		t.err = fmt.Errorf("odm/postgres: Where(%q) has %d placeholders but %d arguments", condition, n, len(args))
	}
	t.where = append(slices.Clip(t.where), condition)
	t.args = append(slices.Clip(t.args), args)
	return t
}

// OrderBy adds orderings, e.g. OrderBy("score DESC", "id").
func (t Table[T]) OrderBy(orderings ...string) Table[T] {
	t.order = append(slices.Clip(t.order), orderings...)
	return t
}

// Limit returns at most n rows.
func (t Table[T]) Limit(n int) Table[T] {
	if n < 0 && t.err == nil {
		t.err = fmt.Errorf("odm/postgres: Limit(%d) must not be negative", n)
	}
	t.limit = n
	return t
}

// Offset skips the first n rows.
func (t Table[T]) Offset(n int) Table[T] {
	if n < 0 && t.err == nil {
		t.err = fmt.Errorf("odm/postgres: Offset(%d) must not be negative", n)
	}
	t.offset = n
	return t
}

/* ── terminal methods ── */

// Find returns the matching rows.
func (t Table[T]) Find(ctx context.Context) <-chan async.Result[[]T] {
	return async.Go(func() ([]T, error) {
		return t.find(ctx)
	})
}

// First returns the first matching row, or odm.ErrNotFound.
func (t Table[T]) First(ctx context.Context) <-chan async.Result[*T] {
	return async.Go(func() (*T, error) {
		rows, err := t.Limit(1).find(ctx)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, odm.ErrNotFound
		}
		return &rows[0], nil
	})
}

// FindByID returns the row with id (that also matches t's conditions), or
// odm.ErrNotFound.
func (t Table[T]) FindByID(ctx context.Context, id string) <-chan async.Result[*T] {
	return t.Where("id = ?", id).First(ctx)
}

// Count counts the matching rows (within Limit and Offset, if set).
func (t Table[T]) Count(ctx context.Context) <-chan async.Result[int64] {
	return async.Go(func() (int64, error) {
		q, err := t.build("1")
		if err != nil {
			return 0, err
		}
		var n int64
		err = t.db.pool.QueryRow(ctx, "SELECT count(*) FROM ("+q.sql+") AS q", q.args...).Scan(&n)
		if isMissing(err) {
			return 0, nil
		}
		return n, t.queryErr(err, q.sql)
	})
}

// Exists reports whether any row matches.
func (t Table[T]) Exists(ctx context.Context) <-chan async.Result[bool] {
	return async.Go(func() (bool, error) {
		q, err := t.build("1")
		if err != nil {
			return false, err
		}
		var ok bool
		err = t.db.pool.QueryRow(ctx, "SELECT EXISTS ("+q.sql+")", q.args...).Scan(&ok)
		if isMissing(err) {
			return false, nil
		}
		return ok, t.queryErr(err, q.sql)
	})
}

// Delete deletes the matching rows and returns how many it deleted. It needs
// at least one Where (use Where("true") to delete every row) and no OrderBy,
// Limit or Offset.
func (t Table[T]) Delete(ctx context.Context) <-chan async.Result[int64] {
	return async.Go(func() (int64, error) {
		if t.err != nil {
			return 0, t.err
		}
		if len(t.where) == 0 {
			return 0, errors.New(`odm/postgres: Delete needs a Where; use Where("true") to delete every row`)
		}
		if len(t.order) > 0 || t.limit >= 0 || t.offset > 0 {
			return 0, errors.New("odm/postgres: Delete doesn't take OrderBy, Limit or Offset")
		}
		where, args := t.whereSQL(nil)
		sql := "DELETE FROM " + t.qualified() + where
		tag, err := t.db.pool.Exec(ctx, sql, args...)
		if isMissing(err) {
			return 0, nil
		}
		return tag.RowsAffected(), t.queryErr(err, sql)
	})
}

// DeleteByID deletes the row with id (that also matches t's conditions), if
// there is one.
func (t Table[T]) DeleteByID(ctx context.Context, id string) <-chan async.Result[struct{}] {
	return async.Go(func() (struct{}, error) {
		_, err := async.Await(t.Where("id = ?", id).Delete(ctx))
		return struct{}{}, err
	})
}

// Save inserts model, or replaces the row with the same Id. It sets
// created_on on insert and updated_on on every save, when the model has those
// int64 columns. The query built on t doesn't apply.
func (t Table[T]) Save(ctx context.Context, model T) <-chan async.Result[struct{}] {
	return async.Go(func() (struct{}, error) {
		s, err := schemaOf[T]()
		if err != nil {
			return struct{}{}, err
		}
		id := model.Id()
		if err := odm.CheckID(id); err != nil {
			return struct{}{}, err
		}

		v := reflect.ValueOf(model)
		now := t.db.timer.Now()
		var (
			names, values, updates []string
			args                   []any
		)
		for _, c := range s.columns {
			var (
				arg  any
				cast string
			)
			switch c.name {
			case "id":
				arg = id
			case "created_on", "updated_on":
				if (c.name == "created_on" && s.createdOn != nil) || (c.name == "updated_on" && s.updatedOn != nil) {
					arg = now
					break
				}
				fallthrough
			default:
				if arg, cast, err = c.arg(v); err != nil {
					return struct{}{}, err
				}
			}
			args = append(args, arg)
			names = append(names, c.quoted())
			values = append(values, "$"+strconv.Itoa(len(args))+cast)
			if c.name != "id" && !(c.name == "created_on" && s.createdOn != nil) {
				updates = append(updates, c.quoted()+" = EXCLUDED."+c.quoted())
			}
		}

		conflict := "DO NOTHING"
		if len(updates) > 0 {
			conflict = "DO UPDATE SET " + strings.Join(updates, ", ")
		}
		_, err = t.db.pool.Exec(ctx,
			"INSERT INTO "+t.qualified()+" ("+strings.Join(names, ", ")+") VALUES ("+strings.Join(values, ", ")+
				") ON CONFLICT (id) "+conflict, args...)
		return struct{}{}, writeErr(err, t.schema, t.name)
	})
}

/* ── query building ── */

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

func (t Table[T]) find(ctx context.Context) ([]T, error) {
	s, err := schemaOf[T]()
	if err != nil {
		return nil, err
	}
	q, err := t.build(selectList(s))
	if err != nil {
		return nil, err
	}
	rows, err := t.db.pool.Query(ctx, q.sql, q.args...)
	if isMissing(err) {
		return []T{}, nil
	}
	if err != nil {
		return nil, t.queryErr(err, q.sql)
	}
	out, err := scanRows[T](rows, s, nil)
	if isMissing(err) {
		return []T{}, nil
	}
	return out, t.queryErr(err, q.sql)
}

// queryErr adds the SQL to errors from it, which mostly come from the
// conditions and orderings callers wrote.
func (t Table[T]) queryErr(err error, sql string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("odm/postgres: %w (in %s)", err, sql)
}

func selectList(s *tableSchema) string {
	cols := make([]string, len(s.columns))
	for i, c := range s.columns {
		cols[i] = c.selectExpr()
	}
	return strings.Join(cols, ", ")
}

// scanRows decodes rows into T. With score non-nil, each row has one more
// float8 column, appended to score.
func scanRows[T odm.Model](rows pgx.Rows, s *tableSchema, scores *[]float64) ([]T, error) {
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		var model T
		v := reflect.ValueOf(&model).Elem()
		targets := make([]any, 0, len(s.columns)+1)
		var finish []func() error
		for _, c := range s.columns {
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

/* ── placeholders ── */

// countPlaceholders counts the ? placeholders in sql.
func countPlaceholders(sql string) int {
	n := 0
	scanPlaceholders(sql, func() string { n++; return "" })
	return n
}

// bindPlaceholders replaces each ? in sql with $k, numbering from after+1.
func bindPlaceholders(sql string, after int) string {
	return scanPlaceholders(sql, func() string { after++; return "$" + strconv.Itoa(after) })
}

// scanPlaceholders rewrites sql, replacing each placeholder ? with next() and
// each ?? with ?. Question marks inside quoted strings, quoted identifiers and
// comments are left alone.
func scanPlaceholders(sql string, next func() string) string {
	var b strings.Builder
	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		switch {
		case ch == '\'' || ch == '"':
			end := closingQuote(sql, i)
			b.WriteString(sql[i:end])
			i = end - 1
		case ch == '-' && strings.HasPrefix(sql[i:], "--"):
			end := strings.IndexByte(sql[i:], '\n')
			if end < 0 {
				end = len(sql) - i
			}
			b.WriteString(sql[i : i+end])
			i += end - 1
		case ch == '/' && strings.HasPrefix(sql[i:], "/*"):
			end := strings.Index(sql[i+2:], "*/")
			if end < 0 {
				end = len(sql) - i - 2
			} else {
				end += 2
			}
			b.WriteString(sql[i : i+2+end])
			i += 2 + end - 1
		case ch == '?' && strings.HasPrefix(sql[i:], "??"):
			b.WriteByte('?')
			i++
		case ch == '?':
			b.WriteString(next())
		default:
			b.WriteByte(ch)
		}
	}
	return b.String()
}

// closingQuote returns the index just past the quoted string or identifier
// starting at i; a doubled quote is an escaped quote.
func closingQuote(sql string, i int) int {
	q := sql[i]
	for j := i + 1; j < len(sql); j++ {
		if sql[j] == q {
			if j+1 < len(sql) && sql[j+1] == q {
				j++
				continue
			}
			return j + 1
		}
	}
	return len(sql)
}
