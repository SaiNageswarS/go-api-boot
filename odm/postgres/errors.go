package postgres

import (
	"errors"
	"fmt"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrPgSearchNotInstalled is returned (wrapped) when term search is used on
	// a database without ParadeDB's pg_search extension.
	ErrPgSearchNotInstalled = errors.New("term search needs ParadeDB's pg_search extension " +
		"(add pg_search to shared_preload_libraries and CREATE EXTENSION pg_search; see https://www.paradedb.com/docs)")

	// ErrVectorNotInstalled is returned (wrapped) when vector indexes are
	// declared on a database without the pgvector extension.
	ErrVectorNotInstalled = errors.New("vector search needs the pgvector extension " +
		"(CREATE EXTENSION vector; see https://github.com/pgvector/pgvector)")
)

// annotate prefixes a non-nil *err with the table it happened on:
// "odm/postgres: <tenant>.<table>: <cause>". Public functions defer it.
func annotate(err *error, tenant, table string) {
	if *err != nil {
		*err = fmt.Errorf("odm/postgres: %s.%s: %w", tenant, table, *err)
	}
}

// isMissing reports a query on a schema or table that doesn't exist.
func isMissing(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "42P01" || pgErr.Code == "3F000")
}

// writeErr explains the write failures callers can act on.
func writeErr(err error) error {
	var pgErr *pgconn.PgError
	switch {
	case err == nil:
		return nil
	case isMissing(err):
		return fmt.Errorf("the table doesn't exist; create it with postgres.EnsureTable: %w", err)
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		return fmt.Errorf("%w: %s", odm.ErrDuplicateKey, pgErr.Message)
	}
	return err
}

// queryErr adds the SQL to an error from running it: most such errors come
// from the conditions and orderings callers wrote.
func queryErr(err error, sql string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w (in %s)", err, sql)
}
