// Package postgres is the Postgres backend of odm.
//
//	db, err := postgres.Open(ctx, os.Getenv("POSTGRES_URI"))
//	err = postgres.EnsureCollection[Movie](ctx, db, tenant) // at deploy / startup
//	movies := postgres.CollectionOf[Movie](db, tenant)
//	movie, err := async.Await(movies.FindByID(ctx, id))
//
// Storage: a tenant is a schema; a collection is a table
// "<tenant>"."<collection>" (id TEXT PRIMARY KEY, doc JSONB) holding the
// model's bson encoding. Filters in MongoDB syntax are compiled to SQL over doc.
// Vector search uses pgvector; term search uses ParadeDB's pg_search (BM25).
//
// Collections never change the schema at runtime: EnsureCollection creates
// tables and indexes. Reads of a collection that doesn't exist find nothing;
// writes to it fail.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool is the subset of *pgxpool.Pool the ODM uses.
type Pool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

// DB is a Postgres connection pool plus the ODM's settings.
type DB struct {
	pool    Pool
	close   func()
	timer   odm.Timer
	stemmer string

	pgSearchOK atomic.Bool // pg_search confirmed installed

	vectorOnce      sync.Once
	vectorIterative bool // pgvector >= 0.8 (iterative index scans)
}

// Option configures a DB.
type Option func(*DB)

// WithTimer sets the clock used for createdOn / updatedOn.
func WithTimer(t odm.Timer) Option { return func(db *DB) { db.timer = t } }

// WithStemmer sets the term search stemmer language (default "english");
// "" disables stemming. It applies to indexes EnsureCollection creates.
func WithStemmer(language string) Option { return func(db *DB) { db.stemmer = language } }

// Open connects to uri and pings the server.
func Open(ctx context.Context, uri string, opts ...Option) (*DB, error) {
	if uri == "" {
		return nil, errors.New("odm/postgres: empty connection uri")
	}
	pool, err := pgxpool.New(ctx, uri)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	db := New(pool, opts...)
	db.close = pool.Close
	return db, nil
}

// OpenFromEnv connects to the uri in POSTGRES_URI.
func OpenFromEnv(ctx context.Context, opts ...Option) (*DB, error) {
	return Open(ctx, os.Getenv("POSTGRES_URI"), opts...)
}

// New wraps an existing pool. Close does not close it.
func New(pool Pool, opts ...Option) *DB {
	db := &DB{pool: pool, timer: odm.SystemTimer{}, stemmer: "english"}
	for _, opt := range opts {
		opt(db)
	}
	return db
}

// Pool returns the underlying pool, for SQL the ODM doesn't cover.
func (db *DB) Pool() Pool { return db.pool }

// Close closes a pool opened by Open.
func (db *DB) Close() {
	if db.close != nil {
		db.close()
	}
}

var (
	// ErrPgSearchNotInstalled is returned when term search is used on a
	// database without ParadeDB's pg_search extension.
	ErrPgSearchNotInstalled = errors.New("odm/postgres: term search needs ParadeDB's pg_search extension " +
		"(add pg_search to shared_preload_libraries and CREATE EXTENSION pg_search; see https://www.paradedb.com/docs)")

	// ErrVectorNotInstalled is returned when vector indexes are declared on a
	// database without the pgvector extension.
	ErrVectorNotInstalled = errors.New("odm/postgres: vector search needs the pgvector extension " +
		"(CREATE EXTENSION vector; see https://github.com/pgvector/pgvector)")
)

func (db *DB) requirePgSearch(ctx context.Context) error {
	if db.pgSearchOK.Load() {
		return nil
	}
	ok, err := db.hasExtension(ctx, "pg_search")
	if err != nil {
		return err
	}
	if !ok {
		return ErrPgSearchNotInstalled
	}
	db.pgSearchOK.Store(true)
	return nil
}

func (db *DB) hasExtension(ctx context.Context, name string) (bool, error) {
	var ok bool
	err := db.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = $1)", name).Scan(&ok)
	return ok, err
}

// installExtension creates an extension if the server has it available.
func (db *DB) installExtension(ctx context.Context, name string, missing error) error {
	if ok, err := db.hasExtension(ctx, name); err != nil || ok {
		return err
	}
	// Fails when the server doesn't ship it (or, for pg_search, doesn't preload it).
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

// iterativeScans reports whether pgvector supports hnsw.iterative_scan (0.8+),
// which keeps filtered vector searches from returning fewer than K hits.
func (db *DB) iterativeScans(ctx context.Context) bool {
	db.vectorOnce.Do(func() {
		var version string
		if err := db.pool.QueryRow(ctx, "SELECT extversion FROM pg_extension WHERE extname = 'vector'").Scan(&version); err != nil {
			return
		}
		major, minor, _ := strings.Cut(version, ".")
		minor, _, _ = strings.Cut(minor, ".")
		ma, _ := strconv.Atoi(major)
		mi, _ := strconv.Atoi(minor)
		db.vectorIterative = ma > 0 || mi >= 8
	})
	return db.vectorIterative
}

/* ── errors ── */

// isMissing reports a query on a schema or table that doesn't exist.
func isMissing(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "42P01" || pgErr.Code == "3F000")
}

// writeErr explains write failures callers can act on.
func writeErr(err error, schema, table string) error {
	var pgErr *pgconn.PgError
	switch {
	case err == nil:
		return nil
	case isMissing(err):
		return fmt.Errorf("odm/postgres: collection %s.%s doesn't exist; create it with postgres.EnsureCollection: %w", schema, table, err)
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		return fmt.Errorf("%w: %s", odm.ErrDuplicateKey, pgErr.Message)
	}
	return err
}
