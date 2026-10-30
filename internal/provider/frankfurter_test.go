package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"plata/internal/model"
)

func TestFrankfurter_FetchRate(t *testing.T) {
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got %s", r.Method)
		}

		if r.URL.Path != "/latest" {
			t.Errorf("expected path /latest, got %s", r.URL.Path)
		}

		if got := r.URL.Query().Get("base"); got != "EUR" {
			t.Errorf("expected base EUR, got %s", got)
		}

		if got := r.URL.Query().Get("symbols"); got != "MXN" {
			t.Errorf("expected symbols MXN, got %s", got)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"base":"EUR","date":"2026-10-23","rates":{"MXN":21.02}}`)
	}))
	defer srv.Close()

	p := NewFrankfurter(srv.URL, time.Second)

	rate, err := p.FetchRate(ctx, model.CurrencyPair{
		Base:  "EUR",
		Quote: "MXN",
	})
	if err != nil {
		t.Fatalf("FetchRate returned error: %v", err)
	}

	if rate.Pair.Base != "EUR" {
		t.Errorf("expected base EUR, got %s", rate.Pair.Base)
	}

	if rate.Pair.Quote != "MXN" {
		t.Errorf("expected quote MXN, got %s", rate.Pair.Quote)
	}

	if rate.Price != 21.02 {
		t.Errorf("expected price 21.02, got %f", rate.Price)
	}

	if rate.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt to be set, got zero value")
	}

	t.Run("HTTP 500", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		p := NewFrankfurter(srv.URL, time.Second)

		rate, err := p.FetchRate(ctx, model.CurrencyPair{
			Base:  "EUR",
			Quote: "MXN",
		})
		if err == nil {
			t.Fatal("expected error, got rate", rate)
		}

		if !strings.Contains(err.Error(), "status 500") {
			t.Errorf("expected error to contain %q, got %v", "status 500", err)
		}
	})

	t.Run("empty rates", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"base":"EUR","date":"2026-10-23","rates":{}}`)
		}))
		defer srv.Close()

		p := NewFrankfurter(srv.URL, time.Second)

		rate, err := p.FetchRate(ctx, model.CurrencyPair{
			Base:  "EUR",
			Quote: "MXN",
		})
		if err == nil {
			t.Fatal("expected error, got rate", rate)
		}

		if !strings.Contains(err.Error(), "no rate") {
			t.Errorf("expected error to contain %q, got %v", "no rate", err)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{not json`)
		}))
		defer srv.Close()

		p := NewFrankfurter(srv.URL, time.Second)

		rate, err := p.FetchRate(ctx, model.CurrencyPair{
			Base:  "EUR",
			Quote: "MXN",
		})
		if err == nil {
			t.Fatal("expected error, got rate", rate)
		}
	})
}
