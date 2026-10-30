package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"plata/internal/model"
	"time"

	"github.com/google/uuid"
)

type RateService interface {
	RequestUpdate(ctx context.Context, pair model.CurrencyPair) (model.RateUpdate, error)
	GetUpdate(ctx context.Context, id string) (model.RateUpdate, error)
	GetLatestRate(ctx context.Context, pair model.CurrencyPair) (model.Rate, error)
}

type Handler struct {
	service RateService
	log     *slog.Logger
}

func NewHandler(service RateService, log *slog.Logger) *Handler {
	return &Handler{
		service: service,
		log:     log,
	}
}

func (h *Handler) Router() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/rates/updates", h.createUpdate)
	mux.HandleFunc("GET /api/v1/rates/updates/{id}", h.getUpdate)
	mux.HandleFunc("GET /api/v1/rates/latest", h.getLatest)

	return logRequests(h.log, recoverPanics(h.log, mux))
}

type createUpdateRequest struct {
	Pair string `json:"pair"`
}

type updateResponse struct {
	ID        string     `json:"id"`
	Pair      string     `json:"pair"`
	Status    string     `json:"status"`
	Price     *float64   `json:"price"`
	UpdatedAt *time.Time `json:"updated_at"`
	Error     string     `json:"error,omitempty"`
}

type latestResponse struct {
	Pair      string    `json:"pair"`
	Price     float64   `json:"price"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (h *Handler) createUpdate(w http.ResponseWriter, r *http.Request) {
	var req createUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}

	pair, err := model.ParseCurrencyPair(req.Pair)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid pair")
		return
	}

	created, err := h.service.RequestUpdate(r.Context(), pair)
	if err != nil {
		if errors.Is(err, model.ErrUnsupportedPair) {
			WriteError(w, http.StatusBadRequest, "unsupported pair")
			return
		}
		h.log.Error("request update", "err", err)
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	WriteJSON(w, http.StatusAccepted, updateResponse{
		ID:     created.ID,
		Pair:   created.Pair.String(),
		Status: string(created.Status),
	})
}

func (h *Handler) getUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, err := uuid.Parse(id); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	updated, err := h.service.GetUpdate(r.Context(), id)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "not found")
			return
		}
		h.log.Error("get update", "err", err)
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	WriteJSON(w, http.StatusOK, updateResponse{
		ID:        updated.ID,
		Pair:      updated.Pair.String(),
		Status:    string(updated.Status),
		Price:     updated.Price,
		UpdatedAt: updated.CompletedAt,
		Error:     updated.Error,
	})
}

func (h *Handler) getLatest(w http.ResponseWriter, r *http.Request) {
	pair, err := model.ParseCurrencyPair(r.URL.Query().Get("pair"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid pair")
		return
	}

	latest, err := h.service.GetLatestRate(r.Context(), pair)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "not found")
			return
		}
		h.log.Error("get latest rate", "err", err)
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	WriteJSON(w, http.StatusOK, latestResponse{
		Pair:      latest.Pair.String(),
		Price:     latest.Price,
		UpdatedAt: latest.UpdatedAt,
	})
}

func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"message": msg})
}
