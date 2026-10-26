package ports

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type CardPurchaseFilter struct {
	From, To   *time.Time // [From, To) sobre purchased_at
	CardID     *string
	CategoryID *string
	UserID     *string
	Limit      int
	Offset     int
}

type CardRepository interface {
	List(ctx context.Context) ([]domain.Card, error)
	Get(ctx context.Context, id string) (domain.Card, error) // application.ErrNotFound
	Create(ctx context.Context, c domain.Card) (domain.Card, error)
	Update(ctx context.Context, c domain.Card) (domain.Card, error)
	Delete(ctx context.Context, id string) error
	HasPurchases(ctx context.Context, id string) (bool, error)

	// CreatePurchase / UpdatePurchase guardan la compra, reemplazan sus cuotas por `plan`
	// y crean (si no existen) los resúmenes indicados en `dueDates`, todo en una transacción.
	CreatePurchase(ctx context.Context, p domain.CardPurchase, plan []domain.Installment, dueDates map[domain.Month]time.Time) (domain.CardPurchase, error)
	UpdatePurchase(ctx context.Context, p domain.CardPurchase, plan []domain.Installment, dueDates map[domain.Month]time.Time) (domain.CardPurchase, error)
	GetPurchase(ctx context.Context, cardID, id string) (domain.CardPurchase, error)
	DeletePurchase(ctx context.Context, cardID, id string) error
	ListPurchases(ctx context.Context, f CardPurchaseFilter) ([]domain.CardPurchase, int, error)

	// Resúmenes.
	InstallmentTotals(ctx context.Context, cardID string) (map[domain.Month]decimal.Decimal, error)
	StatementRows(ctx context.Context, cardID string) (map[domain.Month]domain.StatementRow, error)
	StatementItems(ctx context.Context, cardID string, m domain.Month) ([]domain.StatementItem, error)
	SetAdjustment(ctx context.Context, cardID string, m domain.Month, dueDate time.Time, adjustment decimal.Decimal) error
	MarkPaid(ctx context.Context, cardID string, m domain.Month, dueDate time.Time, paidAt time.Time) error
}
