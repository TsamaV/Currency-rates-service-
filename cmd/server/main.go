package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"plata/internal/config"
	"plata/internal/database"
	"plata/internal/handler"
	"plata/internal/provider"
	"plata/internal/repository"
	"plata/internal/service"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	if err := run(log); err != nil {
		log.Error("service stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	svcCfg := service.DefaultConfig()
	svcCfg.Workers = cfg.Workers
	svcCfg.AllowedCurrencies = cfg.AllowedCurrencies

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	repo := repository.NewRepository(pool)
	frankfurter := provider.NewFrankfurter(cfg.ProviderURL, cfg.ProviderTimeout)
	svc, err := service.NewRateService(repo, frankfurter, svcCfg, log)
	if err != nil {
		return err
	}

	h := handler.NewHandler(svc, log)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return svc.Run(ctx)
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           h.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	g.Go(func() error {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	})

	log.Info("service started", "addr", cfg.HTTPAddr, "provider", cfg.ProviderURL)
	if err := g.Wait(); err != nil {
		return err
	}
	log.Info("service stopped")
	return nil
}
