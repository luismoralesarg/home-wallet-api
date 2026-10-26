package application

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
	"github.com/luismoralesarg/home-wallet-api/internal/ports"
)

type memCats struct{ byID map[string]domain.Category }

func (m *memCats) List(context.Context, *domain.Kind) ([]domain.Category, error) { return nil, nil }
func (m *memCats) Get(_ context.Context, id string) (domain.Category, error) {
	c, ok := m.byID[id]
	if !ok {
		return c, ErrNotFound
	}
	return c, nil
}
func (m *memCats) Create(context.Context, domain.Category) (domain.Category, error) {
	return domain.Category{}, nil
}
func (m *memCats) Update(context.Context, string, string) (domain.Category, error) {
	return domain.Category{}, nil
}
func (m *memCats) Delete(context.Context, string) error { return nil }

type memTxs struct {
	byID   map[string]domain.Transaction
	filter ports.TransactionFilter
}

func (m *memTxs) List(_ context.Context, f ports.TransactionFilter) ([]domain.Transaction, int, error) {
	m.filter = f
	return nil, 0, nil
}
func (m *memTxs) Get(_ context.Context, id string) (domain.Transaction, error) {
	t, ok := m.byID[id]
	if !ok {
		return t, ErrNotFound
	}
	return t, nil
}
func (m *memTxs) Create(_ context.Context, t domain.Transaction) (domain.Transaction, error) {
	t.ID = "tx-" + strconv.Itoa(len(m.byID)+1)
	m.byID[t.ID] = t
	return t, nil
}
func (m *memTxs) Update(_ context.Context, t domain.Transaction) (domain.Transaction, error) {
	m.byID[t.ID] = t
	return t, nil
}
func (m *memTxs) Delete(context.Context, string) error { return nil }

// fixedRates convierte con una venta distinta por día (clave YYYY-MM-DD, hora Mendoza).
type fixedRates map[string]string

func (f fixedRates) ConvertToARS(_ context.Context, amount decimal.Decimal, cur domain.Currency, date time.Time) (decimal.Decimal, decimal.Decimal, error) {
	if cur == domain.ARS {
		return decimal.NewFromInt(1), amount, nil
	}
	sell, ok := f[date.In(time.FixedZone("AR", -3*3600)).Format("2006-01-02")]
	if !ok {
		return decimal.Zero, decimal.Zero, ErrNoExchangeRate
	}
	fx := decimal.RequireFromString(sell)
	return fx, amount.Mul(fx).Round(2), nil
}

func newTxSvc(rates fixedRates) (*TransactionService, *memTxs) {
	cats := &memCats{byID: map[string]domain.Category{
		"cat-food":   {ID: "cat-food", Name: "Comida", Kind: domain.Expense},
		"cat-salary": {ID: "cat-salary", Name: "Sueldo", Kind: domain.Income},
	}}
	txs := &memTxs{byID: map[string]domain.Transaction{}}
	s := NewTransactionService(txs, cats, rates)
	s.now = func() time.Time { return time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC) }
	return s, txs
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestCreateTransactionCurrency(t *testing.T) {
	ctx := context.Background()
	s, _ := newTxSvc(fixedRates{"2026-10-15": "1450.33", "2026-10-10": "1400"})

	t.Run("ARS: fx_rate = 1 y amount_ars = amount", func(t *testing.T) {
		tx, err := s.Create(ctx, "u-1", TransactionInput{Kind: domain.Expense, Amount: dec("2500.50"), Currency: domain.ARS, CategoryID: "cat-food"})
		if err != nil {
			t.Fatal(err)
		}
		if !tx.FxRate.Equal(dec("1")) || !tx.AmountARS.Equal(dec("2500.50")) || tx.CreatedBy.ID != "u-1" {
			t.Fatalf("%+v", tx)
		}
	})

	t.Run("USD: guarda la cotización del día y amount_ars; occurred_at por defecto es ahora", func(t *testing.T) {
		tx, err := s.Create(ctx, "u-1", TransactionInput{Kind: domain.Expense, Amount: dec("20"), Currency: domain.USD, CategoryID: "cat-food"})
		if err != nil {
			t.Fatal(err)
		}
		if !tx.FxRate.Equal(dec("1450.33")) || !tx.AmountARS.Equal(dec("29006.60")) {
			t.Fatalf("fx=%v ars=%v", tx.FxRate, tx.AmountARS)
		}
		if !tx.OccurredAt.Equal(time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)) {
			t.Fatalf("occurred_at = %v", tx.OccurredAt)
		}
	})

	t.Run("USD con fecha pasada usa la cotización de esa fecha", func(t *testing.T) {
		at := time.Date(2026, 10, 10, 18, 0, 0, 0, time.UTC)
		tx, err := s.Create(ctx, "u-1", TransactionInput{Kind: domain.Expense, Amount: dec("1"), Currency: domain.USD, CategoryID: "cat-food", OccurredAt: &at})
		if err != nil || !tx.FxRate.Equal(dec("1400")) {
			t.Fatalf("fx=%v err=%v", tx.FxRate, err)
		}
	})

	t.Run("sin cotización disponible falla", func(t *testing.T) {
		at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		_, err := s.Create(ctx, "u-1", TransactionInput{Kind: domain.Expense, Amount: dec("1"), Currency: domain.USD, CategoryID: "cat-food", OccurredAt: &at})
		if !errors.Is(err, ErrNoExchangeRate) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestCreateTransactionValidation(t *testing.T) {
	s, _ := newTxSvc(fixedRates{"2026-10-15": "1000"})
	base := TransactionInput{Kind: domain.Expense, Amount: dec("10"), Currency: domain.ARS, CategoryID: "cat-food"}
	mut := func(f func(*TransactionInput)) TransactionInput { in := base; f(&in); return in }

	cases := map[string]TransactionInput{
		"monto cero":              mut(func(i *TransactionInput) { i.Amount = dec("0") }),
		"monto negativo":          mut(func(i *TransactionInput) { i.Amount = dec("-5") }),
		"3 decimales":             mut(func(i *TransactionInput) { i.Amount = dec("1.234") }),
		"moneda inválida":         mut(func(i *TransactionInput) { i.Currency = "EUR" }),
		"kind inválido":           mut(func(i *TransactionInput) { i.Kind = "otro" }),
		"categoría inexistente":   mut(func(i *TransactionInput) { i.CategoryID = "nope" }),
		"categoría de otro tipo":  mut(func(i *TransactionInput) { i.CategoryID = "cat-salary" }),
		"monto fuera de NUMERIC":  mut(func(i *TransactionInput) { i.Amount = dec("1000000000000") }),
		"USD que desborda en ARS": mut(func(i *TransactionInput) { i.Currency = domain.USD; i.Amount = dec("2000000000") }),
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			var ve *ValidationError
			if _, err := s.Create(context.Background(), "u-1", in); !errors.As(err, &ve) {
				t.Fatalf("esperaba ValidationError, got %v", err)
			}
		})
	}

	t.Run("3 decimales con ceros sobrantes es válido", func(t *testing.T) {
		in := mut(func(i *TransactionInput) { i.Amount = dec("10.500") })
		if _, err := s.Create(context.Background(), "u-1", in); err != nil {
			t.Fatal(err)
		}
	})
}

