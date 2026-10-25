package application

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
	"github.com/luismoralesarg/home-wallet-api/internal/ports"
)

// Converter lo implementa ExchangeRateService.
type Converter interface {
	ConvertToARS(ctx context.Context, amount decimal.Decimal, currency domain.Currency, date time.Time) (fx, ars decimal.Decimal, err error)
}

const (
	DefaultPageSize = 30
	MaxPageSize     = 100
)

// maxARS es el límite de NUMERIC(14,2).
var maxARS = decimal.New(1, 12)

type TransactionService struct {
	repo       ports.TransactionRepository
	categories ports.CategoryRepository
	fx         Converter
	now        func() time.Time
}

func NewTransactionService(repo ports.TransactionRepository, categories ports.CategoryRepository, fx Converter) *TransactionService {
	return &TransactionService{repo: repo, categories: categories, fx: fx, now: time.Now}
}

type TransactionInput struct {
	Kind        domain.Kind
	Amount      decimal.Decimal
	Currency    domain.Currency
	CategoryID  string
	Description string
	OccurredAt  *time.Time // por defecto, ahora
}

type TransactionPatch struct {
	Amount      *decimal.Decimal
	Currency    *domain.Currency
	CategoryID  *string
	Description *string
	OccurredAt  *time.Time
}

type ListTransactionsQuery struct {
	Month      *time.Time // primer instante del mes en Mendoza
	Kind       *domain.Kind
	CategoryID *string
	UserID     *string
	Limit      int
	Offset     int
}

type TransactionPage struct {
	Items         []domain.Transaction
	Total         int
	Limit, Offset int
}

func (s *TransactionService) Create(ctx context.Context, userID string, in TransactionInput) (domain.Transaction, error) {
	occurred := s.now()
	if in.OccurredAt != nil {
		occurred = *in.OccurredAt
	}
	t := domain.Transaction{
		Kind:        in.Kind,
		Amount:      in.Amount,
		Currency:    in.Currency,
		CategoryID:  in.CategoryID,
		Description: in.Description,
		OccurredAt:  occurred,
		CreatedBy:   domain.UserRef{ID: userID},
	}
	if err := s.prepare(ctx, &t); err != nil {
		return domain.Transaction{}, err
	}
	return s.repo.Create(ctx, t)
}

func (s *TransactionService) Update(ctx context.Context, id string, p TransactionPatch) (domain.Transaction, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.Transaction{}, err
	}
	if p.Amount != nil {
		t.Amount = *p.Amount
	}
	if p.Currency != nil {
		t.Currency = *p.Currency
	}
	if p.CategoryID != nil {
		t.CategoryID = *p.CategoryID
	}
	if p.Description != nil {
		t.Description = *p.Description
	}
	if p.OccurredAt != nil {
		t.OccurredAt = *p.OccurredAt
	}
	// Se recalcula siempre: el fx_rate es el del día en que ocurrió el movimiento.
	if err := s.prepare(ctx, &t); err != nil {
		return domain.Transaction{}, err
	}
	return s.repo.Update(ctx, t)
}

func (s *TransactionService) Get(ctx context.Context, id string) (domain.Transaction, error) {
	return s.repo.Get(ctx, id)
}

func (s *TransactionService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *TransactionService) List(ctx context.Context, q ListTransactionsQuery) (TransactionPage, error) {
	if q.Kind != nil && !q.Kind.Valid() {
		return TransactionPage{}, invalid("kind inválido")
	}
	if q.Limit <= 0 {
		q.Limit = DefaultPageSize
	}
	if q.Limit > MaxPageSize {
		q.Limit = MaxPageSize
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	f := ports.TransactionFilter{Kind: q.Kind, CategoryID: q.CategoryID, UserID: q.UserID, Limit: q.Limit, Offset: q.Offset}
	if q.Month != nil {
		from := *q.Month
		to := from.AddDate(0, 1, 0)
		f.From, f.To = &from, &to
	}
	items, total, err := s.repo.List(ctx, f)
	if err != nil {
		return TransactionPage{}, err
	}
	return TransactionPage{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}

// prepare valida y completa fx_rate y amount_ars.
func (s *TransactionService) prepare(ctx context.Context, t *domain.Transaction) error {
	if !t.Kind.Valid() {
		return invalid("kind debe ser income o expense")
	}
	if !t.Currency.Valid() {
		return invalid("currency debe ser ARS o USD")
	}
	if !t.Amount.IsPositive() {
		return invalid("el monto debe ser mayor a cero")
	}
	if !t.Amount.Equal(t.Amount.Round(2)) {
		return invalid("el monto admite hasta 2 decimales")
	}
	cat, err := s.categories.Get(ctx, t.CategoryID)
	if err != nil {
		if err == ErrNotFound {
			return invalid("la categoría no existe")
		}
		return err
	}
	if cat.Kind != t.Kind {
		return invalid("la categoría es de tipo %s, no %s", cat.Kind, t.Kind)
	}
	fx, ars, err := s.fx.ConvertToARS(ctx, t.Amount, t.Currency, t.OccurredAt)
	if err != nil {
		return err
	}
	if ars.Abs().GreaterThanOrEqual(maxARS) {
		return invalid("el monto es demasiado grande")
	}
	t.FxRate, t.AmountARS = fx, ars
	return nil
}
