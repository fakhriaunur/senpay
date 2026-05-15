# Senpay task runner
# See https://just.systems/man/en/

# Build all binaries
build:
    go build ./cmd/server
    go build ./cmd/tui

# Run all tests (core only, no integration)
test:
    go test ./internal/... -count=1 -timeout 60s -short

# Run linter
lint:
    go vet ./...
    golangci-lint run ./...

# Run the server
run-server:
    go run ./cmd/server

# Run the TUI
run-tui:
    go run ./cmd/tui

# Start Docker services
db-up:
    docker compose up -d

# Stop Docker services (also rolls back migrations)
db-down:
    go run ./cmd/migrate down
    docker compose down

# Run database migrations up
db-migrate:
    go run ./cmd/migrate up
