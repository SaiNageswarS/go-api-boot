package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/jackc/pgx/v5"
)

// managedComment marks the indexes EnsureCollection owns.
const managedComment = "odm"

// EnsureCollection creates T's table in tenant and reconciles its indexes with
// the ones T declares. Run it when deploying or onboarding a tenant; it is
// idempotent and runs in one transaction.
//
//   - IndexSpecs become btree indexes (TTL is unsupported);
//   - TermSearchIndexSpecs become one ParadeDB index over all their paths
//     (needs pg_search, else ErrPgSearchNotInstalled);
//   - VectorIndexSpecs become pgvector HNSW indexes over documents whose
//     embedding has the declared dimensions (needs pgvector, else
//     ErrVectorNotInstalled); their FilterPaths get btree indexes.
//
// Index names end in a hash of their definition. A changed definition creates
// a new index and drops the old one; indexes no longer declared are dropped.
// Indexes EnsureCollection didn't create are left alone.
func EnsureCollection[T odm.Model](ctx context.Context, db *DB, tenant string) error {
	var zero T
	table := zero.CollectionName()
	qualified := pgx.Identifier{tenant, table}.Sanitize()

	type index struct{ name, sql string }
	var indexes []index
	add := func(label, unique, def string) {
		name := truncate(table+"_"+label, 54) + "_" + shortHash(unique+def)
		indexes = append(indexes, index{name, "CREATE " + unique + "INDEX " + pgx.Identifier{name}.Sanitize() + " " + def})
	}

	// Every equality filter compiles to doc @> ..., served by this index.
	add("doc", "", "ON "+qualified+" USING gin (doc jsonb_path_ops)")

	for _, spec := range odm.IndexesOf[T]() {
		if len(spec.Keys) == 0 {
			return fmt.Errorf("odm/postgres: index %q has no keys", spec.Name)
		}
		if spec.TTL > 0 {
			return errUnsupported("index %s: TTL", spec.DisplayName())
		}
		cols := make([]string, len(spec.Keys))
		for i, k := range spec.Keys {
			cols[i] = sortExpr(k.Field)
			if k.Desc {
				cols[i] += " DESC NULLS LAST"
			} else {
				cols[i] += " ASC NULLS FIRST"
			}
		}
		unique := ""
		if spec.Unique {
			unique = "UNIQUE "
		}
		add(spec.DisplayName(), unique, "ON "+qualified+" ("+strings.Join(cols, ", ")+")")
	}

	vectorSpecs := odm.VectorIndexesOf[T]()
	for _, spec := range vectorSpecs {
		vec, err := pgVectorFor(spec)
		if err != nil {
			return err
		}
		add(spec.Name, "", "ON "+qualified+" USING hnsw ("+vec.expr+" "+vec.opClass+") WHERE "+vec.predicate)
		for _, p := range spec.FilterPaths {
			add(spec.Name+"_"+p, "", "ON "+qualified+" ("+sortExpr(p)+")")
		}
	}

	var searchPaths []string
	for _, spec := range odm.TermSearchIndexesOf[T]() {
		for _, p := range spec.Paths {
			if !slices.Contains(searchPaths, p) {
				searchPaths = append(searchPaths, p)
			}
		}
	}
	if len(searchPaths) > 0 {
		add("search", "", searchIndexSQL(qualified, searchPaths, db.stemmer))
	}

	// Extensions can't be created inside the transaction below without
	// aborting it when they're missing.
	if len(vectorSpecs) > 0 {
		if err := db.installExtension(ctx, "vector", ErrVectorNotInstalled); err != nil {
			return err
		}
	}
	if len(searchPaths) > 0 {
		if err := db.installExtension(ctx, "pg_search", ErrPgSearchNotInstalled); err != nil {
			return err
		}
	}

	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	exec := func(sql string, args ...any) error {
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			return fmt.Errorf("odm/postgres: %s: %w", sql, err)
		}
		return nil
	}
	if err := exec("SELECT pg_advisory_xact_lock(hashtext($1))", "odm:"+tenant+"."+table); err != nil {
		return err
	}
	if err := exec("CREATE SCHEMA IF NOT EXISTS " + pgx.Identifier{tenant}.Sanitize()); err != nil {
		return err
	}
	if err := exec("CREATE TABLE IF NOT EXISTS " + qualified + " (id TEXT PRIMARY KEY, doc JSONB NOT NULL)"); err != nil {
		return err
	}

	// Drop managed indexes that are no longer wanted (first: ParadeDB allows
	// only one index per table, so an old search index must go before a new one).
	existing, err := queryStrings(ctx, tx,
		`SELECT c.relname FROM pg_index x JOIN pg_class c ON c.oid = x.indexrelid
		  WHERE x.indrelid = $1::regclass AND obj_description(c.oid, 'pg_class') = $2`,
		qualified, managedComment)
	if err != nil {
		return err
	}
	wanted := map[string]bool{}
	for _, ix := range indexes {
		wanted[ix.name] = true
	}
	for _, name := range existing {
		if !wanted[name] {
			if err := exec("DROP INDEX " + pgx.Identifier{tenant, name}.Sanitize()); err != nil {
				return err
			}
		}
	}

	// Reconcile the generated search columns.
	columns, err := queryStrings(ctx, tx,
		`SELECT attname FROM pg_attribute
		  WHERE attrelid = $1::regclass AND attnum > 0 AND NOT attisdropped AND attname LIKE 'search\_%'`,
		qualified)
	if err != nil {
		return err
	}
	wantedColumns := map[string]string{}
	for _, p := range searchPaths {
		wantedColumns[searchColumn(p)] = p
	}
	for _, col := range columns {
		if _, ok := wantedColumns[col]; !ok {
			if err := exec("ALTER TABLE " + qualified + " DROP COLUMN " + pgx.Identifier{col}.Sanitize()); err != nil {
				return err
			}
		}
	}
	for _, p := range searchPaths {
		col := searchColumn(p)
		if slices.Contains(columns, col) {
			continue
		}
		if err := exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s text GENERATED ALWAYS AS (%s) STORED",
			qualified, pgx.Identifier{col}.Sanitize(), textExpr(strings.Split(p, ".")))); err != nil {
			return err
		}
	}

	for _, ix := range indexes {
		if slices.Contains(existing, ix.name) {
			continue
		}
		if err := exec(ix.sql); err != nil {
			return err
		}
		if err := exec("COMMENT ON INDEX " + pgx.Identifier{tenant, ix.name}.Sanitize() + " IS " + quoteLiteral(managedComment)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func queryStrings(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]string, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:8]
}

// truncate cuts s to at most n bytes without splitting a UTF-8 character.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
