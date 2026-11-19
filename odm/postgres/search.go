package postgres

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-api-boot/odm/internal/asyncx"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/jackc/pgx/v5"
)

// Term search uses ParadeDB's pg_search extension (BM25, like Atlas Search).
//
// Each searchable path gets a stored generated column,
//
//	search_<path> text GENERATED ALWAYS AS (doc #>> '{path}') STORED
//
// and one ParadeDB index (ParadeDB allows one per table) covers all of them.
// Indexing columns rather than expressions on doc keeps ParadeDB out of
// ordinary queries that filter on doc. Postgres fills the columns on write.

// TermSearch matches any word of the query in the index's paths, ranked by BM25.
// Other filters run as row filters inside ParadeDB's top-K scan.
func (c *collection[T]) TermSearch(ctx context.Context, query string, params odm.TermSearchParams) <-chan async.Result[[]odm.SearchHit[T]] {
	return asyncx.Go(func() ([]odm.SearchHit[T], error) {
		if err := params.Validate(query); err != nil {
			return nil, err
		}
		spec, err := odm.TermSearchIndexOf[T](params.IndexName)
		if err != nil {
			return nil, err
		}
		if err := c.db.requirePgSearch(ctx); err != nil {
			return nil, err
		}

		q := &pgQuery{}
		where, err := q.where(params.Filter)
		if err != nil {
			return nil, err
		}
		text := q.arg(query) + "::text"
		matches := make([]string, len(spec.Paths))
		for i, p := range spec.Paths {
			matches[i] = pgx.Identifier{searchColumn(p)}.Sanitize() + " ||| " + text
		}
		sql := "SELECT doc::text, pdb.score(id)::float8 AS score FROM " + c.qualified() +
			" WHERE (" + strings.Join(matches, " OR ") + ") AND " + where +
			" ORDER BY score DESC LIMIT " + strconv.Itoa(params.Limit)
		return queryHits[T](ctx, c.db.pool, sql, q.args, func(s float64) float64 { return s })
	})
}

var nonColumnChars = regexp.MustCompile(`[^A-Za-z0-9_]`)

// searchColumn names the generated column holding a path's text, e.g.
// search_plot. Paths that need sanitising or truncating get a hash suffix, so
// "a.b" and "a_b" stay distinct.
func searchColumn(path string) string {
	clean := nonColumnChars.ReplaceAllString(path, "_")
	name := "search_" + clean
	if clean != path || len(name) > 54 {
		name = truncate(name, 54) + "_" + shortHash(path)
	}
	return name
}

// searchIndexSQL is the definition of the collection's ParadeDB index.
func searchIndexSQL(qualified string, paths []string, stemmer string) string {
	tokenizer := "pdb.unicode_words"
	if stemmer != "" {
		tokenizer += "(" + quoteLiteral("stemmer="+stemmer) + ")"
	}
	fields := []string{"id"}
	for _, p := range paths {
		fields = append(fields, "("+pgx.Identifier{searchColumn(p)}.Sanitize()+"::"+tokenizer+")")
	}
	return "ON " + qualified + " USING paradedb (" + strings.Join(fields, ", ") + ")"
}
