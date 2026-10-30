package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"plata/internal/model"
)

type fakeRepo struct {
	createResult model.RateUpdate
	createCalls  int

	claims     []model.RateUpdate
	claimCalls int

	resetCount int64

	completeCalls int
	completedWith float64
	failCalls     int
	failMsg       string
	requeueCalls  int
}

func (f *fakeRepo) CreatePendingUpdate(_ context.Context, upd model.RateUpdate) (model.RateUpdate, error) {
	f.createCalls++
	if f.createResult.ID != "" {
		upd.ID = f.createResult.ID
	}
	return upd, nil
}

func (f *fakeRepo) GetUpdate(_ context.Context, _ string) (model.RateUpdate, error) {
	return model.RateUpdate{}, model.ErrNotFound
}

func (f *fakeRepo) ClaimPendingUpdates(_ context.Context, _ int, _ time.Time) ([]model.RateUpdate, error) {
	f.claimCalls++
	batch := f.claims
	f.claims = nil
	return batch, nil
}

func (f *fakeRepo) CompleteUpdateWithRate(_ context.Context, _ string, price float64, _ time.Time, _ model.Rate) error {
	f.completeCalls++
	f.completedWith = price
	return nil
}

func (f *fakeRepo) FailUpdate(_ context.Context, _ string, msg string, _ time.Time) error {
	f.failCalls++
	f.failMsg = msg
	return nil
}

func (f *fakeRepo) RequeueUpdate(_ context.Context, _ string, _ time.Time) error {
	f.requeueCalls++
	return nil
}

func (f *fakeRepo) GetLatestRate(_ context.Context, _ model.CurrencyPair) (model.Rate, error) {
	return model.Rate{}, model.ErrNotFound
}

func (f *fakeRepo) ResetStaleUpdates(_ context.Context, _ time.Duration, _ time.Time) (int64, error) {
	return f.resetCount, nil
}

type fakeProvider struct {
	errors []error
	rate   model.Rate
	calls  int
}

func (f *fakeProvider) FetchRate(_ context.Context, _ model.CurrencyPair) (model.Rate, error) {
	f.calls++
	if f.calls <= len(f.errors) {
		return model.Rate{}, f.errors[f.calls-1]
	}
	return f.rate, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func newTestService(t *testing.T, repo RateRepository, prov RateProvider, cfg Config) *RateService {
	t.Helper()
	svc, err := NewRateService(repo, prov, cfg, discardLogger())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

var testPair = model.CurrencyPair{Base: "EUR", Quote: "MXN"}

func TestProcess_Success(t *testing.T) {
	repo := &fakeRepo{}
	prov := &fakeProvider{rate: model.Rate{Pair: testPair, Price: 21.02}}
	svc := newTestService(t, repo, prov, DefaultConfig())

	svc.process(context.Background(), model.RateUpdate{
		ID: "test-id", Pair: testPair, Status: model.StatusInProgress,
	})

	if repo.completeCalls != 1 || repo.completedWith != 21.02 {
		t.Errorf("completeCalls = %d, price = %v; want 1 and 21.02", repo.completeCalls, repo.completedWith)
	}
	if repo.failCalls != 0 {
		t.Errorf("failCalls = %d, want 0", repo.failCalls)
	}
}

func TestProcess_ProviderFails(t *testing.T) {
	repo := &fakeRepo{}
	prov := &fakeProvider{errors: []error{errors.New("provider timeout")}}
	cfg := DefaultConfig()
	cfg.FetchAttempts = 1
	svc := newTestService(t, repo, prov, cfg)

	svc.process(context.Background(), model.RateUpdate{
		ID: "x", Pair: testPair, Status: model.StatusInProgress,
	})

	if repo.failCalls != 1 || !strings.Contains(repo.failMsg, "provider timeout") {
		t.Errorf("failCalls = %d, msg = %q; want 1 and reason text inside", repo.failCalls, repo.failMsg)
	}
	if repo.completeCalls != 0 {
		t.Errorf("completeCalls = %d, want 0", repo.completeCalls)
	}
}

func TestFetchWithRetry_SucceedsAfterRetries(t *testing.T) {
	prov := &fakeProvider{
		errors: []error{errors.New("boom 1"), errors.New("boom 2")},
		rate:   model.Rate{Pair: testPair, Price: 1.5},
	}
	cfg := DefaultConfig()
	cfg.FetchAttempts = 3
	cfg.FetchRetryPause = time.Millisecond
	svc := newTestService(t, &fakeRepo{}, prov, cfg)

	rate, err := svc.fetchWithRetry(context.Background(), testPair)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate.Price != 1.5 || prov.calls != 3 {
		t.Errorf("price = %v, calls = %d; want 1.5 and 3 (две ошибки + успех)", rate.Price, prov.calls)
	}
}

func TestFetchWithRetry_AllAttemptsFail(t *testing.T) {
	prov := &fakeProvider{errors: []error{errors.New("a"), errors.New("b")}}
	cfg := DefaultConfig()
	cfg.FetchAttempts = 2
	cfg.FetchRetryPause = time.Millisecond
	svc := newTestService(t, &fakeRepo{}, prov, cfg)

	_, err := svc.fetchWithRetry(context.Background(), testPair)

	if err == nil {
		t.Fatal("want error when all attempts fail")
	}
	if prov.calls != 2 {
		t.Errorf("calls = %d, want 2", prov.calls)
	}
}

func TestClaimLoop_ProcessesClaims(t *testing.T) {
	repo := &fakeRepo{claims: []model.RateUpdate{
		{ID: "c1", Pair: testPair, Status: model.StatusInProgress},
	}}
	prov := &fakeProvider{rate: model.Rate{Pair: testPair, Price: 10.5}}
	svc := newTestService(t, repo, prov, DefaultConfig())

	svc.claimLoop(context.Background())

	if repo.claimCalls < 1 {
		t.Fatalf("claimCalls = %d, want >= 1", repo.claimCalls)
	}
	if repo.completeCalls != 1 {
		t.Errorf("completeCalls = %d, want 1: заявленная партия обработана", repo.completeCalls)
	}
	if repo.claimCalls < 2 {
		t.Errorf("claimCalls = %d, want >= 2: пустая партия должна остановить цикл", repo.claimCalls)
	}
}

func TestRequestUpdate_UnsupportedPair(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(t, repo, &fakeProvider{}, DefaultConfig())

	_, err := svc.RequestUpdate(context.Background(), model.CurrencyPair{Base: "USD", Quote: "GBP"})

	if !errors.Is(err, model.ErrUnsupportedPair) {
		t.Errorf("err = %v, want ErrUnsupportedPair", err)
	}
	if repo.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0: неподдерживаемая пара не должна попадать в БД", repo.createCalls)
	}
}

func TestRequestUpdate_Ok(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(t, repo, &fakeProvider{}, DefaultConfig())

	created, err := svc.RequestUpdate(context.Background(), testPair)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if created.ID == "" {
		t.Fatal("created id is empty")
	}
	if repo.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1", repo.createCalls)
	}

	select {
	case <-svc.wake:
	default:
		t.Error("wake signal не отправлен: новое задание должно будить воркера")
	}
}

