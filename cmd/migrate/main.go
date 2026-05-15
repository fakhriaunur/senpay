// Command migrate applies or rolls back database migrations.
//
// Usage:
//	go run ./cmd/migrate up    — apply all pending migrations
//	go run ./cmd/migrate down  — roll back all migrations
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"senpay/internal/store/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run ./cmd/migrate <up|down>")
		os.Exit(1)
	}

	direction := os.Args[1]
	if direction != "up" && direction != "down" {
		fmt.Printf("unknown direction: %s (use 'up' or 'down')\n", direction)
		os.Exit(1)
	}

	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "*********://senpay:senpay_dev@localhost:5432/senpay?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	switch direction {
	case "up":
		if err := migrations.Up(ctx, pool); err != nil {
			log.Fatalf("migration up failed: %v", err)
		}
		fmt.Println("All migrations applied successfully.")
	case "down":
		if err := migrations.Down(ctx, pool); err != nil {
			log.Fatalf("migration down failed: %v", err)
		}
		fmt.Println("All migrations rolled back successfully.")
	}
}
