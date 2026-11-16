package domain

import (
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func daily(today time.Time, income, commitments, untilYesterday, spentToday string) DailyIndicator {
	return ComputeDaily(DailyInput{
		Today:               today,
		Income:              d(income),
		Commitments:         Commitments{Cards: d(commitments)},
		SpentUntilYesterday: d(untilYesterday),
		SpentToday:          d(spentToday),
	})
}

func TestRemainingDays(t *testing.T) {
	cases := []struct {
		today time.Time
		want  int
	}{
		{day(2026, 10, 1), 31},
		{day(2026, 10, 15), 17},
		{day(2026, 10, 31), 1},
		{day(2026, 2, 28), 1},
		{day(2028, 2, 28), 2}, // bisiesto
		{day(2026, 11, 30), 1},
	}
	for _, c := range cases {
		if got := RemainingDays(c.today); got != c.want {
			t.Errorf("%s: %d, quería %d", c.today.Format("2006-01-02"), got, c.want)
		}
	}
}

func TestDailyWithoutExpenses(t *testing.T) {
	// 1.000.000 de ingresos − 385.000 de compromisos = 615.000 / 17 días.
	got := daily(day(2026, 10, 15), "1000000", "385000", "0", "0")
	if !got.VariableBudget.Equal(d("615000")) || got.RemainingDays != 17 {
		t.Fatalf("presupuesto=%s días=%d", got.VariableBudget, got.RemainingDays)
	}
	if !got.Limit.Equal(d("36176.47")) || !got.Remaining.Equal(d("36176.47")) || got.OverLimit || !got.OverAmount.IsZero() {
		t.Fatalf("%+v", got)
	}
}

func TestDailySpendingBelowLimit(t *testing.T) {
	got := daily(day(2026, 10, 15), "1000000", "385000", "115000", "8000")
	// (615.000 − 115.000) / 17 = 29.411,76
	if !got.Limit.Equal(d("29411.76")) || !got.Remaining.Equal(d("21411.76")) || got.OverLimit {
		t.Fatalf("%+v", got)
	}
}

func TestDailySpendingAboveLimit(t *testing.T) {
	got := daily(day(2026, 10, 15), "1000000", "385000", "115000", "38000")
	if !got.Remaining.Equal(d("-8588.24")) || !got.OverLimit || !got.OverAmount.Equal(d("8588.24")) {
		t.Fatalf("%+v", got)
	}
}

func TestDailyLimitDoesNotMoveWhileLoadingTodaysExpenses(t *testing.T) {
	base := daily(day(2026, 10, 15), "1000000", "385000", "115000", "0")
	after := daily(day(2026, 10, 15), "1000000", "385000", "115000", "20000")
	if !base.Limit.Equal(after.Limit) {
		t.Fatalf("el límite se movió: %s → %s", base.Limit, after.Limit)
	}
	if !after.Remaining.Equal(base.Remaining.Sub(d("20000"))) {
		t.Fatalf("restante: %s", after.Remaining)
	}
}

func TestDailyTomorrowAdjustsToTodaysOverspending(t *testing.T) {
	// Si hoy se gasta de más, mañana el límite baja; si se gasta de menos, sube.
	today := daily(day(2026, 10, 15), "1000000", "385000", "115000", "0")
	over := daily(day(2026, 10, 16), "1000000", "385000", "215000", "0")  // ayer se gastaron 100.000
	under := daily(day(2026, 10, 16), "1000000", "385000", "115000", "0") // ayer no se gastó nada
	if !over.Limit.LessThan(today.Limit) {
		t.Fatalf("pasarse de más debería bajar el límite de mañana: %s vs %s", over.Limit, today.Limit)
	}
	if !under.Limit.GreaterThan(today.Limit) {
		t.Fatalf("gastar de menos debería subir el límite de mañana: %s vs %s", under.Limit, today.Limit)
	}
}

func TestDailyLastDayOfMonth(t *testing.T) {
	// Un solo día: el límite es todo lo que queda del presupuesto.
	got := daily(day(2026, 10, 31), "1000000", "385000", "590000", "0")
	if got.RemainingDays != 1 || !got.Limit.Equal(d("25000")) || got.OverLimit {
		t.Fatalf("%+v", got)
	}
}

func TestDailyMonthWithoutIncome(t *testing.T) {
	// Sin ingresos el presupuesto es negativo: ya se pasó antes de gastar nada.
	got := daily(day(2026, 11, 10), "0", "35000", "0", "0")
	if !got.VariableBudget.Equal(d("-35000")) || !got.Limit.IsNegative() || !got.OverLimit || !got.OverAmount.IsPositive() {
		t.Fatalf("%+v", got)
	}
}

func TestDailyCommitmentsTotal(t *testing.T) {
	c := Commitments{Cards: d("35000"), Loans: d("100000"), Vehicle: d("200000"), Subscriptions: d("50000")}
	if !c.Total().Equal(d("385000")) {
		t.Fatal(c.Total())
	}
}
