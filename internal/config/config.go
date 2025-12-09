package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds application configuration
type Config struct {
	ServerAddr        string
	DatabaseURL       string
	RedisURL          string
	IngestionInterval time.Duration
	LogLevel          string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		ServerAddr:        getEnv("SERVER_ADDR", ":8080"),
		DatabaseURL:       getEnv("DATABASE_URL", "postgres://user:password@localhost/rpki_viz?sslmode=disable"),
		RedisURL:          getEnv("REDIS_URL", "redis://localhost:6379"),
		IngestionInterval: getEnvDuration("INGESTION_INTERVAL", 15*time.Minute),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
	}

	return cfg, nil
}

// getEnv gets an environment variable with a fallback value
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// getEnvDuration gets an environment variable as a duration with a fallback value
func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return fallback
}

// getEnvInt gets an environment variable as an integer with a fallback value
func getEnvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}
