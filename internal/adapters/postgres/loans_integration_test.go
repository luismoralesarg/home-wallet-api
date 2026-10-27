package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/luismoralesarg/home-wallet-api/internal/adapters/postgres"
	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

// testPool conecta a TEST_DATABASE_URL y crea un usuario de prueba que se borra al terminar.
// Todo lo que cuelga del usuario (préstamos, suscripciones) debe borrarse en `cleanup` antes.
func testPool(t *testing.T, cleanup ...string) (*pgxpool.Pool, string) {
	t.Helper()
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
		for _, q := range append(cleanup, `DELETE FROM users WHERE id = $1`) {
			if _, err := pool.Exec(ctx, q, userID); err != nil {
				t.Errorf("limpieza: %v", err)
			}
		}
	})
	return pool, userID
}

func mustMonth(t *testing.T, s string) domain.Month {
	t.Helper()
	m, err := domain.ParseMonth(s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestLoans(t *testing.T) {
	pool, userID := testPool(t, `DELETE FROM loans WHERE created_by = $1`)
	ctx := context.Background()
	repo := postgres.NewLoanRepo(pool)
	now := ar(2026, 10, 25, 12, 0)
	clock := func() time.Time { return now }
	svc := application.NewLoanService(repo, flatRate{sell: "1000"}).WithClock(clock)

	var ve *application.ValidationError
	good := application.LoanInput{Name: "Taos", Kind: domain.LoanKindVehicle, BaseAmount: d("500000"), Currency: domain.ARS, StartMonth: mustMonth(t, "2026-08"), EndMonth: mustMonth(t, "2027-01")}
	for name, mutate := range map[string]func(*application.LoanInput){
		"fin anterior al inicio": func(i *application.LoanInput) { i.EndMonth = mustMonth(t, "2026-07") },
		"demasiadas cuotas":      func(i *application.LoanInput) { i.EndMonth = mustMonth(t, "2056-09") }, // 361
		"tipo inválido":          func(i *application.LoanInput) { i.Kind = "otro" },
		"cuota en cero":          func(i *application.LoanInput) { i.BaseAmount = d("0") },
		"3 decimales":            func(i *application.LoanInput) { i.BaseAmount = d("10.123") },
		"sin nombre":             func(i *application.LoanInput) { i.Name = "  " },
	} {
		in := good
		mutate(&in)
		if _, err := svc.Create(ctx, userID, in); !errors.As(err, &ve) {
			t.Fatalf("%s: esperaba ValidationError, got %v", name, err)
		}
	}

	// Taos: 6 cuotas de ago-2026 a ene-2027. Hoy es octubre.
	taos, err := svc.Create(ctx, userID, good)
	if err != nil {
		t.Fatal(err)
	}
	if len(taos.Installments) != 6 || taos.Current == nil || taos.Current.Month.String() != "2026-10" {
		t.Fatalf("cuotas=%d actual=%+v", len(taos.Installments), taos.Current)
	}
	if taos.RemainingCount != 4 || !taos.RemainingTotalARS.Equal(d("2000000")) { // oct, nov, dic, ene
		t.Fatalf("restantes=%d total=%s", taos.RemainingCount, taos.RemainingTotalARS)
	}

	// Pagar la de octubre baja las restantes.
	paid := true
	if _, err := svc.UpdateInstallment(ctx, taos.Loan.ID, mustMonth(t, "2026-10"), application.InstallmentPatch{Paid: &paid}); err != nil {
		t.Fatal(err)
	}
	// Ajustar una cuota puntual (noviembre).
	nov := d("520000")
	got, err := svc.UpdateInstallment(ctx, taos.Loan.ID, mustMonth(t, "2026-11"), application.InstallmentPatch{Amount: &nov})
	if err != nil || !got.Amount.Equal(nov) || !got.AmountARS.Equal(nov) {
		t.Fatalf("cuota puntual: %+v %v", got, err)
	}
	if _, err := svc.UpdateInstallment(ctx, taos.Loan.ID, mustMonth(t, "2030-01"), application.InstallmentPatch{Paid: &paid}); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("mes fuera del préstamo: %v", err)
	}

	// +10% desde diciembre: afecta dic y ene, no nov ni lo pagado.
	pct := d("10")
	v, err := svc.Adjust(ctx, taos.Loan.ID, application.AdjustInput{FromMonth: mustMonth(t, "2026-12"), Percent: &pct})
	if err != nil {
		t.Fatal(err)
	}
	amounts := map[string]string{}
	for _, in := range v.Installments {
		amounts[in.Month.String()] = in.Amount.StringFixed(2)
	}
	want := map[string]string{"2026-08": "500000.00", "2026-09": "500000.00", "2026-10": "500000.00", "2026-11": "520000.00", "2026-12": "550000.00", "2027-01": "550000.00"}
	for m, a := range want {
		if amounts[m] != a {
			t.Fatalf("cuota %s = %s, quería %s (todas: %v)", m, amounts[m], a, amounts)
		}
	}
	if v.RemainingCount != 3 || !v.RemainingTotalARS.Equal(d("1620000")) {
		t.Fatalf("restantes=%d total=%s", v.RemainingCount, v.RemainingTotalARS)
	}

	// Un ajuste por monto desde noviembre baja las cuotas sin pagar de ahí en adelante.
	delta := d("-100000")
	v, err = svc.Adjust(ctx, taos.Loan.ID, application.AdjustInput{FromMonth: mustMonth(t, "2026-11"), Amount: &delta})
	if err != nil || !v.Installments[3].Amount.Equal(d("420000")) {
		t.Fatalf("ajuste por monto: %v %+v", err, v.Installments)
	}
	// Un ajuste inválido no modifica nada (todo o nada).
	huge := d("-9999999")
	if _, err := svc.Adjust(ctx, taos.Loan.ID, application.AdjustInput{FromMonth: mustMonth(t, "2026-11"), Amount: &huge}); !errors.As(err, &ve) {
		t.Fatalf("ajuste que deja cuotas negativas: %v", err)
	}
	after, _ := svc.Get(ctx, taos.Loan.ID)
	if !after.Installments[3].Amount.Equal(d("420000")) {
		t.Fatalf("un ajuste rechazado no debe modificar cuotas: %s", after.Installments[3].Amount)
	}
	if _, err := svc.Adjust(ctx, taos.Loan.ID, application.AdjustInput{FromMonth: mustMonth(t, "2028-01"), Percent: &pct}); !errors.As(err, &ve) {
		t.Fatalf("sin cuotas desde ese mes: %v", err)
	}
	if _, err := svc.Adjust(ctx, taos.Loan.ID, application.AdjustInput{FromMonth: mustMonth(t, "2026-11"), Percent: &pct, Amount: &delta}); !errors.As(err, &ve) {
		t.Fatalf("percent y amount juntos: %v", err)
	}
	if _, err := svc.Adjust(ctx, taos.Loan.ID, application.AdjustInput{FromMonth: mustMonth(t, "2026-11")}); !errors.As(err, &ve) {
		t.Fatalf("ni percent ni amount: %v", err)
	}

	// Préstamo en USD: 100 USD x 3 meses a 1000 → 100000 ARS por cuota.
	usd, err := svc.Create(ctx, userID, application.LoanInput{Name: "Préstamo USD", Kind: domain.LoanKindLoan, BaseAmount: d("100"), Currency: domain.USD, StartMonth: mustMonth(t, "2026-10"), EndMonth: mustMonth(t, "2026-12")})
	if err != nil {
		t.Fatal(err)
	}
	if !usd.FxRate.Equal(d("1000")) || !usd.BaseAmountARS.Equal(d("100000")) || !usd.RemainingTotalARS.Equal(d("300000")) {
		t.Fatalf("USD: fx=%s base=%s restante=%s", usd.FxRate, usd.BaseAmountARS, usd.RemainingTotalARS)
	}
	if _, err := svc.UpdateInstallment(ctx, usd.Loan.ID, mustMonth(t, "2026-10"), application.InstallmentPatch{Paid: &paid}); err != nil {
		t.Fatal(err)
	}
	// El dólar sube a 1200: las cuotas sin pagar se revalúan; la pagada conserva el valor del día que se pagó.
	svc2 := application.NewLoanService(repo, flatRate{sell: "1200"}).WithClock(clock)
	v, err = svc2.Get(ctx, usd.Loan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !v.RemainingTotalARS.Equal(d("240000")) || !v.Installments[0].AmountARS.Equal(d("100000")) || !v.Installments[1].AmountARS.Equal(d("120000")) {
		t.Fatalf("revaluación: restante=%s cuotas=%+v", v.RemainingTotalARS, v.Installments)
	}

	// Un aumento que alcanza a una cuota pagada no la toca: octubre (pagada) queda en 100, nov y dic suben 10%.
	v, err = svc2.Adjust(ctx, usd.Loan.ID, application.AdjustInput{FromMonth: mustMonth(t, "2026-10"), Percent: &pct})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{v.Installments[0].Amount.StringFixed(2), v.Installments[1].Amount.StringFixed(2), v.Installments[2].Amount.StringFixed(2)}; got[0] != "100.00" || got[1] != "110.00" || got[2] != "110.00" {
		t.Fatalf("ajuste con una cuota pagada en el rango: %v", got)
	}

	// Listado, renombrar y borrar.
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mine := 0
	for _, l := range list {
		if l.Loan.CreatedBy.ID == userID {
			mine++
		}
	}
	if mine != 2 {
		t.Fatalf("listado: %d", mine)
	}
	name := "Taos 2.0"
	if r, err := svc.Rename(ctx, taos.Loan.ID, &name); err != nil || r.Loan.Name != name {
		t.Fatalf("renombrar: %v", err)
	}
	if err := svc.Delete(ctx, taos.Loan.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, taos.Loan.ID); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("get borrado: %v", err)
	}
	var left int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM loan_installments WHERE loan_id = $1`, taos.Loan.ID).Scan(&left)
	if left != 0 {
		t.Fatalf("las cuotas deben borrarse en cascada: %d", left)
	}
}

func TestSubscriptions(t *testing.T) {
	pool, userID := testPool(t, `DELETE FROM subscriptions WHERE created_by = $1`)
	ctx := context.Background()
	now := ar(2026, 10, 25, 12, 0)
	svc := application.NewSubscriptionService(postgres.NewSubscriptionRepo(pool), flatRate{sell: "1000"}).WithClock(func() time.Time { return now })

	var ve *application.ValidationError
	if _, err := svc.Create(ctx, userID, application.SubscriptionInput{Name: "x", Amount: d("10"), Currency: domain.ARS, ChargeDay: 0}); !errors.As(err, &ve) {
		t.Fatalf("día 0: %v", err)
	}
	if _, err := svc.Create(ctx, userID, application.SubscriptionInput{Name: "x", Amount: d("0"), Currency: domain.ARS, ChargeDay: 5}); !errors.As(err, &ve) {
		t.Fatalf("monto 0: %v", err)
	}

	netflix, err := svc.Create(ctx, userID, application.SubscriptionInput{Name: "Netflix", Amount: d("1000"), Currency: domain.ARS, ChargeDay: 20})
	if err != nil {
		t.Fatal(err)
	}
	spotify, err := svc.Create(ctx, userID, application.SubscriptionInput{Name: "Spotify", Amount: d("10"), Currency: domain.USD, ChargeDay: 31})
	if err != nil {
		t.Fatal(err)
	}
	if !netflix.Subscription.Active || netflix.NextChargeDate.Format("2006-01-02") != "2026-11-20" || spotify.NextChargeDate.Format("2006-01-02") != "2026-10-31" {
		t.Fatalf("próximos cobros: %s / %s", netflix.NextChargeDate, spotify.NextChargeDate)
	}
	if !spotify.AmountARS.Equal(d("10000")) || !spotify.FxRate.Equal(d("1000")) {
		t.Fatalf("USD: %s %s", spotify.FxRate, spotify.AmountARS)
	}

	total := func() string {
		l, err := svc.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return l.MonthlyTotalARS.StringFixed(2)
	}
	if got := total(); got != "11000.00" {
		t.Fatalf("total: %s", got)
	}
	if v, err := svc.SetActive(ctx, spotify.Subscription.ID, false); err != nil || v.Subscription.Active {
		t.Fatalf("pausar: %+v %v", v, err)
	}
	if got := total(); got != "1000.00" { // la pausada no suma
		t.Fatalf("total con una pausada: %s", got)
	}
	if v, err := svc.SetActive(ctx, spotify.Subscription.ID, true); err != nil || !v.Subscription.Active {
		t.Fatalf("reactivar: %+v %v", v, err)
	}

	amt, day := d("1500"), 5
	if v, err := svc.Update(ctx, netflix.Subscription.ID, application.SubscriptionPatch{Amount: &amt, ChargeDay: &day}); err != nil || !v.AmountARS.Equal(d("1500")) || v.NextChargeDate.Format("2006-01-02") != "2026-11-05" {
		t.Fatalf("editar: %+v %v", v, err)
	}
	bad := 40
	if _, err := svc.Update(ctx, netflix.Subscription.ID, application.SubscriptionPatch{ChargeDay: &bad}); !errors.As(err, &ve) {
		t.Fatalf("editar con día 40: %v", err)
	}

	if err := svc.Delete(ctx, netflix.Subscription.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, netflix.Subscription.ID); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("get borrada: %v", err)
	}
}
