package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/adapters/postgres"
	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

func eq(t *testing.T, what string, got decimal.Decimal, want string) {
	t.Helper()
	if !got.Equal(d(want)) {
		t.Errorf("%s = %s, quería %s", what, got, want)
	}
}

func TestDashboard(t *testing.T) {
	pool, userID := testPool(t,
		`DELETE FROM transactions WHERE created_by = $1`,
		`DELETE FROM loans WHERE created_by = $1`,
		`DELETE FROM subscriptions WHERE created_by = $1`,
		`DELETE FROM cards WHERE created_by = $1`, // cascada: compras, cuotas, resúmenes
	)
	ctx := context.Background()

	now := ar(2026, 10, 15, 12, 0) // 15/10, 17 días restantes incluyendo hoy
	clock := func() time.Time { return now }
	fx := flatRate{sell: "1000"}
	catRepo := postgres.NewCategoryRepo(pool)
	txs := application.NewTransactionService(postgres.NewTransactionRepo(pool), catRepo, fx)
	cards := application.NewCardService(postgres.NewCardRepo(pool), catRepo, fx).WithClock(clock)
	loans := application.NewLoanService(postgres.NewLoanRepo(pool), fx).WithClock(clock)
	subs := application.NewSubscriptionService(postgres.NewSubscriptionRepo(pool), fx).WithClock(clock)
	dash := application.NewDashboardService(postgres.NewDashboardRepo(pool), cards, loans, subs).WithClock(clock)

	catID := func(name string) string {
		all, err := catRepo.List(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range all {
			if c.Name == name {
				return c.ID
			}
		}
		t.Fatalf("categoría %q inexistente", name)
		return ""
	}
	comida, transporte, salud, sueldo := catID("Comida"), catID("Transporte"), catID("Salud"), catID("Sueldo")

	tx := func(kind domain.Kind, cat, amount string, at time.Time) {
		t.Helper()
		if _, err := txs.Create(ctx, userID, application.TransactionInput{Kind: kind, Amount: d(amount), Currency: domain.ARS, CategoryID: cat, OccurredAt: &at}); err != nil {
			t.Fatal(err)
		}
	}
	purchase := func(cardID, cat, amount string, installments int, at time.Time) {
		t.Helper()
		if _, err := cards.CreatePurchase(ctx, userID, cardID, application.PurchaseInput{Amount: d(amount), Currency: domain.ARS, Installments: installments, CategoryID: cat, PurchasedAt: &at}); err != nil {
			t.Fatal(err)
		}
	}

	// Ingresos: 900.000 en septiembre y 1.000.000 en octubre.
	tx(domain.Income, sueldo, "900000", ar(2026, 9, 2, 10, 0))
	tx(domain.Income, sueldo, "1000000", ar(2026, 10, 1, 10, 0))

	// Compromisos de octubre: préstamo 100.000 + vehículo 200.000 + suscripción 50.000 (la pausada no suma).
	mustLoan := func(in application.LoanInput) {
		if _, err := loans.Create(ctx, userID, in); err != nil {
			t.Fatal(err)
		}
	}
	mustLoan(application.LoanInput{Name: "Préstamo", Kind: domain.LoanKindLoan, BaseAmount: d("100000"), Currency: domain.ARS, StartMonth: mustMonth(t, "2026-09"), EndMonth: mustMonth(t, "2026-12")})
	mustLoan(application.LoanInput{Name: "Taos", Kind: domain.LoanKindVehicle, BaseAmount: d("200000"), Currency: domain.ARS, StartMonth: mustMonth(t, "2026-10"), EndMonth: mustMonth(t, "2027-03")})
	if _, err := subs.Create(ctx, userID, application.SubscriptionInput{Name: "Netflix", Amount: d("50000"), Currency: domain.ARS, ChargeDay: 20}); err != nil {
		t.Fatal(err)
	}
	spotify, err := subs.Create(ctx, userID, application.SubscriptionInput{Name: "Spotify", Amount: d("10"), Currency: domain.USD, ChargeDay: 31})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := subs.SetActive(ctx, spotify.Subscription.ID, false); err != nil {
		t.Fatal(err)
	}

	// Tarjeta A (cierra 20, vence 5): compra del 10/09 en 3 cuotas de 30.000. El resumen de septiembre
	// vence el 5/10: 30.000 de cuota + 5.000 de saldo previo = 35.000 comprometidos en octubre.
	cardA, err := cards.Create(ctx, userID, application.CardInput{Name: "Visa", Last4: "4242", ClosingDay: 20, DueDay: 5})
	if err != nil {
		t.Fatal(err)
	}
	purchase(cardA.ID, comida, "90000", 3, ar(2026, 9, 10, 12, 0))
	if _, err := cards.SetAdjustment(ctx, cardA.ID, mustMonth(t, "2026-09"), d("5000")); err != nil {
		t.Fatal(err)
	}

	// Gastos variables: 100.000 el 3/10, 15.000 (Transporte) el 14/10 y 8.000 hoy. Septiembre: 20.000 de Salud.
	tx(domain.Expense, comida, "100000", ar(2026, 10, 3, 12, 0))
	tx(domain.Expense, transporte, "15000", ar(2026, 10, 14, 23, 59))
	tx(domain.Expense, comida, "8000", ar(2026, 10, 15, 8, 0))
	tx(domain.Expense, salud, "20000", ar(2026, 9, 20, 12, 0))

	t.Run("indicador diario", func(t *testing.T) {
		got, err := dash.Daily(ctx)
		if err != nil {
			t.Fatal(err)
		}
		eq(t, "ingresos", got.Income, "1000000")
		eq(t, "compromisos tarjeta", got.Commitments.Cards, "35000")
		eq(t, "compromisos préstamo", got.Commitments.Loans, "100000")
		eq(t, "compromisos vehículo", got.Commitments.Vehicle, "200000")
		eq(t, "compromisos suscripciones", got.Commitments.Subscriptions, "50000")
		eq(t, "presupuesto variable", got.VariableBudget, "615000")
		eq(t, "gastado hasta ayer", got.SpentUntilYesterday, "115000")
		eq(t, "límite", got.Limit, "29411.76") // 500.000 / 17
		eq(t, "gastado hoy", got.Spent, "8000")
		eq(t, "restante", got.Remaining, "21411.76")
		if got.OverLimit || got.RemainingDays != 17 || got.Date.Format("2006-01-02") != "2026-10-15" {
			t.Errorf("%+v", got)
		}
	})

	t.Run("una compra grande con tarjeta no cambia el límite ni lo gastado", func(t *testing.T) {
		before, _ := dash.Daily(ctx)
		// Tarjeta A, hoy (antes del cierre del 20): va al resumen que vence el 5/11, no a octubre.
		purchase(cardA.ID, comida, "5000000", 1, now)
		after, err := dash.Daily(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !after.Limit.Equal(before.Limit) || !after.Spent.Equal(before.Spent) || !after.Remaining.Equal(before.Remaining) ||
			!after.Commitments.Total().Equal(before.Commitments.Total()) || after.OverLimit {
			t.Fatalf("antes=%+v después=%+v", before, after)
		}
	})

	t.Run("una compra que cae en un resumen que vence este mes suma compromisos, no gasto variable", func(t *testing.T) {
		before, _ := dash.Daily(ctx)
		// Tarjeta B cierra el 5 y vence el 15: una compra del 3/10 va al resumen que vence el 15/10.
		cardB, err := cards.Create(ctx, userID, application.CardInput{Name: "Master", ClosingDay: 5, DueDay: 15})
		if err != nil {
			t.Fatal(err)
		}
		purchase(cardB.ID, comida, "1000", 1, ar(2026, 10, 3, 12, 0))
		after, _ := dash.Daily(ctx)
		eq(t, "tarjeta", after.Commitments.Cards, "36000")
		if !after.Spent.Equal(before.Spent) || !after.SpentUntilYesterday.Equal(before.SpentUntilYesterday) {
			t.Fatalf("el gasto variable no debe cambiar: antes=%s/%s después=%s/%s", before.SpentUntilYesterday, before.Spent, after.SpentUntilYesterday, after.Spent)
		}
	})

	t.Run("gastar de más hoy", func(t *testing.T) {
		// 15/10 22:00 en Mendoza = 16/10 01:00 UTC: sigue siendo hoy.
		tx(domain.Expense, comida, "30000", time.Date(2026, 10, 16, 1, 0, 0, 0, time.UTC))
		got, _ := dash.Daily(ctx)
		eq(t, "gastado hoy", got.Spent, "38000")
		// Con el compromiso de 1.000 que sumó la tarjeta Master: (614.000 − 115.000) / 17 = 29.352,94.
		eq(t, "límite", got.Limit, "29352.94")
		eq(t, "restante", got.Remaining, "-8647.06")
		eq(t, "cuánto se pasó", got.OverAmount, "8647.06")
		if !got.OverLimit {
			t.Error("debería estar pasado del límite")
		}
	})

	t.Run("último día del mes y mes sin ingresos", func(t *testing.T) {
		lastDay := application.NewDashboardService(postgres.NewDashboardRepo(pool), cards, loans, subs).WithClock(func() time.Time { return ar(2026, 10, 31, 23, 30) })
		got, err := lastDay.Daily(ctx)
		if err != nil {
			t.Fatal(err)
		}
		// Hasta ayer (30/10) se gastaron 100.000 + 15.000 + 8.000 + 30.000 + ... las compras con tarjeta no cuentan.
		if got.RemainingDays != 1 {
			t.Errorf("días restantes: %d", got.RemainingDays)
		}
		eq(t, "gastado hasta ayer", got.SpentUntilYesterday, "153000")

		nov := application.NewDashboardService(postgres.NewDashboardRepo(pool), cards, loans, subs).WithClock(func() time.Time { return ar(2026, 11, 10, 12, 0) })
		got, err = nov.Daily(ctx)
		if err != nil {
			t.Fatal(err)
		}
		eq(t, "ingresos de noviembre", got.Income, "0")
		if !got.OverLimit || !got.Limit.IsNegative() {
			t.Errorf("sin ingresos debería estar pasado: %+v", got)
		}
	})

	t.Run("métricas del mes", func(t *testing.T) {
		s, err := dash.Summary(ctx, mustMonth(t, "2026-10"))
		if err != nil {
			t.Fatal(err)
		}
		eq(t, "ingresos", s.Income, "1000000")
		eq(t, "ingresos mes anterior", s.PreviousIncome, "900000")
		// Octubre: gastos 100.000 + 15.000 + 8.000 + 30.000 y compras con tarjeta 5.000.000 + 1.000.
		eq(t, "egresos", s.Expenses, "5154000")
		eq(t, "egresos mes anterior", s.PreviousExpenses, "110000") // 20.000 de Salud + 90.000 de la compra con tarjeta
		eq(t, "compromiso tarjetas", s.Commitments.Cards, "36000")
		eq(t, "cuotas futuras", s.CardFutureInstallment, "5060000") // oct 30.000 + 5.000.000, nov 30.000

		type row struct{ name, total, prev string }
		want := []row{{"Comida", "5139000", "90000"}, {"Transporte", "15000", "0"}, {"Salud", "0", "20000"}}
		if len(s.SpendByCategory) != len(want) {
			t.Fatalf("categorías: %+v", s.SpendByCategory)
		}
		for i, w := range want {
			got := s.SpendByCategory[i]
			if got.Name != w.name || !got.Total.Equal(d(w.total)) || !got.PreviousTotal.Equal(d(w.prev)) {
				t.Errorf("categoría %d: %+v, quería %+v", i, got, w)
			}
		}
	})

	t.Run("próximos pagos", func(t *testing.T) {
		got, err := dash.Upcoming(ctx, 20)
		if err != nil {
			t.Fatal(err)
		}
		type row struct {
			typ  application.PaymentType
			name string
			date string
			amt  string
		}
		want := []row{
			{application.PaymentCardStatement, "Visa", "2026-10-05", "35000"},   // vencido y sin pagar
			{application.PaymentCardStatement, "Master", "2026-10-15", "1000"},  // vence hoy
			{application.PaymentSubscription, "Netflix", "2026-10-20", "50000"}, // la pausada no figura
			{application.PaymentLoan, "Préstamo", "2026-10-31", "100000"},
			{application.PaymentVehicle, "Taos", "2026-10-31", "200000"},
			{application.PaymentCardStatement, "Visa", "2026-11-05", "5030000"},
			{application.PaymentCardStatement, "Visa", "2026-12-05", "30000"},
		}
		if len(got) != len(want) {
			t.Fatalf("%d pagos: %+v", len(got), got)
		}
		for i, w := range want {
			g := got[i]
			if g.Type != w.typ || g.Name != w.name || g.Date.Format("2006-01-02") != w.date || !g.AmountARS.Equal(d(w.amt)) {
				t.Errorf("pago %d: %s %s %s %s, quería %+v", i, g.Type, g.Name, g.Date.Format("2006-01-02"), g.AmountARS, w)
			}
		}
		if limited, _ := dash.Upcoming(ctx, 2); len(limited) != 2 || limited[0].Name != "Visa" {
			t.Errorf("límite: %+v", limited)
		}

		// Pagar el resumen vencido lo saca de la lista, pero sigue contando como compromiso del mes.
		if _, err := cards.Pay(ctx, cardA.ID, mustMonth(t, "2026-09")); err != nil {
			t.Fatal(err)
		}
		after, _ := dash.Upcoming(ctx, 20)
		if len(after) != len(want)-1 || after[0].Name != "Master" {
			t.Errorf("tras pagar: %+v", after)
		}
		daily, _ := dash.Daily(ctx)
		eq(t, "compromiso tarjetas tras pagar", daily.Commitments.Cards, "36000")
	})
}
