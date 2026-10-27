package domain

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/tz"
)

type Subscription struct {
	ID        string
	Name      string
	Amount    decimal.Decimal
	Currency  Currency
	ChargeDay int
	Active    bool
	CreatedBy UserRef
	CreatedAt time.Time
}

// NextChargeDate es el próximo cobro desde `now` (hoy incluido), en hora Mendoza.
// Si el mes es más corto que el día de cobro, se cobra el último día del mes.
func (s Subscription) NextChargeDate(now time.Time) time.Time {
	today := tz.DateOf(now)
	m := MonthOfDate(today)
	if d := m.Day(s.ChargeDay); !d.Before(today) {
		return d
	}
	return m.AddMonths(1).Day(s.ChargeDay)
}
