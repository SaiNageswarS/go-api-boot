package postgres

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-api-boot/odm/internal/core"
	"github.com/jackc/pgx/v5"
)

// Term search with ParadeDB's pg_search extension (BM25, like Atlas Search).
// EnsureTable creates one ParadeDB index over the text columns of all the
// model's TermSearchIndexSpecs: ParadeDB allows one per table.

// termSearch is TermSearch.
func (t Table[T]) termSearch(ctx context.Context, index, query string, limit int) ([]odm.SearchHit[T], error) {
	if err := t.searchOnly("TermSearch"); err != nil {
		return nil, err
	}
	if err := core.CheckTermQuery(query, limit); err != nil {
		return nil, err
	}
	spec, err := core.TermSearchIndex[T](index)
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
	where = andWhere(where, "("+strings.Join(matches, " OR ")+")")
	sql := "SELECT " + s.selectList() + ", pdb.score(" + idColumn + ")::float8 AS score FROM " + t.qualified() + where +
		" ORDER BY score DESC LIMIT " + strconv.Itoa(limit)
	return t.queryHits(ctx, t.db.pool, s, sql, args, func(score float64) float64 { return score })
}

// searchIndexSQL is the definition of the table's ParadeDB index over
// columns.
func searchIndexSQL(qualified string, columns []string, stemmer string) string {
	tokenizer := "pdb.unicode_words"
	if stemmer != "" {
		tokenizer += "(" + quoteLiteral("stemmer="+stemmer) + ")"
	}
	fields := []string{idColumn}
	for _, c := range columns {
		fields = append(fields, "("+pgx.Identifier{c}.Sanitize()+"::"+tokenizer+")")
	}
	return "ON " + qualified + " USING paradedb (" + strings.Join(fields, ", ") + ")"
}

// searchColumns checks that the term search paths are text columns of s, and
// returns them without duplicates.
func searchColumns(s *tableSchema, specs []odm.TermSearchIndexSpec) ([]string, error) {
	var columns []string
	for _, spec := range specs {
		if err := core.ValidateTermSearchIndex(spec); err != nil {
			return nil, err
		}
		for _, p := range spec.Paths {
			c, ok := s.byName[p]
			if !ok || c.sqlType != "text" {
				return nil, fmt.Errorf("term search index %s: %q is not a text column", spec.Name, p)
			}
			if !slices.Contains(columns, p) {
				columns = append(columns, p)
			}
		}
	}
	return columns, nil
}
