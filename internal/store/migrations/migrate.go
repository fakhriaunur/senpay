// Package migrations provides embedded SQL migration files and a runner
// for applying and rolling back database schema changes.
package migrations

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed *.sql
var migrationFiles embed.FS

// migration represents a single migration with up and down SQL statements.
type migration struct {
	number int
	upSQL  string
	downSQL string
}

// loadMigrations reads all embedded SQL files and groups them into migrations.
func loadMigrations() ([]migration, error) {
	entries, err := migrationFiles.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	files := make(map[int]map[string]string) // number → dir → sql

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		// Parse filename: <number>_<description>.<direction>.sql
		// e.g. 001_initial.up.sql
		parts := strings.Split(name, "_")
		if len(parts) < 2 {
			continue
		}

		var number int
		if _, err := fmt.Sscanf(parts[0], "%d", &number); err != nil {
			continue
		}

		var dir string
		if strings.HasSuffix(name, ".up.sql") {
			dir = "up"
		} else if strings.HasSuffix(name, ".down.sql") {
			dir = "down"
		} else {
			continue
		}

		data, err := migrationFiles.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}

		if files[number] == nil {
			files[number] = make(map[string]string)
		}
		files[number][dir] = strings.TrimSpace(string(data))
	}

	// Sort by migration number and build slice.
	var nums []int
	for n := range files {
		nums = append(nums, n)
	}
	sort.Ints(nums)

	var migrations []migration
	for _, n := range nums {
		m := migration{number: n}
		if up, ok := files[n]["up"]; ok {
			m.upSQL = up
		}
		if down, ok := files[n]["down"]; ok {
			m.downSQL = down
		}
		if m.upSQL == "" && m.downSQL == "" {
			continue
		}
		migrations = append(migrations, m)
	}

	return migrations, nil
}

// Up applies all pending migrations.
func Up(ctx context.Context, pool *pgxpool.Pool) error {
	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	// Create migrations tracking table if not exists.
	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	for _, m := range migrations {
		if m.upSQL == "" {
			continue
		}

		// Check if already applied.
		var exists bool
		err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", m.number).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", m.number, err)
		}
		if exists {
			continue
		}

		// Apply migration in a transaction.
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for migration %d: %w", m.number, err)
		}

		if _, err := tx.Exec(ctx, m.upSQL); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %d: %w", m.number, err)
		}

		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", m.number); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %d: %w", m.number, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.number, err)
		}

		fmt.Printf("Applied migration %d\n", m.number)
	}

	return nil
}

// Down rolls back all migrations in reverse order.
func Down(ctx context.Context, pool *pgxpool.Pool) error {
	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	// Apply down migrations in reverse order.
	for i := len(migrations) - 1; i >= 0; i-- {
		m := migrations[i]
		if m.downSQL == "" {
			continue
		}

		// Check if applied.
		var exists bool
		err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", m.number).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", m.number, err)
		}
		if !exists {
			continue
		}

		// Roll back in a transaction.
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for rollback %d: %w", m.number, err)
		}

		if _, err := tx.Exec(ctx, m.downSQL); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("rollback migration %d: %w", m.number, err)
		}

		if _, err := tx.Exec(ctx, "DELETE FROM schema_migrations WHERE version = $1", m.number); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record rollback %d: %w", m.number, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit rollback %d: %w", m.number, err)
		}

		fmt.Printf("Rolled back migration %d\n", m.number)
	}

	return nil
}
