package service

import (
	"context"
	"fmt"
	"log/slog"
	"plata/internal/model"
	"time"

	"github.com/google/uuid"
)

type RateRepository interface {
	CreatePendingUpdate(ctx context.Context, upd model.RateUpdate) (model.RateUpdate, error)
	GetUpdate(ctx context.Context, id string) (model.RateUpdate, error)
	CompleteUpdateWithRate(ctx context.Context, id string, price float64, completedAt time.Time, rate model.Rate) error
	ClaimPendingUpdates(ctx context.Context, limit int, now time.Time) ([]model.RateUpdate, error)
	FailUpdate(ctx context.Context, id string, errMsg string, now time.Time) error
	RequeueUpdate(ctx context.Context, id string, now time.Time) error
	GetLatestRate(ctx context.Context, pair model.CurrencyPair) (model.Rate, error)
	ResetStaleUpdates(ctx context.Context, staleAfter time.Duration, now time.Time) (int64, error)
}

type RateProvider interface {
	FetchRate(ctx context.Context, pair model.CurrencyPair) (model.Rate, error)
}

type Config struct {
	Workers           int
	FetchAttempts     int
	FetchRetryPause   time.Duration
	FinishTimeout     time.Duration
	PollInterval      time.Duration
	RecoveryInterval  time.Duration
	StaleAfter        time.Duration
	AllowedCurrencies []string
}

type RateService struct {
	repo     RateRepository
	provider RateProvider
	wake     chan struct{}
	allowed  map[string]struct{}
	cfg      Config
	log      *slog.Logger
}

func DefaultConfig() Config {
	return Config{
		Workers:           2,
		FetchAttempts:     3,
		FetchRetryPause:   500 * time.Millisecond,
		FinishTimeout:     5 * time.Second,
		PollInterval:      5 * time.Second,
		RecoveryInterval:  30 * time.Second,
		StaleAfter:        5 * time.Minute,
		AllowedCurrencies: []string{"USD", "EUR", "MXN"},
	}
}

func NewRateService(repo RateRepository, provider RateProvider, cfg Config, log *slog.Logger) (*RateService, error) {
	allowed := make(map[string]struct{})
	for _, c := range cfg.AllowedCurrencies {
		allowed[c] = struct{}{}
	}

	if cfg.Workers < 1 || cfg.FetchAttempts < 1 {
		return nil, fmt.Errorf("invalid service config: workers=%d fetch_attempts=%d", cfg.Workers, cfg.FetchAttempts)
	}

	return &RateService{
		repo:     repo,
		provider: provider,
		wake:     make(chan struct{}, 1),
		allowed:  allowed,
		cfg:      cfg,
		log:      log,
	}, nil
}

func (s *RateService) pairAllowed(pair model.CurrencyPair) bool {
	_, baseOK := s.allowed[pair.Base]
	_, quoteOK := s.allowed[pair.Quote]

	return baseOK && quoteOK
}

func (s *RateService) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *RateService) RequestUpdate(ctx context.Context, pair model.CurrencyPair) (model.RateUpdate, error) {
	if !s.pairAllowed(pair) {
		return model.RateUpdate{}, model.ErrUnsupportedPair
	}

	now := time.Now()
	newUpdate := model.RateUpdate{
		ID:          uuid.NewString(),
		Pair:        pair,
		Status:      model.StatusPending,
		RequestedAt: now,
		UpdatedAt:   now,
	}

	created, err := s.repo.CreatePendingUpdate(ctx, newUpdate)
	if err != nil {
		return model.RateUpdate{}, fmt.Errorf("create pending update: %w", err)
	}

	if created.ID == newUpdate.ID {
		s.notify()
	}

	return created, nil
}

func (s *RateService) GetUpdate(ctx context.Context, id string) (model.RateUpdate, error) {
	return s.repo.GetUpdate(ctx, id)
}

func (s *RateService) GetLatestRate(ctx context.Context, pair model.CurrencyPair) (model.Rate, error) {
	return s.repo.GetLatestRate(ctx, pair)
}
