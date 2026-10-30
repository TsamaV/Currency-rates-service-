package model

import "testing"

func TestParseCurrencyPair(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    CurrencyPair
		wantErr bool
	}{
		{
			name:  "valid pair",
			input: "EUR/MXN",
			want:  CurrencyPair{Base: "EUR", Quote: "MXN"},
		},
		{
			name:  "trim spaces and normalize case",
			input: " EUR/mxn ",
			want:  CurrencyPair{Base: "EUR", Quote: "MXN"},
		},
		{
			name:    "missing separator",
			input:   "EURMXN",
			wantErr: true,
		},
		{
			name:    "too many parts",
			input:   "EUR/MXN/USD",
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "short base currency",
			input:   "EU/MXN",
			wantErr: true,
		},
		{
			name:    "digits in currency code",
			input:   "EU1/MXN",
			wantErr: true,
		},
		{
			name:    "non ASCII letters",
			input:   "РУБ/USD",
			wantErr: true,
		},
		{
			name:    "same currencies",
			input:   "USD/USD",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCurrencyPair(tt.input)

			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseCurrencyPair() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				return
			}

			if got != tt.want {
				t.Errorf("ParseCurrencyPair() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCurrencyPairString(t *testing.T) {
	pair := CurrencyPair{
		Base:  "EUR",
		Quote: "MXN",
	}

	got := pair.String()
	want := "EUR/MXN"

	if got != want {
		t.Errorf("CurrencyPair.String() = %q, want %q", got, want)
	}
}
