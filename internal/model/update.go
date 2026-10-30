package model

import "time"

type UpdateStatus string

const (
	StatusPending    UpdateStatus = "pending"
	StatusInProgress UpdateStatus = "in_progress"
	StatusCompleted  UpdateStatus = "completed"
	StatusFailed     UpdateStatus = "failed"
)

type RateUpdate struct {
	ID          string
	Pair        CurrencyPair
	Status      UpdateStatus
	Price       *float64
	Error       string
	RequestedAt time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
}
