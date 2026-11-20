package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/jackc/pgx/v5"
)

// Term search uses ParadeDB's pg_search extension (BM25, like Atlas Search).
// EnsureCollection creates one ParadeDB index over the text columns of all
// the model's TermSearchIndexSpecs (ParadeDB allows one per table).

// TermSearch returns the limit rows best matching query (any of its words)
// in the columns of the model's TermSearchIndexSpec named index, ranked by
// BM25, among the rows matching t's conditions. OrderBy, Limit and Offset
// don't apply.
func (t Table[T]) TermSearch(ctx context.Context, index, query string, limit int) <-chan async.Result[[]odm.SearchHit[T]] {
	return async.Go(func() ([]odm.SearchHit[T], error) {
		if query == "" || limit <= 0 {
			return nil, errors.New("odm/postgres: TermSearch needs a query and limit > 0")
		}
		if err := t.searchOnly("TermSearch"); err != nil {
			return nil, err
		}
		spec, err := odm.TermSearchIndexOf[T](index)
		if err != nil {
			return nil, err
		}
		s, err := schemaOf[T]()
		if err != nil {
			return nil, err
		}
		if err := t.db.requirePgSearch(ctx); err != nil {
			return nil, err
		}

		matches := make([]string, len(spec.Paths))
		for i, p := range spec.Paths {
			matches[i] = pgx.Identifier{p}.Sanitize() + " ||| $1::text"
		}
		where, args := t.whereSQL([]any{query})
		match := "(" + strings.Join(matches, " OR ") + ")"
		if where == "" {
			where = " WHERE " + match
		} else {
			where += " AND " + match
		}
		sql := "SELECT " + selectList(s) + ", pdb.score(id)::float8 AS score FROM " + t.qualified() + where +
			" ORDER BY score DESC LIMIT " + strconv.Itoa(limit)
		return t.queryHits(ctx, t.db.pool, s, sql, args, func(score float64) float64 { return score })
	})
}

// searchIndexSQL is the definition of the table's ParadeDB index over
// columns.
func searchIndexSQL(qualified string, columns []string, stemmer string) string {
	tokenizer := "pdb.unicode_words"
	if stemmer != "" {
		tokenizer += "(" + quoteLiteral("stemmer="+stemmer) + ")"
	}
	fields := []string{"id"}
	for _, c := range columns {
		fields = append(fields, "("+pgx.Identifier{c}.Sanitize()+"::"+tokenizer+")")
	}
	return "ON " + qualified + " USING paradedb (" + strings.Join(fields, ", ") + ")"
}

// searchColumns checks that the term search paths are text columns of s.
func searchColumns(s *tableSchema, specs []odm.TermSearchIndexSpec) ([]string, error) {
	var columns []string
	for _, spec := range specs {
		if len(spec.Paths) == 0 {
			return nil, fmt.Errorf("odm/postgres: term search index %s has no paths", spec.Name)
		}
		for _, p := range spec.Paths {
			c, ok := s.byName[p]
			if !ok || c.sqlType != "text" {
				return nil, fmt.Errorf("odm/postgres: term search index %s: %q is not a text column", spec.Name, p)
			}
			if !slices.Contains(columns, p) {
				columns = append(columns, p)
			}
		}
	}
	return columns, nil
}
