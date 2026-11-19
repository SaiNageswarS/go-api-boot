package odm

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
)

// Term search on Postgres uses ParadeDB's pg_search extension (BM25, like
// Atlas Search). See https://www.paradedb.com/docs.
//
// Each searchable path gets a stored generated column,
//
//	search_<path> text GENERATED ALWAYS AS (doc #>> '{path}') STORED
//
// and one ParadeDB index (ParadeDB allows one per table) covers all of them.
// Indexing the columns rather than expressions on doc keeps ParadeDB out of
// ordinary queries that filter on doc. Postgres fills the columns on every
// write, so Save is unchanged.

// ErrPgSearchNotInstalled is returned by EnsurePgIndexes and TermSearch when
// the database doesn't have ParadeDB's pg_search extension.
var ErrPgSearchNotInstalled = errors.New(
	"odm/postgres: term search needs ParadeDB's pg_search extension " +
		"(add pg_search to shared_preload_libraries and CREATE EXTENSION pg_search; see https://www.paradedb.com/docs)")

// PgSearchStemmer is the stemmer language for term search indexes (for example
// "english"); empty disables stemming. Change it before creating indexes.
var PgSearchStemmer = "english"

var pgSearchChecked sync.Map // PgClient -> struct{}, once pg_search is confirmed

func requirePgSearch(ctx context.Context, db PgClient) error {
	key := fmt.Sprintf("%p", db)
	if _, ok := pgSearchChecked.Load(key); ok {
		return nil
	}
	var installed bool
	if err := db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_search')").Scan(&installed); err != nil {
		return err
	}
	if !installed {
		return ErrPgSearchNotInstalled
	}
	pgSearchChecked.Store(key, struct{}{})
	return nil
}

// ensurePgSearchIndex adds the search columns and creates the collection's
// ParadeDB index over them, or checks that an existing one covers them.
func ensurePgSearchIndex(ctx context.Context, db PgClient, schema, table string, paths []string) error {
	// Installing needs pg_search in shared_preload_libraries; if that's missing
	// CREATE EXTENSION fails and requirePgSearch reports it.
	_, _ = db.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pg_search CASCADE")
	if err := requirePgSearch(ctx, db); err != nil {
		return err
	}

	qualified := pgx.Identifier{schema, table}.Sanitize()
	for _, p := range paths {
		if _, err := db.Exec(ctx, fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s text GENERATED ALWAYS AS (%s) STORED",
			qualified, pgSearchColumn(p), textExpr(strings.Split(p, ".")))); err != nil {
			return err
		}
	}

	var existingName, existingDef string
	err := db.QueryRow(ctx,
		`SELECT i.relname, pg_get_indexdef(i.oid)
		   FROM pg_index x
		   JOIN pg_class i ON i.oid = x.indexrelid
		   JOIN pg_am am ON am.oid = i.relam
		  WHERE x.indrelid = $1::regclass AND am.amname IN ('paradedb', 'bm25')`,
		qualified).Scan(&existingName, &existingDef)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		_, err = db.Exec(ctx, pgSearchIndexSQL(schema, table, paths))
		return err
	case err != nil:
		return err
	}

	var missing []string
	for _, p := range paths {
		// pg_get_indexdef quotes the column only when it needs quoting.
		raw := pgSearchColumnName(p)
		if !strings.Contains(existingDef, "("+raw+")") && !strings.Contains(existingDef, "("+pgSearchColumn(p)+")") {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("odm/postgres: ParadeDB index %s on %s.%s doesn't cover %s; "+
			"Postgres allows one ParadeDB index per table, so drop it and run EnsurePgIndexes again",
			existingName, schema, table, strings.Join(missing, ", "))
	}
	return nil
}

func pgSearchIndexSQL(schema, table string, paths []string) string {
	tokenizer := "pdb.unicode_words"
	if PgSearchStemmer != "" {
		tokenizer += "(" + quoteLiteral("stemmer="+PgSearchStemmer) + ")"
	}
	fields := []string{"id"}
	for _, p := range paths {
		fields = append(fields, "("+pgSearchColumn(p)+"::"+tokenizer+")")
	}
	return fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s USING paradedb (%s)",
		pgx.Identifier{table + "_search"}.Sanitize(),
		pgx.Identifier{schema, table}.Sanitize(),
		strings.Join(fields, ", "))
}

var nonColumnChars = regexp.MustCompile(`[^A-Za-z0-9_]`)

// pgSearchColumnName is the generated column holding a path's text, e.g. search_plot.
func pgSearchColumnName(path string) string {
	return "search_" + nonColumnChars.ReplaceAllString(path, "_")
}

func pgSearchColumn(path string) string { return pgx.Identifier{pgSearchColumnName(path)}.Sanitize() }

// termSearchSQL matches any word of the query in any of the paths, ranked by
// BM25. ParadeDB applies the other filters as row filters inside its top-K scan.
func termSearchSQL(qualified string, q *pgQuery, query string, paths []string, filter any, limit int) (string, error) {
	where, err := q.where(filter)
	if err != nil {
		return "", err
	}
	text := q.arg(query) + "::text"
	matches := make([]string, len(paths))
	for i, p := range paths {
		matches[i] = pgSearchColumn(p) + " ||| " + text
	}
	return "SELECT doc::text, pdb.score(id)::float8 AS score FROM " + qualified +
		" WHERE (" + strings.Join(matches, " OR ") + ") AND " + where +
		" ORDER BY score DESC LIMIT " + strconv.Itoa(limit), nil
}
