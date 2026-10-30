package model

import "time"

type Rate struct {
	Pair      CurrencyPair
	Price     float64
	UpdatedAt time.Time
}
