package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"plata/internal/model"
)

type fakeService struct {
	requestResult model.RateUpdate
	requestErr    error

	getResult model.RateUpdate
	getErr    error

	latestResult model.Rate
	latestErr    error
}

func (f *fakeService) RequestUpdate(_ context.Context, _ model.CurrencyPair) (model.RateUpdate, error) {
	return f.requestResult, f.requestErr
}

func (f *fakeService) GetUpdate(_ context.Context, _ string) (model.RateUpdate, error) {
	return f.getResult, f.getErr
}

func (f *fakeService) GetLatestRate(_ context.Context, _ model.CurrencyPair) (model.Rate, error) {
	return f.latestResult, f.latestErr
}

func newTestServer(svc *fakeService) *httptest.Server {
	h := NewHandler(svc, slog.New(slog.DiscardHandler))
	return httptest.NewServer(h.Router())
}

func postJSON(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	return resp
}

func decode(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

func TestCreateUpdate_Success(t *testing.T) {
	svc := &fakeService{requestResult: model.RateUpdate{
		ID: "abc-123", Pair: model.CurrencyPair{Base: "EUR", Quote: "MXN"}, Status: model.StatusPending,
	}}
	srv := newTestServer(svc)
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/v1/rates/updates", `{"pair":"EUR/MXN"}`)
	body := decode(t, resp)

	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("status = %d, want 202", resp.StatusCode)
	}
	if body["id"] != "abc-123" || body["status"] != "pending" || body["pair"] != "EUR/MXN" {
		t.Errorf("body = %v, want id/pair/status", body)
	}
	if body["price"] != nil {
		t.Error("price в ответе создания должен быть null: обновление ещё не выполнено")
	}
}

func TestCreateUpdate_InvalidJSON(t *testing.T) {
	srv := newTestServer(&fakeService{})
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/v1/rates/updates", `{not json`)

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCreateUpdate_InvalidPair(t *testing.T) {
	srv := newTestServer(&fakeService{})
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/v1/rates/updates", `{"pair":"NOT-A-PAIR"}`)

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCreateUpdate_UnsupportedPair(t *testing.T) {
	svc := &fakeService{requestErr: model.ErrUnsupportedPair}
	srv := newTestServer(svc)
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/v1/rates/updates", `{"pair":"USD/GBP"}`)

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for unsupported pair", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCreateUpdate_InternalError(t *testing.T) {
	svc := &fakeService{requestErr: errors.New("db is down")}
	srv := newTestServer(svc)
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/v1/rates/updates", `{"pair":"EUR/MXN"}`)
	body := decode(t, resp)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if body["message"] != "internal error" {
		t.Errorf("message = %v, want exactly \"internal error\": внутренний текст не должен протекать наружу", body["message"])
	}
}

func TestGetUpdate_Found(t *testing.T) {
	price := 21.02
	completed := time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC)
	const id = "11111111-1111-1111-1111-111111111111"
	svc := &fakeService{getResult: model.RateUpdate{
		ID: id, Pair: model.CurrencyPair{Base: "EUR", Quote: "MXN"},
		Status: model.StatusCompleted, Price: &price, CompletedAt: &completed,
	}}
	srv := newTestServer(svc)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/rates/updates/" + id)
	if err != nil {
		t.Fatal(err)
	}
	body := decode(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if body["status"] != "completed" || body["price"] != 21.02 {
		t.Errorf("body = %v, want completed with price", body)
	}
	if body["updated_at"] == nil {
		t.Error("updated_at должен быть заполнен для completed")
	}
}

func TestGetUpdate_NotFound(t *testing.T) {
	svc := &fakeService{getErr: model.ErrNotFound}
	srv := newTestServer(svc)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/rates/updates/99999999-9999-9999-9999-999999999999")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestGetLatest_Found(t *testing.T) {
	svc := &fakeService{latestResult: model.Rate{
		Pair:  model.CurrencyPair{Base: "EUR", Quote: "MXN"},
		Price: 21.02, UpdatedAt: time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC),
	}}
	srv := newTestServer(svc)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/rates/latest?pair=EUR/MXN")
	if err != nil {
		t.Fatal(err)
	}
	body := decode(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if body["price"] != 21.02 || body["pair"] != "EUR/MXN" {
		t.Errorf("body = %v, want pair and price", body)
	}
}

func TestGetLatest_NotFound(t *testing.T) {
	svc := &fakeService{latestErr: model.ErrNotFound}
	srv := newTestServer(svc)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/rates/latest?pair=USD/GBP")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestGetUpdate_InvalidUUID(t *testing.T) {
	srv := newTestServer(&fakeService{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/rates/updates/not-a-uuid")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}
