package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"senpay/internal/auth"
	"senpay/internal/config"
	"senpay/internal/store/migrations"
	"senpay/internal/telemetry"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg := config.Load()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("starting senpay server", "port", cfg.Port)

	// Connect to PostgreSQL.
	ctx := context.Background()
	pool, err := connectDB(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Run migrations.
	if err := migrations.Up(ctx, pool); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}

	// Initialize auth handler.
	userStore := auth.NewPostgresUserStore(pool)
	authHandler := auth.NewHandler(pool, userStore, cfg.JWTSecret)

	// Apply auth middleware.
	authMiddleware := auth.AuthMiddleware(cfg.JWTSecret)

	metrics := telemetry.NewMetrics()

	mux := http.NewServeMux()

	// Health and metrics (no auth required).
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	mux.Handle("GET /metrics", metrics.MetricsHandler())

	// Auth endpoints (register and login are public; refresh uses its own token validation).
	mux.HandleFunc("POST /v1/auth/register", authHandler.Register)
	mux.HandleFunc("POST /v1/auth/login", authHandler.Login)
	mux.HandleFunc("POST /v1/auth/refresh", authHandler.Refresh)

	// Protected auth endpoints (require valid Bearer token).
	mux.Handle("POST /v1/auth/kyc", authMiddleware(http.HandlerFunc(authHandler.KYC)))
	mux.Handle("GET /v1/auth/me", authMiddleware(http.HandlerFunc(authHandler.Me)))
	mux.Handle("GET /v1/balance", authMiddleware(http.HandlerFunc(authHandler.Balance)))

	// Wrap mux with telemetry middleware.
	handler := metrics.Middleware(mux)

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server listening", "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

// connectDB establishes a connection pool to PostgreSQL.
func connectDB(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	slog.Info("connected to database")
	return pool, nil
}
