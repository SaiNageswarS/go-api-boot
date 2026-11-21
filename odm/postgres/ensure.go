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
	"github.com/SaiNageswarS/go-api-boot/odm/internal/core"
	"github.com/jackc/pgx/v5"
)

// managedComment marks the indexes EnsureTable owns.
const managedComment = "odm"

// EnsureTable creates T's table in tenant and reconciles it with the model. Run it when deploying or onboarding a tenant; it is idempotent and
// runs in one transaction.
//
// Columns: missing ones are added (NOT NULL with the zero value as default,
// so existing rows stay readable). A column whose type no longer matches its
// field is an error, since changing it needs a data migration. Columns the
// model no longer has are left in place.
//
// Indexes:
//   - IndexSpecs become btree indexes over their key columns (TTL is
//     unsupported);
//   - TermSearchIndexSpecs become one ParadeDB index over all their text
//     columns (needs pg_search, else ErrPgSearchNotInstalled);
//   - VectorIndexSpecs become pgvector HNSW indexes on their vector column
//     (needs pgvector, else ErrVectorNotInstalled). FilterPaths are MongoDB
//     only; on Postgres, declare IndexSpecs for the columns you filter on.
//
// Index names end in a hash of their definition. A changed definition creates
// a new index and drops the old one; indexes no longer declared are dropped.
// Indexes EnsureTable didn't create are left alone.
func EnsureTable[T odm.Model](ctx context.Context, db *DB, tenant string) error {
	var zero T
	return core.Wrap(backend, "EnsureTable", tenant+"."+zero.CollectionName(), ensureTable[T](ctx, db, tenant))
}

func ensureTable[T odm.Model](ctx context.Context, db *DB, tenant string) error {
	s, err := schemaOf[T]()
	if err != nil {
		return err
	}
	var zero T
	table := zero.CollectionName()
	qualified := pgx.Identifier{tenant, table}.Sanitize()

	type index struct{ name, sql string }
	var indexes []index
	add := func(label, unique, def string) {
		name := truncate(table+"_"+label, 54) + "_" + shortHash(unique+def)
		indexes = append(indexes, index{name, "CREATE " + unique + "INDEX " + pgx.Identifier{name}.Sanitize() + " " + def})
	}

	for _, spec := range core.Indexes[T]() {
		if err := core.ValidateIndex(spec); err != nil {
			return err
		}
		if spec.TTL > 0 {
			return core.Unsupported("index %s: TTL", core.IndexName(spec))
		}
		cols := make([]string, len(spec.Keys))
		for i, k := range spec.Keys {
			if _, ok := s.byName[k.Field]; !ok {
				return fmt.Errorf("index %s: model has no column %q", core.IndexName(spec), k.Field)
			}
			cols[i] = pgx.Identifier{k.Field}.Sanitize()
			if k.Desc {
				cols[i] += " DESC"
			}
		}
		unique := ""
		if spec.Unique {
			unique = "UNIQUE "
		}
		add(core.IndexName(spec), unique, "ON "+qualified+" ("+strings.Join(cols, ", ")+")")
	}

	vectorSpecs := core.VectorIndexes[T]()
	for _, spec := range vectorSpecs {
		vec, err := pgVectorFor(spec)
		if err != nil {
			return err
		}
		add(spec.Name, "", "ON "+qualified+" USING hnsw ("+pgx.Identifier{spec.Path}.Sanitize()+" "+vec.opClass+")")
	}

	searchCols, err := searchColumns(s, core.TermSearchIndexes[T]())
	if err != nil {
		return err
	}
	if len(searchCols) > 0 {
		add("search", "", searchIndexSQL(qualified, searchCols, db.stemmer))
	}

	// Extensions can't be created inside the transaction below without
	// aborting it when they're missing.
	if len(vectorSpecs) > 0 {
		if err := db.installExtension(ctx, "vector", ErrVectorNotInstalled); err != nil {
			return err
		}
	}
	if len(searchCols) > 0 {
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
			return fmt.Errorf("%s: %w", sql, err)
		}
		return nil
	}
	if err := exec("SELECT pg_advisory_xact_lock(hashtext($1))", "odm:"+tenant+"."+table); err != nil {
		return err
	}
	if err := exec("CREATE SCHEMA IF NOT EXISTS " + pgx.Identifier{tenant}.Sanitize()); err != nil {
		return err
	}
	defs := make([]string, len(s.columns))
	for i, c := range s.columns {
		defs[i] = c.definition()
	}
	if err := exec("CREATE TABLE IF NOT EXISTS " + qualified + " (" + strings.Join(defs, ", ") + ")"); err != nil {
		return err
	}

	// Reconcile columns with the model.
	rows, err := tx.Query(ctx,
		`SELECT attname, format_type(atttypid, atttypmod) FROM pg_attribute
		  WHERE attrelid = $1::regclass AND attnum > 0 AND NOT attisdropped`, qualified)
	if err != nil {
		return err
	}
	existingCols := map[string]string{}
	var name, typ string
	if _, err := pgx.ForEachRow(rows, []any{&name, &typ}, func() error {
		existingCols[name] = typ
		return nil
	}); err != nil {
		return err
	}
	for _, c := range s.columns {
		typ, ok := existingCols[c.name]
		switch {
		case !ok:
			if err := exec("ALTER TABLE " + qualified + " ADD COLUMN " + c.definition()); err != nil {
				return err
			}
		case typ != c.sqlType:
			return fmt.Errorf("column %s.%s is %s but the model needs %s; migrate it, then run EnsureTable again",
				table, c.name, typ, c.sqlType)
		}
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
