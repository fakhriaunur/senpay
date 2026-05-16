// Copy .env.example to .env and fill in values.

package config

import (
	"os"
	"strconv"
	"time"

	"senpay/internal/types"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// Server
	Port int

	// HTTP server timeouts
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	// ShutdownTimeout is the grace period for server shutdown.
	ShutdownTimeout time.Duration

	// Database
	DatabaseURL string

	// Redis
	RedisURL string

	// NATS
	NatsURL string
	// NATSReconnectWait is the wait time between reconnect attempts.
	NATSReconnectWait time.Duration
	// NATSTimeout is the timeout for NATS operations.
	NATSTimeout time.Duration

	// Auth
	JWTSecret string
	// TokenStoreCleanupInterval is how often used refresh tokens are cleaned up.
	TokenStoreCleanupInterval time.Duration

	// Feature flags
	SenpaiFullEnabled bool

	// Bank provider: "stub" or "snap"
	BankProvider types.BankProvider
	// BankHTTPTimeout is the HTTP client timeout for bank adapter requests.
	BankHTTPTimeout time.Duration
	// WithdrawTimeout is the timeout for withdraw operations.
	WithdrawTimeout time.Duration
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	return Config{
		Port:              getEnvInt("PORT", 8384),
		ReadTimeout:       getEnvDuration("SERVER_READ_TIMEOUT", 10*time.Second),
		WriteTimeout:      getEnvDuration("SERVER_WRITE_TIMEOUT", 10*time.Second),
		IdleTimeout:       getEnvDuration("SERVER_IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout:   getEnvDuration("SERVER_SHUTDOWN_TIMEOUT", 5*time.Second),
		DatabaseURL:       getEnv("DATABASE_URL", ""),
		RedisURL:          getEnv("REDIS_URL", "redis://localhost:6379"),
		NatsURL:           getEnv("NATS_URL", "nats://localhost:4222"),
		NATSReconnectWait: getEnvDuration("NATS_RECONNECT_WAIT", 2*time.Second),
		NATSTimeout:       getEnvDuration("NATS_TIMEOUT", 5*time.Second),
		JWTSecret:         getEnv("JWT_SECRET", ""),
		TokenStoreCleanupInterval: getEnvDuration("TOKEN_CLEANUP_INTERVAL", 1*time.Hour),
		SenpaiFullEnabled:         getEnvBool("SENPAI_FULL_ENABLED", false),
		BankProvider:              parseBankProvider(getEnv("BANK_PROVIDER", "stub")),
		BankHTTPTimeout:           getEnvDuration("BANK_HTTP_TIMEOUT", 30*time.Second),
		WithdrawTimeout:           getEnvDuration("WITHDRAW_TIMEOUT", 30*time.Second),
	}
}

// getEnv returns the value of an environment variable or a default if not set.
func getEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}

// getEnvInt returns the integer value of an environment variable or a default if not set.
func getEnvInt(key string, defaultValue int) int {
	if value, ok := os.LookupEnv(key); ok {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getEnvBool returns the boolean value of an environment variable or a default if not set.
func getEnvBool(key string, defaultValue bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

// getEnvDuration returns the duration value of an environment variable or a default if not set.
// The env var value is parsed as a Go duration string (e.g., "30s", "5m", "100ms").
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

// parseBankProvider parses a bank provider string, defaulting to stub on invalid values.
func parseBankProvider(s string) types.BankProvider {
	p, err := types.ParseBankProvider(s)
	if err != nil {
		return types.BankProviderStub
	}
	return p
}
