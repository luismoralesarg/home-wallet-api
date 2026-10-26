package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/adapters/postgres"
	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
	"github.com/luismoralesarg/home-wallet-api/internal/ports"
	"github.com/luismoralesarg/home-wallet-api/internal/tz"
)

// Requiere una DB con las migraciones aplicadas: TEST_DATABASE_URL=postgres://... go test ./internal/adapters/postgres
func TestTransactionsAndCategories(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL no definida")
	}
	ctx := context.Background()
	pool, err := postgres.NewPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close) // se registra primero: t.Cleanup corre en orden inverso

	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO users (username, password_hash) VALUES ('it-'||gen_random_uuid(), 'x') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []string{`DELETE FROM transactions WHERE created_by = $1`, `DELETE FROM users WHERE id = $1`} {
			if _, err := pool.Exec(ctx, q, userID); err != nil {
				t.Errorf("limpieza: %v", err)
			}
		}
	})

	cats := postgres.NewCategoryRepo(pool)
	txs := postgres.NewTransactionRepo(pool)

	expense := domain.Expense
	list, err := cats.List(ctx, &expense)
	if err != nil || len(list) == 0 {
		t.Fatalf("categorías de gasto: %v %v", list, err)
	}
	food := list[0]

	// Categorías: crear, duplicado, renombrar, borrar.
	c, err := cats.Create(ctx, domain.Category{Name: "it-" + userID, Kind: domain.Expense})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cats.Create(ctx, domain.Category{Name: c.Name, Kind: domain.Expense}); err != application.ErrConflict {
		t.Fatalf("duplicado: %v", err)
	}
	if c, err = cats.Update(ctx, c.ID, c.Name+"-b"); err != nil || c.Name != "it-"+userID+"-b" {
		t.Fatalf("update: %+v %v", c, err)
	}

	mk := func(at time.Time, usd bool) domain.Transaction {
		in := domain.Transaction{Kind: domain.Expense, Amount: decimal.RequireFromString("10.50"), Currency: domain.ARS,
			FxRate: decimal.NewFromInt(1), AmountARS: decimal.RequireFromString("10.50"), CategoryID: food.ID,
			Description: "t", OccurredAt: at, CreatedBy: domain.UserRef{ID: userID}}
		if usd {
			in.Currency, in.FxRate, in.AmountARS = domain.USD, decimal.RequireFromString("1450.33"), decimal.RequireFromString("15228.47")
		}
		out, err := txs.Create(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	// 31/10 23:30 Mendoza (= 01/11 02:30 UTC) debe caer en octubre; 01/11 00:30 Mendoza en noviembre.
	oct := mk(time.Date(2026, 11, 1, 2, 30, 0, 0, time.UTC), true)
	nov := mk(time.Date(2026, 11, 1, 3, 30, 0, 0, time.UTC), false)

	if oct.CreatedBy.Name == "" || !oct.FxRate.Equal(decimal.RequireFromString("1450.33")) || !oct.AmountARS.Equal(decimal.RequireFromString("15228.47")) {
		t.Fatalf("roundtrip de decimales: %+v", oct)
	}

	month := func(y int, m time.Month) *time.Time {
		v := time.Date(y, m, 1, 0, 0, 0, 0, tz.Mendoza)
		return &v
	}
	inMonth := func(m time.Time) ports.TransactionFilter {
		to := m.AddDate(0, 1, 0)
		return ports.TransactionFilter{From: &m, To: &to, UserID: &userID, Limit: 50}
	}
	items, total, err := txs.List(ctx, inMonth(*month(2026, 10)))
	if err != nil || total != 1 || items[0].ID != oct.ID {
		t.Fatalf("octubre: total=%d items=%v err=%v", total, items, err)
	}
	items, total, _ = txs.List(ctx, inMonth(*month(2026, 11)))
	if total != 1 || items[0].ID != nov.ID {
		t.Fatalf("noviembre: total=%d", total)
	}

	// Paginado: total cuenta todo aunque la página sea chica.
	items, total, _ = txs.List(ctx, ports.TransactionFilter{UserID: &userID, Limit: 1})
	if total != 2 || len(items) != 1 {
		t.Fatalf("paginado: total=%d len=%d", total, len(items))
	}
	income := domain.Income
	_, total, _ = txs.List(ctx, ports.TransactionFilter{UserID: &userID, Kind: &income, Limit: 10})
	if total != 0 {
		t.Fatalf("filtro por kind: %d", total)
	}

	// Una categoría en uso no se puede borrar; una libre sí.
	if err := cats.Delete(ctx, food.ID); err != application.ErrConflict {
		t.Fatalf("borrar categoría en uso: %v", err)
	}
	if err := cats.Delete(ctx, c.ID); err != nil {
		t.Fatalf("borrar categoría libre: %v", err)
	}
	if err := cats.Delete(ctx, c.ID); err != application.ErrNotFound {
		t.Fatalf("borrar de nuevo: %v", err)
	}

	oct.Description = "editada"
	if got, err := txs.Update(ctx, oct); err != nil || got.Description != "editada" {
		t.Fatalf("update: %+v %v", got, err)
	}
	if err := txs.Delete(ctx, nov.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := txs.Get(ctx, nov.ID); err != application.ErrNotFound {
		t.Fatalf("get borrada: %v", err)
	}
}
