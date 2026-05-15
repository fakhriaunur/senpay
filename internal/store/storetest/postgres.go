//go:build integration

// Package storetest provides test helpers for PostgreSQL adapter contract tests.
// All files in this package are only compiled with the "integration" build tag.
package storetest

import (
	"context"
	"fmt"
	"os"

	"senpay/internal/store/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewTestPool connects to PostgreSQL (via TEST_DATABASE_URL env var or DATABASE_URL),
// drops existing tables, runs migrations, and returns a connection pool.
//
// The TEST_DATABASE_URL env var takes precedence over DATABASE_URL.
// Set either to the docker-compose default or your own instance.
// The cleanup function drops all tables and closes the pool.
func NewTestPool(ctx context.Context) (pool *pgxpool.Pool, cleanup func(), err error) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		return nil, nil, fmt.Errorf("TEST_DATABASE_URL or DATABASE_URL must be set")
	}

	pool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to postgres: %w", err)
	}

	// Drop all existing tables to start clean.
	dropAllTables(ctx, pool)

	// Run up migrations.
	if err := migrations.Up(ctx, pool); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("run migrations: %w", err)
	}

	cleanup = func() {
		dropAllTables(ctx, pool)
		pool.Close()
	}

	return pool, cleanup, nil
}

// dropAllTables drops all known tables from the database.
func dropAllTables(ctx context.Context, pool *pgxpool.Pool) {
	_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS schema_migrations CASCADE`)
	_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS balance_snapshot CASCADE`)
	_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS tx_log CASCADE`)
	_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS idempotency_keys CASCADE`)
	_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS users CASCADE`)
}
