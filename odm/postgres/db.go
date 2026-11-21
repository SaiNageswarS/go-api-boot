package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is a Postgres connection pool plus the ORM's settings.
type DB struct {
	pool    pool          // what the ORM runs queries on
	raw     *pgxpool.Pool // returned by Pool
	owned   bool          // opened by Open, so closed by Close
	timer   odm.Timer
	stemmer string

	ext extensions
}

// pool is the part of *pgxpool.Pool the ORM uses; tests substitute it.
type pool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Option configures a DB.
type Option func(*DB)

// WithTimer sets the clock Save stamps created_on and updated_on from.
func WithTimer(t odm.Timer) Option { return func(db *DB) { db.timer = t } }

// WithStemmer sets the term search stemmer language (default "english");
// "" disables stemming. It applies to indexes EnsureTable creates.
func WithStemmer(language string) Option { return func(db *DB) { db.stemmer = language } }

// Open connects to uri and pings the server.
func Open(ctx context.Context, uri string, opts ...Option) (*DB, error) {
	if uri == "" {
		return nil, errors.New("odm/postgres: Open: empty connection uri")
	}
	p, err := pgxpool.New(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("odm/postgres: Open: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := p.Ping(pingCtx); err != nil {
		p.Close()
		return nil, fmt.Errorf("odm/postgres: Open: %w", err)
	}
	db := New(p, opts...)
	db.owned = true
	return db, nil
}

// OpenFromEnv connects to the uri in POSTGRES_URI.
func OpenFromEnv(ctx context.Context, opts ...Option) (*DB, error) {
	return Open(ctx, os.Getenv("POSTGRES_URI"), opts...)
}

// New wraps an existing pool. Close does not close it.
func New(p *pgxpool.Pool, opts ...Option) *DB {
	db := newDB(p, opts...)
	db.raw = p
	return db
}

func newDB(p pool, opts ...Option) *DB {
	db := &DB{pool: p, timer: odm.SystemTimer{}, stemmer: "english"}
	for _, opt := range opts {
		opt(db)
	}
	return db
}

// Pool returns the underlying pool, for SQL the ORM doesn't cover (extensions,
// migrations, COPY, batches, ...).
func (db *DB) Pool() *pgxpool.Pool { return db.raw }

// Close closes a pool opened by Open.
func (db *DB) Close() {
	if db.owned {
		db.raw.Close()
	}
}
