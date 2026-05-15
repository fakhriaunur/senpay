// Copy .env.example to .env and fill in values.

package config

import (
	"os"
	"strconv"

	"senpay/internal/types"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// Server
	Port int

	// Database
	DatabaseURL string

	// Redis
	RedisURL string

	// NATS
	NatsURL string

	// Auth
	JWTSecret string

	// Feature flags
	SenpaiFullEnabled bool

	// Bank provider: "stub" or "snap"
	BankProvider types.BankProvider
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	return Config{
		Port:              getEnvInt("PORT", 8384),
		DatabaseURL:       getEnv("DATABASE_URL", ""),
		RedisURL:          getEnv("REDIS_URL", "redis://localhost:6379"),
		NatsURL:           getEnv("NATS_URL", "nats://localhost:4222"),
		JWTSecret:         getEnv("JWT_SECRET", ""),
		SenpaiFullEnabled: getEnvBool("SENPAI_FULL_ENABLED", false),
		BankProvider:      parseBankProvider(getEnv("BANK_PROVIDER", "stub")),
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

// parseBankProvider parses a bank provider string, defaulting to stub on invalid values.
func parseBankProvider(s string) types.BankProvider {
	p, err := types.ParseBankProvider(s)
	if err != nil {
		return types.BankProviderStub
	}
	return p
}
