package odm

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// PostgresClient is the subset of *pgxpool.Pool the Postgres ODM uses.
//
// The Postgres backend stores every collection as a table of JSON documents:
//
//	"<tenant>"."<collection>" (id TEXT PRIMARY KEY, doc JSONB NOT NULL)
//
// A tenant maps to a Postgres schema, the way it maps to a database in MongoDB.
// Documents are encoded with the model's bson tags, so the same model type works
// against both backends.
type PostgresClient interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
	Ping(ctx context.Context) error
	Close()
}

var postgresConnect = func(ctx context.Context, uri string) (PostgresClient, error) {
	return pgxpool.New(ctx, uri)
}

// ProvidePostgresClient connects to the database in POSTGRES_URI.
func ProvidePostgresClient() PostgresClient {
	uri := os.Getenv("POSTGRES_URI")
	if uri == "" {
		logger.Fatal("POSTGRES_URI environment variable is not set")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := postgresConnect(ctx, uri)
	if err != nil {
		logger.Fatal("Failed to connect to Postgres", zap.Error(err))
		return nil
	}

	if err := client.Ping(ctx); err != nil {
		logger.Fatal("Failed to ping Postgres", zap.Error(err))
		return nil
	}

	return client
}

// ensuredTables remembers tables already created by this process, so the
// CREATE ... IF NOT EXISTS round trip runs once per table, not once per call.
var ensuredTables sync.Map

// ensurePgTable creates the tenant schema and the collection table if missing.
// MongoDB creates collections on first write; this keeps that behaviour.
func ensurePgTable(ctx context.Context, db PostgresClient, schema, table string) error {
	key := fmt.Sprintf("%p|%s.%s", db, schema, table)
	if _, ok := ensuredTables.Load(key); ok {
		return nil
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Concurrent CREATE ... IF NOT EXISTS can still collide in the catalog;
	// serialise them with an advisory lock.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", "odm:"+schema+"."+table); err != nil {
		return err
	}

	qualified := pgx.Identifier{schema, table}.Sanitize()
	stmts := []string{
		"CREATE SCHEMA IF NOT EXISTS " + pgx.Identifier{schema}.Sanitize(),
		"CREATE TABLE IF NOT EXISTS " + qualified + " (id TEXT PRIMARY KEY, doc JSONB NOT NULL)",
		// Serves equality filters on any field (they compile to doc @> ...).
		"CREATE INDEX IF NOT EXISTS " + pgx.Identifier{table + "_doc_gin"}.Sanitize() +
			" ON " + qualified + " USING gin (doc jsonb_path_ops)",
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	ensuredTables.Store(key, struct{}{})
	return nil
}
