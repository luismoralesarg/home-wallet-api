package ports

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

// DueStatement es un resumen de tarjeta visto por su vencimiento.
type DueStatement struct {
	CardID   string
	CardName string
	Month    domain.Month // mes de cierre
	DueDate  time.Time
	TotalARS decimal.Decimal // cuotas + ajuste de saldo previo
	Paid     bool
}

type CategoryTotal struct {
	CategoryID string
	Name       string
	Total      decimal.Decimal
}

type DashboardRepository interface {
	// SumTransactions suma amount_ars de las transacciones de un tipo con occurred_at en [from, to).
	SumTransactions(ctx context.Context, kind domain.Kind, from, to time.Time) (decimal.Decimal, error)
	// StatementsDue lista los resúmenes con vencimiento en [from, to) (fechas a las 00:00 UTC).
	StatementsDue(ctx context.Context, from, to time.Time) ([]DueStatement, error)
	// ExpenseByCategory suma los gastos en [from, to) por categoría: transacciones de gasto
	// más compras con tarjeta (por fecha de compra, monto completo).
	ExpenseByCategory(ctx context.Context, from, to time.Time) ([]CategoryTotal, error)
}
