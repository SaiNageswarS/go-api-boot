package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
)

// extensions caches what the database supports. Only successful probes are
// cached, so a failed probe (say, a cancelled context) is retried next time.
type extensions struct {
	pgSearch  atomic.Bool  // pg_search is installed
	iterative atomic.Int32 // pgvector iterative scans: 0 unknown, 1 no, 2 yes
}

func (db *DB) hasExtension(ctx context.Context, name string) (bool, error) {
	var ok bool
	err := db.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = $1)", name).Scan(&ok)
	return ok, err
}

// installExtension creates an extension if the server has it available, and
// returns missing if it doesn't.
func (db *DB) installExtension(ctx context.Context, name string, missing error) error {
	if ok, err := db.hasExtension(ctx, name); err != nil || ok {
		return err
	}
	// Fails when the server doesn't ship it (or, for pg_search, doesn't preload
	// it); the check below reports that.
	_, _ = db.pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+pgx.Identifier{name}.Sanitize()+" CASCADE")
	ok, err := db.hasExtension(ctx, name)
	if err != nil {
		return err
	}
	if !ok {
		return missing
	}
	return nil
}

// requirePgSearch returns ErrPgSearchNotInstalled unless pg_search is
// installed.
func (db *DB) requirePgSearch(ctx context.Context) error {
	if db.ext.pgSearch.Load() {
		return nil
	}
	ok, err := db.hasExtension(ctx, "pg_search")
	if err != nil {
		return err
	}
	if !ok {
		return ErrPgSearchNotInstalled
	}
	db.ext.pgSearch.Store(true)
	return nil
}

// iterativeScans reports whether pgvector supports hnsw.iterative_scan (0.8+),
// which keeps filtered vector searches from returning fewer than k rows.
func (db *DB) iterativeScans(ctx context.Context) (bool, error) {
	switch db.ext.iterative.Load() {
	case 1:
		return false, nil
	case 2:
		return true, nil
	}
	var version string
	if err := db.pool.QueryRow(ctx, "SELECT extversion FROM pg_extension WHERE extname = 'vector'").Scan(&version); err != nil {
		return false, fmt.Errorf("reading the pgvector version: %w", err)
	}
	if !pgvectorAtLeast(version, 0, 8) {
		db.ext.iterative.Store(1)
		return false, nil
	}
	db.ext.iterative.Store(2)
	return true, nil
}

// pgvectorAtLeast compares a "major.minor[.patch]" version.
func pgvectorAtLeast(version string, major, minor int) bool {
	maStr, rest, _ := strings.Cut(version, ".")
	miStr, _, _ := strings.Cut(rest, ".")
	ma, _ := strconv.Atoi(maStr)
	mi, _ := strconv.Atoi(miStr)
	return ma > major || (ma == major && mi >= minor)
}
