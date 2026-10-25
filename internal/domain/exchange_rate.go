package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

type Currency string

const (
	ARS Currency = "ARS"
	USD Currency = "USD"
)

func (c Currency) Valid() bool { return c == ARS || c == USD }

// ExchangeRate es la cotización del dólar oficial de un día (hora Mendoza).
type ExchangeRate struct {
	Date      time.Time // día calendario, 00:00 UTC
	Buy       decimal.Decimal
	Sell      decimal.Decimal
	FetchedAt time.Time
}

// ToARS convierte con la venta del día y redondea a centavos.
// En ARS la cotización es 1 y el monto no cambia.
func ToARS(amount decimal.Decimal, currency Currency, rate *ExchangeRate) (fx, ars decimal.Decimal) {
	if currency == ARS || rate == nil {
		return decimal.NewFromInt(1), amount.Round(2)
	}
	return rate.Sell, amount.Mul(rate.Sell).Round(2)
}
