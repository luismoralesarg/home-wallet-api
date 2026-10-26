package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/adapters/postgres"
	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type flatRate struct{ sell string }

func (f flatRate) ConvertToARS(_ context.Context, amount decimal.Decimal, cur domain.Currency, _ time.Time) (decimal.Decimal, decimal.Decimal, error) {
	if cur == domain.ARS {
		return decimal.NewFromInt(1), amount, nil
	}
	fx := decimal.RequireFromString(f.sell)
	return fx, amount.Mul(fx).Round(2), nil
}

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func ar(y int, m time.Month, day, h, min int) time.Time {
	return time.Date(y, m, day, h, min, 0, 0, time.FixedZone("AR", -3*3600))
}

func ymd(t time.Time) string { return t.Format("2006-01-02") }

func TestCardsEndToEnd(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL no definida")
	}
	ctx := context.Background()
	pool, err := postgres.NewPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close) // primero: t.Cleanup corre en orden inverso

	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO users (username, password_hash) VALUES ('it-card-'||gen_random_uuid(), 'x') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Las tarjetas borran en cascada compras, cuotas y resúmenes.
		for _, q := range []string{`DELETE FROM cards WHERE created_by = $1`, `DELETE FROM users WHERE id = $1`} {
			if _, err := pool.Exec(ctx, q, userID); err != nil {
				t.Errorf("limpieza: %v", err)
			}
		}
	})

	cats := postgres.NewCategoryRepo(pool)
	expense := domain.Expense
	list, err := cats.List(ctx, &expense)
	if err != nil || len(list) == 0 {
		t.Fatal("sin categorías de gasto", err)
	}
	food := list[0].ID
	income := domain.Income
	incomeCats, _ := cats.List(ctx, &income)

	now := ar(2026, 10, 25, 12, 0) // hoy: después del cierre de octubre (día 20)
	svc := application.NewCardService(postgres.NewCardRepo(pool), cats, flatRate{sell: "1000"}).WithClock(func() time.Time { return now })

	card, err := svc.Create(ctx, userID, application.CardInput{Name: "Visa", Last4: "4242", ClosingDay: 20, DueDay: 5})
	if err != nil {
		t.Fatal(err)
	}

	// Validaciones.
	var ve *application.ValidationError
	if _, err := svc.Create(ctx, userID, application.CardInput{Name: "x", ClosingDay: 0, DueDay: 5}); !errors.As(err, &ve) {
		t.Fatalf("closing_day 0: %v", err)
	}
	if _, err := svc.CreatePurchase(ctx, userID, card.ID, application.PurchaseInput{Amount: d("10"), Currency: domain.ARS, Installments: 61, CategoryID: food}); !errors.As(err, &ve) {
		t.Fatalf("61 cuotas: %v", err)
	}
	if _, err := svc.CreatePurchase(ctx, userID, card.ID, application.PurchaseInput{Amount: d("10"), Currency: domain.ARS, CategoryID: incomeCats[0].ID}); !errors.As(err, &ve) {
		t.Fatalf("categoría de ingreso: %v", err)
	}

	// Compra en 12 cuotas después del cierre → primera cuota en el resumen de noviembre.
	at := now
	p12, err := svc.CreatePurchase(ctx, userID, card.ID, application.PurchaseInput{Amount: d("1200.00"), Currency: domain.ARS, Installments: 12, CategoryID: food, Description: "heladera", PurchasedAt: &at})
	if err != nil {
		t.Fatal(err)
	}
	if p12.FirstStatement.String() != "2026-11" || ymd(p12.StatementDueDate) != "2026-12-05" {
		t.Fatalf("12 cuotas: resumen %s vence %s", p12.FirstStatement, ymd(p12.StatementDueDate))
	}

	// Compra de contado antes del cierre (10/10) → resumen de octubre, vence el 5/11.
	early := ar(2026, 10, 10, 9, 0)
	cash, err := svc.CreatePurchase(ctx, userID, card.ID, application.PurchaseInput{Amount: d("500.00"), Currency: domain.ARS, CategoryID: food, PurchasedAt: &early})
	if err != nil {
		t.Fatal(err)
	}
	if cash.Installments != 1 || cash.FirstStatement.String() != "2026-10" || ymd(cash.StatementDueDate) != "2026-11-05" {
		t.Fatalf("contado: %+v", cash)
	}

	// Vista general.
	ov, err := svc.Get(ctx, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ov.OpenStatement.Month.String() != "2026-11" || !ov.OpenStatement.Total.Equal(d("100")) {
		t.Fatalf("abierto: %+v", ov.OpenStatement)
	}
	if len(ov.ClosedUnpaid) != 1 || ov.ClosedUnpaid[0].Month.String() != "2026-10" || !ov.ClosedUnpaid[0].Total.Equal(d("500")) {
		t.Fatalf("cerrados sin pagar: %+v", ov.ClosedUnpaid)
	}
	if len(ov.Upcoming) != 3 || ov.Upcoming[0].Month.String() != "2026-12" || !ov.Upcoming[2].Total.Equal(d("100")) {
		t.Fatalf("próximos: %+v", ov.Upcoming)
	}
	if !ov.FutureInstallmentsARS.Equal(d("1200")) { // las 12 cuotas de noviembre en adelante; el contado de octubre ya cerró
		t.Fatalf("cuotas futuras: %s", ov.FutureInstallmentsARS)
	}

	// Detalle del resumen de octubre, ajuste de saldo previo y pago.
	oct := domain.Month{Year: 2026, Mon: time.October}
	det, err := svc.Statement(ctx, card.ID, oct)
	if err != nil || len(det.Items) != 1 || det.Items[0].PurchaseID != cash.ID || !det.Total.Equal(d("500")) {
		t.Fatalf("detalle: %+v %v", det, err)
	}
	det, err = svc.SetAdjustment(ctx, card.ID, oct, d("250.50"))
	if err != nil || !det.Total.Equal(d("750.50")) || !det.Adjustment.Equal(d("250.50")) {
		t.Fatalf("ajuste: %+v %v", det, err)
	}
	det, err = svc.Pay(ctx, card.ID, oct)
	if err != nil || !det.Paid || det.PaidAt == nil {
		t.Fatalf("pago: %+v %v", det, err)
	}
	firstPaidAt := *det.PaidAt
	now = now.Add(time.Hour)
	if det, _ = svc.Pay(ctx, card.ID, oct); !det.PaidAt.Equal(firstPaidAt) {
		t.Fatalf("pagar dos veces debe conservar la fecha original: %v vs %v", det.PaidAt, firstPaidAt)
	}
	if ov, _ = svc.Get(ctx, card.ID); len(ov.ClosedUnpaid) != 0 {
		t.Fatalf("pagado no debería figurar como pendiente: %+v", ov.ClosedUnpaid)
	}
	// Un mes sin movimientos se puede consultar y ajustar (saldo previo al arrancar la app).
	empty := domain.Month{Year: 2026, Mon: time.September}
	if det, err = svc.SetAdjustment(ctx, card.ID, empty, d("-100")); err != nil || !det.Total.Equal(d("-100")) || len(det.Items) != 0 {
		t.Fatalf("ajuste en mes vacío: %+v %v", det, err)
	}

	// Editar la compra recalcula las cuotas: 900 en 3 cuotas de 300 (nov, dic, ene).
	amt, inst := d("900"), 3
	upd, err := svc.UpdatePurchase(ctx, card.ID, p12.ID, application.PurchasePatch{Amount: &amt, Installments: &inst})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Installments != 3 {
		t.Fatalf("cuotas: %d", upd.Installments)
	}
	ov, _ = svc.Get(ctx, card.ID)
	if !ov.OpenStatement.Total.Equal(d("300")) || !ov.Upcoming[0].Total.Equal(d("300")) || !ov.Upcoming[1].Total.Equal(d("300")) || !ov.Upcoming[2].Total.IsZero() || !ov.FutureInstallmentsARS.Equal(d("900")) {
		t.Fatalf("tras editar: abierto=%s próximos=%+v futuro=%s", ov.OpenStatement.Total, ov.Upcoming, ov.FutureInstallmentsARS)
	}

	// Compra en USD: guarda cotización y monto en ARS; 10 USD en 3 cuotas = 3333.33 + 3333.33 + 3333.34.
	usd, err := svc.CreatePurchase(ctx, userID, card.ID, application.PurchaseInput{Amount: d("10"), Currency: domain.USD, Installments: 3, CategoryID: food, PurchasedAt: &at})
	if err != nil {
		t.Fatal(err)
	}
	if !usd.FxRate.Equal(d("1000")) || !usd.AmountARS.Equal(d("10000")) {
		t.Fatalf("USD: fx=%s ars=%s", usd.FxRate, usd.AmountARS)
	}
	nov := domain.Month{Year: 2026, Mon: time.November}
	jan := domain.Month{Year: 2027, Mon: time.January}
	if det, _ = svc.Statement(ctx, card.ID, nov); !det.InstallmentsTotal.Equal(d("3633.33")) {
		t.Fatalf("noviembre: %s", det.InstallmentsTotal)
	}
	if det, _ = svc.Statement(ctx, card.ID, jan); !det.InstallmentsTotal.Equal(d("3633.34")) {
		t.Fatalf("enero: %s", det.InstallmentsTotal)
	}

	// Listado de compras: el mes se interpreta en Mendoza.
	octStart := ar(2026, 10, 1, 0, 0)
	page, err := svc.ListPurchases(ctx, application.ListPurchasesQuery{Month: &octStart, CardID: &card.ID, Limit: 50})
	if err != nil || page.Total != 3 || page.Items[0].ID == "" {
		t.Fatalf("listado de octubre: total=%d %v", page.Total, err)
	}
	novStart := ar(2026, 11, 1, 0, 0)
	if page, _ = svc.ListPurchases(ctx, application.ListPurchasesQuery{Month: &novStart, CardID: &card.ID}); page.Total != 0 {
		t.Fatalf("listado de noviembre: %d", page.Total)
	}

	// Borrar: la tarjeta con compras no se puede borrar; sin compras sí.
	if err := svc.Delete(ctx, card.ID); !errors.Is(err, application.ErrConflict) {
		t.Fatalf("borrar con compras: %v", err)
	}
	for _, id := range []string{p12.ID, cash.ID, usd.ID} {
		if err := svc.DeletePurchase(ctx, card.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.DeletePurchase(ctx, card.ID, cash.ID); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("borrar de nuevo: %v", err)
	}
	if ov, _ = svc.Get(ctx, card.ID); !ov.FutureInstallmentsARS.IsZero() {
		t.Fatalf("sin compras no debería quedar deuda: %s", ov.FutureInstallmentsARS)
	}
	if err := svc.Delete(ctx, card.ID); err != nil {
		t.Fatalf("borrar sin compras: %v", err)
	}
	if _, err := svc.Get(ctx, card.ID); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("get borrada: %v", err)
	}
}
