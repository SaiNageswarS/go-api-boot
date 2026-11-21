package postgres

import (
	"context"
	"fmt"
	"slices"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-collection-boot/async"
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
	limit  int // -1: no limit
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
		t.err = fmt.Errorf("Where(%q) has %d placeholders but %d arguments", condition, n, len(args))
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
		t.err = fmt.Errorf("Limit(%d) must not be negative", n)
	}
	t.limit = n
	return t
}

// Offset skips the first n rows.
func (t Table[T]) Offset(n int) Table[T] {
	if n < 0 && t.err == nil {
		t.err = fmt.Errorf("Offset(%d) must not be negative", n)
	}
	t.offset = n
	return t
}

// Find returns the matching rows.
func (t Table[T]) Find(ctx context.Context) <-chan async.Result[[]T] {
	return run(t, "Find", func() ([]T, error) { return t.find(ctx) })
}

// First returns the first matching row, or odm.ErrNotFound.
func (t Table[T]) First(ctx context.Context) <-chan async.Result[*T] {
	return run(t, "First", func() (*T, error) { return t.first(ctx) })
}

// FindByID returns the row with id (that also matches t's conditions), or
// odm.ErrNotFound.
func (t Table[T]) FindByID(ctx context.Context, id string) <-chan async.Result[*T] {
	return run(t, "FindByID", func() (*T, error) { return t.Where("id = ?", id).first(ctx) })
}

// Count counts the matching rows (within Limit and Offset, if set).
func (t Table[T]) Count(ctx context.Context) <-chan async.Result[int64] {
	return run(t, "Count", func() (int64, error) { return t.count(ctx) })
}

// Exists reports whether any row matches.
func (t Table[T]) Exists(ctx context.Context) <-chan async.Result[bool] {
	return run(t, "Exists", func() (bool, error) { return t.exists(ctx) })
}

// Delete deletes the matching rows and returns how many it deleted. It needs
// at least one Where (use Where("true") to delete every row) and no OrderBy,
// Limit or Offset.
func (t Table[T]) Delete(ctx context.Context) <-chan async.Result[int64] {
	return run(t, "Delete", func() (int64, error) { return t.delete(ctx) })
}

// DeleteByID deletes the row with id (that also matches t's conditions), if
// there is one.
func (t Table[T]) DeleteByID(ctx context.Context, id string) <-chan async.Result[struct{}] {
	return run(t, "DeleteByID", func() (struct{}, error) {
		_, err := t.Where("id = ?", id).delete(ctx)
		return struct{}{}, err
	})
}

// Save inserts model, or replaces the row with the same Id. It sets
// created_on on insert and updated_on on every save (see odm.Timestamps).
// The query built on t doesn't apply.
//
// It is a single upsert statement, so concurrent saves can't race. For a
// movie it runs:
//
//	INSERT INTO "tenant"."movies" ("id", "title", "year", ..., "created_on", "updated_on")
//	VALUES ($1, $2, $3, ..., $9, $10)
//	ON CONFLICT (id) DO UPDATE SET
//	  "title" = EXCLUDED."title",
//	  "year"  = EXCLUDED."year",
//	  ...,
//	  "updated_on" = EXCLUDED."updated_on"
//
// EXCLUDED is the row that failed to insert. id and created_on are left out
// of the update list, so created_on keeps its first value.
func (t Table[T]) Save(ctx context.Context, model T) <-chan async.Result[struct{}] {
	return run(t, "Save", func() (struct{}, error) { return struct{}{}, t.save(ctx, model) })
}

// VectorSearch returns the k rows nearest to embedding by the model's
// VectorIndexSpec named index, best first, among the rows matching t's
// conditions. It runs a pgvector HNSW search; with conditions it scans the
// index iteratively (pgvector 0.8+) so that filtering doesn't return fewer
// than k rows. OrderBy, Limit and Offset don't apply.
//
// Scores follow Atlas: cosine and dotProduct give (1 + similarity) / 2,
// euclidean gives 1 / (1 + distance).
func (t Table[T]) VectorSearch(ctx context.Context, index string, embedding []float32, k int) <-chan async.Result[[]odm.SearchHit[T]] {
	return run(t, "VectorSearch", func() ([]odm.SearchHit[T], error) { return t.vectorSearch(ctx, index, embedding, k) })
}

// TermSearch returns the limit rows best matching query (any of its words)
// in the columns of the model's TermSearchIndexSpec named index, ranked by
// BM25, among the rows matching t's conditions. It needs ParadeDB's
// pg_search extension. OrderBy, Limit and Offset don't apply.
func (t Table[T]) TermSearch(ctx context.Context, index, query string, limit int) <-chan async.Result[[]odm.SearchHit[T]] {
	return run(t, "TermSearch", func() ([]odm.SearchHit[T], error) { return t.termSearch(ctx, index, query, limit) })
}
