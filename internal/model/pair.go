package model

import (
	"fmt"
	"strings"
)

type CurrencyPair struct {
	Base  string
	Quote string
}

func ParseCurrencyPair(s string) (CurrencyPair, error) {
	s = strings.TrimSpace(s)

	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return CurrencyPair{}, fmt.Errorf("invalid currency pair %q: expected BASE/QUOTE", s)
	}

	base := strings.ToUpper(parts[0])
	quote := strings.ToUpper(parts[1])

	if !isValidCurrencyCode(base) {
		return CurrencyPair{}, fmt.Errorf("invalid base currency: %q", base)
	}

	if !isValidCurrencyCode(quote) {
		return CurrencyPair{}, fmt.Errorf("invalid quote currency: %q", quote)
	}

	if base == quote {
		return CurrencyPair{}, fmt.Errorf("base and quote currencies must differ")
	}

	return CurrencyPair{
		Base:  base,
		Quote: quote,
	}, nil
}

func isValidCurrencyCode(code string) bool {
	if len(code) != 3 {
		return false
	}

	for i := 0; i < len(code); i++ {
		if code[i] < 'A' || code[i] > 'Z' {
			return false
		}
	}

	return true
}

func (p CurrencyPair) String() string {
	return p.Base + "/" + p.Quote
}
