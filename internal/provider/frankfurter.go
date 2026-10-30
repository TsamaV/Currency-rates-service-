package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"plata/internal/model"
	"time"
)

type Frankfurter struct {
	client  *http.Client
	baseURL string
}

type frankfurterResponse struct {
	Base  string             `json:"base"`
	Rates map[string]float64 `json:"rates"`
}

func NewFrankfurter(baseURL string, timeout time.Duration) *Frankfurter {
	return &Frankfurter{
		client: &http.Client{
			Timeout: timeout,
		},
		baseURL: baseURL,
	}
}

func (f *Frankfurter) FetchRate(ctx context.Context, pair model.CurrencyPair) (model.Rate, error) {
	url := fmt.Sprintf("%s/latest?base=%s&symbols=%s", f.baseURL, pair.Base, pair.Quote)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return model.Rate{}, fmt.Errorf("create request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return model.Rate{}, fmt.Errorf("fetch Frankfurter rate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return model.Rate{}, fmt.Errorf("Frankfurter returned HTTP status %d", resp.StatusCode)
	}

	var result frankfurterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return model.Rate{}, fmt.Errorf("decode Frankfurter response: %w", err)
	}

	price, ok := result.Rates[pair.Quote]
	if !ok {
		return model.Rate{}, fmt.Errorf("no rate for %s in frankfurter response", pair.Quote)
	}

	return model.Rate{
		Pair:      pair,
		Price:     price,
		UpdatedAt: time.Now(),
	}, nil
}
