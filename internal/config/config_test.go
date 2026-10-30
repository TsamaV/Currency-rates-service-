package config

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		for _, key := range []string{"HTTP_ADDR", "DATABASE_URL", "PROVIDER_URL", "WORKERS", "ALLOWED_CURRENCIES"} {
			t.Setenv(key, "")
		}
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.HTTPAddr != ":8080" {
			t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
		}
		if !strings.HasPrefix(cfg.DatabaseURL, "postgres://") {
			t.Errorf("DatabaseURL = %q, want postgres:// prefix", cfg.DatabaseURL)
		}
		if cfg.Workers != 2 {
			t.Errorf("Workers = %d, want 2", cfg.Workers)
		}
		if cfg.ProviderTimeout <= 0 {
			t.Errorf("ProviderTimeout = %v, want positive", cfg.ProviderTimeout)
		}
		if got := strings.Join(cfg.AllowedCurrencies, ","); got != "USD,EUR,MXN" {
			t.Errorf("AllowedCurrencies = %q, want USD,EUR,MXN", got)
		}
	})

	t.Run("overrides from environment", func(t *testing.T) {
		t.Setenv("HTTP_ADDR", ":9000")
		t.Setenv("WORKERS", "8")
		t.Setenv("ALLOWED_CURRENCIES", "usd, eur")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.HTTPAddr != ":9000" {
			t.Errorf("HTTPAddr = %q, want :9000", cfg.HTTPAddr)
		}
		if cfg.Workers != 8 {
			t.Errorf("Workers = %d, want 8", cfg.Workers)
		}
		if got := strings.Join(cfg.AllowedCurrencies, ","); got != "USD,EUR" {
			t.Errorf("AllowedCurrencies = %q, want USD,EUR (normalized)", got)
		}
	})

	t.Run("invalid numeric falls back to default", func(t *testing.T) {
		t.Setenv("WORKERS", "not-a-number")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.Workers != 2 {
			t.Errorf("Workers = %d, want fallback 2", cfg.Workers)
		}
	})
}