func TestUpdateTransactionRecalculates(t *testing.T) {
	ctx := context.Background()
	s, _ := newTxSvc(fixedRates{"2026-10-15": "1500", "2026-10-10": "1400"})
	tx, err := s.Create(ctx, "u-1", TransactionInput{Kind: domain.Expense, Amount: dec("10"), Currency: domain.USD, CategoryID: "cat-food"})
	if err != nil {
		t.Fatal(err)
	}

	amt := dec("20")
	got, err := s.Update(ctx, tx.ID, TransactionPatch{Amount: &amt})
	if err != nil || !got.AmountARS.Equal(dec("30000")) || !got.FxRate.Equal(dec("1500")) {
		t.Fatalf("cambio de monto: %+v err=%v", got, err)
	}

	old := time.Date(2026, 10, 10, 18, 0, 0, 0, time.UTC)
	got, err = s.Update(ctx, tx.ID, TransactionPatch{OccurredAt: &old})
	if err != nil || !got.FxRate.Equal(dec("1400")) || !got.AmountARS.Equal(dec("28000")) {
		t.Fatalf("cambio de fecha: %+v err=%v", got, err)
	}

	ars := domain.ARS
	got, err = s.Update(ctx, tx.ID, TransactionPatch{Currency: &ars})
	if err != nil || !got.FxRate.Equal(dec("1")) || !got.AmountARS.Equal(dec("20")) {
		t.Fatalf("cambio a ARS: %+v err=%v", got, err)
	}

	desc := "cena"
	got, _ = s.Update(ctx, tx.ID, TransactionPatch{Description: &desc})
	if got.Description != "cena" || got.CreatedBy.ID != "u-1" {
		t.Fatalf("descripción / autor: %+v", got)
	}

	if _, err := s.Update(ctx, "nope", TransactionPatch{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("id inexistente: %v", err)
	}
	income := "cat-salary"
	var ve *ValidationError
	if _, err := s.Update(ctx, tx.ID, TransactionPatch{CategoryID: &income}); !errors.As(err, &ve) {
		t.Fatalf("categoría de otro tipo debería fallar: %v", err)
	}
}

func TestListTransactionsQuery(t *testing.T) {
	s, txs := newTxSvc(nil)
	month := time.Date(2026, 12, 1, 0, 0, 0, 0, time.FixedZone("AR", -3*3600))
	if _, err := s.List(context.Background(), ListTransactionsQuery{Month: &month, Limit: 5000, Offset: -3}); err != nil {
		t.Fatal(err)
	}
	f := txs.filter
	if f.Limit != MaxPageSize || f.Offset != 0 {
		t.Fatalf("paginado: limit=%d offset=%d", f.Limit, f.Offset)
	}
	if !f.From.Equal(month) || !f.To.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, month.Location())) {
		t.Fatalf("rango de mes (diciembre -> enero): %v - %v", f.From, f.To)
	}
	if _, err := s.List(context.Background(), ListTransactionsQuery{Limit: 0}); err != nil || txs.filter.Limit != DefaultPageSize {
		t.Fatalf("limit por defecto: %d", txs.filter.Limit)
	}
	bad := domain.Kind("x")
	var ve *ValidationError
	if _, err := s.List(context.Background(), ListTransactionsQuery{Kind: &bad}); !errors.As(err, &ve) {
		t.Fatalf("kind inválido: %v", err)
	}
}
