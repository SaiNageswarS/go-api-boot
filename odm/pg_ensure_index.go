package odm

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// EnsurePgIndexes is EnsureIndexes for Postgres. It creates the table and
// translates every index the model advertises:
//   - IndexModels (1 / -1 keys, Unique, Name) become btree indexes on the fields;
//   - TermSearchIndexSpecs become GIN full-text indexes;
//   - VectorIndexSpecs become pgvector HNSW indexes (type "filter" specs become
//     btree indexes). The pgvector extension is created if missing.
//
// Index names are prefixed with the collection name, since Postgres index names
// are unique per schema, not per table.
func EnsurePgIndexes[T DbModel](ctx context.Context, client PostgresClient, tenant string) error {
	var zero T
	table := zero.CollectionName()
	if err := ensurePgTable(ctx, client, tenant, table); err != nil {
		return err
	}

	var stmts []string
	qualified := pgx.Identifier{tenant, table}.Sanitize()
	indexName := func(name string) string { return pgx.Identifier{table + "_" + name}.Sanitize() }

	// --- Classic indexes ----------------------------------------------------
	if ix, ok := any(zero).(Indexed); ok {
		for _, model := range ix.IndexModels() {
			pairs, ok := toPairs(model.Keys)
			if !ok || len(pairs) == 0 {
				return fmt.Errorf("odm/postgres: index keys must be a non-empty document")
			}

			cols := make([]string, len(pairs))
			nameParts := make([]string, len(pairs))
			for i, e := range pairs {
				dir, ok := toInt64(e.Value)
				if !ok || (dir != 1 && dir != -1) {
					return fmt.Errorf("odm/postgres: index key %s: only 1 / -1 indexes are supported, got %v", e.Key, e.Value)
				}
				path := strings.Split(e.Key, ".")
				col := jsonbExpr(path)
				if isIDPath(path) {
					col = "id"
				}
				if dir == 1 {
					cols[i] = col + " ASC NULLS FIRST"
				} else {
					cols[i] = col + " DESC NULLS LAST"
				}
				nameParts[i] = fmt.Sprintf("%s_%d", e.Key, dir)
			}

			opts := options.IndexOptions{}
			if model.Options != nil {
				for _, set := range model.Options.List() {
					if err := set(&opts); err != nil {
						return err
					}
				}
			}
			name := strings.Join(nameParts, "_")
			if opts.Name != nil {
				name = *opts.Name
			}
			unique := ""
			if opts.Unique != nil && *opts.Unique {
				unique = "UNIQUE "
			}

			stmts = append(stmts, fmt.Sprintf("CREATE %sINDEX IF NOT EXISTS %s ON %s (%s)",
				unique, indexName(name), qualified, strings.Join(cols, ", ")))
		}
	}

	// --- Term search indexes -------------------------------------------------
	if sx, ok := any(zero).(SearchIndexed); ok {
		for _, spec := range sx.TermSearchIndexSpecs() {
			stmts = append(stmts, fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s USING gin (%s)",
				indexName(spec.Name), qualified, tsVectorExpr(spec.Paths)))
		}
	}

	// --- Vector indexes ------------------------------------------------------
	if vx, ok := any(zero).(VectorIndexed); ok {
		specs := vx.VectorIndexSpecs()
		needsExtension := false
		var vecStmts []string
		for _, spec := range specs {
			path := strings.Split(spec.Path, ".")
			if spec.Type == "filter" {
				vecStmts = append(vecStmts, fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (%s)",
					indexName(spec.Name), qualified, jsonbExpr(path)))
				continue
			}
			if spec.NumDimensions <= 0 {
				return fmt.Errorf("odm/postgres: vector index %s needs NumDimensions", spec.Name)
			}
			needsExtension = true
			vec := pgVectorFor(&spec, path, spec.NumDimensions)
			vecStmts = append(vecStmts, fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s USING hnsw (%s %s)",
				indexName(spec.Name), qualified, vec.expr, vec.opClass))
		}
		if needsExtension {
			stmts = append(stmts, "CREATE EXTENSION IF NOT EXISTS vector")
		}
		stmts = append(stmts, vecStmts...)
	}

	for _, stmt := range stmts {
		if _, err := client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("odm/postgres: %s: %w", stmt, err)
		}
	}
	return nil
}
