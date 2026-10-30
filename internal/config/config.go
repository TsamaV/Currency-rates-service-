package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL       string
	HTTPAddr          string
	ProviderURL       string
	ProviderTimeout   time.Duration
	Workers           int
	AllowedCurrencies []string
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load .env: %w", err)
	}

	return &Config{
		HTTPAddr:          envOr("HTTP_ADDR", ":8080"),
		DatabaseURL:       envOr("DATABASE_URL", "postgres://plata:dev@localhost:5433/plata"),
		ProviderURL:       envOr("PROVIDER_URL", "https://api.frankfurter.dev/v1"),
		ProviderTimeout:   5 * time.Second,
		Workers:           envInt("WORKERS", 2),
		AllowedCurrencies: envCSV("ALLOWED_CURRENCIES", "USD,EUR,MXN"),
	}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}

	return fallback
}

func envCSV(key, fallback string) []string {
	raw := envOr(key, fallback)
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))

	for _, p := range parts {
		if p = strings.ToUpper(strings.TrimSpace(p)); p != "" {
			out = append(out, p)
		}
	}

	return out
}
