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
	"senpay/internal/bank"
	"senpay/internal/config"
	"senpay/internal/gateway"
	"senpay/internal/idempotency"
	"senpay/internal/ledger"
	"senpay/internal/nats"
	"senpay/internal/store/migrations"
	"senpay/internal/telemetry"
	"senpay/internal/transfer"
	"senpay/internal/transactions"
	"senpay/internal/wallet"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
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

	// Initialize gateway middleware.
	rateLimiter := gateway.DefaultRateLimiter()
	biLimiter := gateway.BILimit(userStore)

	// Initialize Redis client.
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		slog.Error("invalid redis URL", "error", err)
		os.Exit(1)
	}
	redisClient := redis.NewClient(redisOpts)
	defer redisClient.Close()

	// Verify Redis connectivity.
	if err := redisClient.Ping(ctx).Err(); err != nil {
		slog.Error("redis ping failed", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to redis")

	// Initialize NATS client.
	natsClient, err := nats.Connect(cfg.NatsURL)
	if err != nil {
		slog.Error("nats connection failed", "error", err)
		os.Exit(1)
	}
	defer natsClient.Close()
	slog.Info("connected to nats")

	// Initialize transfer service and handler.
	redisCache := idempotency.NewRedisIdempotencyCache(redisClient)
	transferSvc := transfer.NewService(pool, redisCache, natsClient, userStore)
	transferHandler := transfer.NewHandler(transferSvc)

	// Initialize wallet handler (balance projection from tx_log).
	walletHandler := wallet.NewHandler(pool)

	// Initialize transaction history handler (ledger store + counterparty lookup).
	txLogStore := ledger.NewPostgresTxLogStore(pool)
	txHandler := transactions.NewHandler(txLogStore, userStore)

	// ── Bank / SNAP / Top-up ──────────────────────────────────

	// Initialize mock bank server (in-process).
	mockBankConfig := bank.DefaultMockBankConfig()
	mockBankConfig.WebhookURL = fmt.Sprintf("http://127.0.0.1:%d/bank/webhook", cfg.Port)
	mockBank := bank.NewMockBank(mockBankConfig)

	// Initialize bank adapter (PaymentRail).
	var paymentRail bank.PaymentRail
	if cfg.BankProvider == "snap" {
		// Real SNAP adapter contacts the mock bank via HTTP.
		baseURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.Port)
		paymentRail = bank.NewSnapAdapter(baseURL, mockBankConfig.ClientSecret,
			mockBankConfig.PartnerID, mockBankConfig.ChannelID)
	} else {
		// Stub adapter — no network calls, returns canned responses.
		paymentRail = bank.NewStubAdapter()
	}

	// Initialize VA store (PostgreSQL).
	vaStore := bank.NewPostgresVAStore(pool)

	// Initialize bank service orchestrator.
	bankSvc := bank.NewService(pool, vaStore, redisCache, paymentRail, natsClient, userStore)

	// Initialize bank HTTP handlers.
	bankHandler := bank.NewHandler(bankSvc)
	bankWebhook := bank.NewWebhookHandler(bankSvc)

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

	// Wallet endpoint (protected, balance projected from tx_log).
	mux.Handle("GET /v1/wallet/balance", authMiddleware(http.HandlerFunc(walletHandler.Balance)))

	// Transaction history endpoints (protected).
	mux.Handle("GET /v1/transactions", authMiddleware(http.HandlerFunc(txHandler.List)))
	mux.Handle("GET /v1/transactions/{id}", authMiddleware(http.HandlerFunc(txHandler.Detail)))

	// Transfer endpoint (protected + BI limit enforced).
	mux.Handle("POST /v1/transfer", authMiddleware(biLimiter(http.HandlerFunc(transferHandler.Transfer))))

	// Bank / SNAP endpoints.
	// Top-up endpoint (protected + BI limit enforced).
	mux.Handle("POST /v1/topup", authMiddleware(biLimiter(http.HandlerFunc(bankHandler.Topup))))

	// Mock bank endpoints (no auth — mock bank validates SNAP headers internally).
	mh := mockBank.Handler()
	mux.Handle("GET /bank/health", mh)
	mux.Handle("POST /bank/api/v1/credit", mh)
	mux.Handle("POST /bank/api/v1/withdraw", mh)
	mux.Handle("POST /bank/api/v1/reversal", mh)

	// Bank webhook callback endpoint (no auth — called by mock bank internally).
	mux.HandleFunc("POST /bank/webhook", bankWebhook.HandleWebhook)

	// Apply global gateway middleware stack (outermost to innermost).
	handler := gateway.Recovery(mux)
	handler = gateway.RequestID(handler)
	handler = gateway.Logging(handler)
	handler = gateway.RateLimit(rateLimiter)(handler)
	handler = metrics.Middleware(handler)

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
