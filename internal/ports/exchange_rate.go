package ports

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type ExchangeRateRepository interface {
	// Upsert guarda la cotización del día (única por fecha).
	Upsert(ctx context.Context, r domain.ExchangeRate) error
	// LatestOnOrBefore devuelve la cotización más reciente con fecha <= date.
	LatestOnOrBefore(ctx context.Context, date time.Time) (domain.ExchangeRate, bool, error)
}

// RateFetcher obtiene el dólar oficial actual de un proveedor externo.
type RateFetcher interface {
	FetchOfficial(ctx context.Context) (buy, sell decimal.Decimal, err error)
}
