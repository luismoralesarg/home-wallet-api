package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

type UserRef struct {
	ID   string
	Name string
}

// Transaction es un ingreso o gasto en efectivo/cuenta (las compras con tarjeta son otro módulo).
type Transaction struct {
	ID          string
	Kind        Kind
	Amount      decimal.Decimal
	Currency    Currency
	FxRate      decimal.Decimal
	AmountARS   decimal.Decimal
	CategoryID  string
	Description string
	OccurredAt  time.Time
	CreatedBy   UserRef
	CreatedAt   time.Time
}