func TestRequestUpdate_DuplicateDoesNotNotify(t *testing.T) {
	repo := &fakeRepo{createResult: model.RateUpdate{ID: "existing-id", Pair: testPair, Status: model.StatusInProgress}}
	svc := newTestService(t, repo, &fakeProvider{}, DefaultConfig())

	created, err := svc.RequestUpdate(context.Background(), testPair)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if created.ID != "existing-id" {
		t.Errorf("id = %q, want existing-id", created.ID)
	}

	select {
	case <-svc.wake:
		t.Error("дубль не должен будить воркера")
	default:
	}
}

func TestRecoverOnce_NotifiesAfterReset(t *testing.T) {
	repo := &fakeRepo{resetCount: 2}
	svc := newTestService(t, repo, &fakeProvider{}, DefaultConfig())

	svc.recoverOnce(context.Background())

	select {
	case <-svc.wake:
	default:
		t.Error("после сброса зависших заданий воркер должен получить сигнал")
	}
}

func TestRecoverOnce_NoResetNoNotify(t *testing.T) {
	repo := &fakeRepo{resetCount: 0}
	svc := newTestService(t, repo, &fakeProvider{}, DefaultConfig())

	svc.recoverOnce(context.Background())

	select {
	case <-svc.wake:
		t.Error("без сброшенных заданий сигнала быть не должно")
	default:
	}
}

type slowProvider struct{ delay time.Duration }

func (p slowProvider) FetchRate(ctx context.Context, _ model.CurrencyPair) (model.Rate, error) {
	select {
	case <-time.After(p.delay):
		return model.Rate{Pair: testPair, Price: 9.99}, nil
	case <-ctx.Done():
		return model.Rate{}, ctx.Err()
	}
}

func TestProcess_FinishAfterSlowFetch(t *testing.T) {
	repo := &fakeRepo{}
	cfg := DefaultConfig()
	cfg.FinishTimeout = 50 * time.Millisecond
	svc := newTestService(t, repo, slowProvider{delay: 120 * time.Millisecond}, cfg)

	svc.process(context.Background(), model.RateUpdate{
		ID: "slow", Pair: testPair, Status: model.StatusInProgress,
	})

	if repo.completeCalls != 1 || repo.completedWith != 9.99 {
		t.Errorf("completeCalls = %d, price = %v; want 1 and 9.99: результат обязан записаться после медленного fetch",
			repo.completeCalls, repo.completedWith)
	}
}

func TestProcess_CancelRequeues(t *testing.T) {
	repo := &fakeRepo{}
	prov := &fakeProvider{errors: []error{context.Canceled}}
	cfg := DefaultConfig()
	cfg.FetchAttempts = 1
	svc := newTestService(t, repo, prov, cfg)

	svc.process(context.Background(), model.RateUpdate{
		ID: "x", Pair: testPair, Status: model.StatusInProgress,
	})

	if repo.requeueCalls != 1 {
		t.Errorf("requeueCalls = %d, want 1", repo.requeueCalls)
	}
	if repo.failCalls != 0 {
		t.Errorf("failCalls = %d, want 0: отмена — не ошибка задания", repo.failCalls)
	}
}
