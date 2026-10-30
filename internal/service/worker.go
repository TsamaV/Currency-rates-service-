package service

import (
	"context"
	"errors"
	"fmt"
	"plata/internal/model"
	"sync"
	"time"
)

const claimBatchSize = 1

func (s *RateService) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	for range s.cfg.Workers {
		wg.Go(func() {
			s.worker(ctx)
		})
	}

	wg.Go(func() {
		s.recoverLoop(ctx)
	})

	s.notify()

	wg.Wait()

	return nil
}

func (s *RateService) worker(ctx context.Context) {
	poll := time.NewTicker(s.cfg.PollInterval)
	defer poll.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-poll.C:
		}
		s.claimLoop(ctx)
	}
}

func (s *RateService) fetchWithRetry(ctx context.Context, pair model.CurrencyPair) (model.Rate, error) {
	var lastErr error

	for attempt := 1; attempt <= s.cfg.FetchAttempts; attempt++ {
		rate, err := s.provider.FetchRate(ctx, pair)
		if err == nil {
			return rate, nil
		}

		lastErr = err

		s.log.Warn("fetch attempt failed", "pair", pair.String(), "attempt", attempt, "err", err)

		if attempt == s.cfg.FetchAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return model.Rate{}, ctx.Err()
		case <-time.After(s.cfg.FetchRetryPause):
		}
	}

	if lastErr == nil {
		return model.Rate{}, fmt.Errorf("fetch %s: no attempts configured", pair.String())
	}

	return model.Rate{}, fmt.Errorf("fetch %s after %d attempts: %w", pair.String(), s.cfg.FetchAttempts, lastErr)
}

func (s *RateService) process(ctx context.Context, upd model.RateUpdate) {
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Error("panic in process recovered", "err", rec)
		}
	}()

	rate, err := s.fetchWithRetry(ctx, upd.Pair)

	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.cfg.FinishTimeout)
	defer cancel()

	if err != nil {
		if errors.Is(err, context.Canceled) {
			if rqErr := s.repo.RequeueUpdate(finishCtx, upd.ID, time.Now()); rqErr != nil {
				s.log.Error("requeue update", "id", upd.ID, "err", rqErr)
			}
			return
		}
		if failErr := s.repo.FailUpdate(finishCtx, upd.ID, err.Error(), time.Now()); failErr != nil {
			s.log.Error("failed to mark update failed", "id", upd.ID, "err", failErr)
		}
		return
	}

	if err := s.repo.CompleteUpdateWithRate(finishCtx, upd.ID, rate.Price, rate.UpdatedAt, rate); err != nil {
		s.log.Error("failed to complete and update", "id", upd.ID, "err", err)
	}
}

func (s *RateService) recoverOnce(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Error("panic in recovery recovered", "err", rec)
		}
	}()

	reset, err := s.repo.ResetStaleUpdates(ctx, s.cfg.StaleAfter, time.Now())
	if err != nil {
		s.log.Error("reset stale updates", "err", err)
	}

	if reset > 0 {
		s.log.Warn("reset stale in-progress updates", "count", reset)
		s.notify()
	}
}

func (s *RateService) claimLoop(ctx context.Context) {
	for {
		claims, err := s.repo.ClaimPendingUpdates(ctx, claimBatchSize, time.Now())
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				s.log.Error("claim pending updates", "err", err)
			}
			return
		}
		if len(claims) == 0 {
			return
		}
		for _, upd := range claims {
			s.process(ctx, upd)
		}
	}
}

func (s *RateService) recoverLoop(ctx context.Context) {
	s.recoverOnce(ctx)

	ticker := time.NewTicker(s.cfg.RecoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.recoverOnce(ctx)
		}
	}
}
