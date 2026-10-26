package domain

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// at crea un instante a partir de una hora de pared en Mendoza (UTC-3).
func at(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, time.FixedZone("AR", -3*3600))
}

func ymd(t time.Time) string { return t.Format("2006-01-02") }

func TestStatementMonthFor(t *testing.T) {
	card := Card{ClosingDay: 20, DueDay: 5}
	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{"antes del cierre", at(2026, 10, 10, 12, 0), "2026-10"},
		{"primer día del mes", at(2026, 10, 1, 0, 0), "2026-10"},
		{"el día del cierre a primera hora", at(2026, 10, 20, 0, 1), "2026-10"},
		{"el día del cierre a última hora (inclusive)", at(2026, 10, 20, 23, 59), "2026-10"},
		{"el día después del cierre", at(2026, 10, 21, 0, 0), "2026-11"},
		{"fin de mes", at(2026, 10, 31, 23, 59), "2026-11"},
		{"después del cierre en diciembre cruza de año", at(2026, 12, 28, 10, 0), "2027-01"},
		// 01:00 UTC del 21 son las 22:00 del 20 en Mendoza: todavía es el día de cierre.
		{"borde UTC/Mendoza", time.Date(2026, 10, 21, 1, 0, 0, 0, time.UTC), "2026-10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := card.StatementMonthFor(tc.at).String(); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestDueDate(t *testing.T) {
	cases := []struct {
		name         string
		closing, due int
		month        Month
		want         string
	}{
		{"vence el mes siguiente (due_day < closing_day)", 25, 5, Month{2026, 10}, "2026-11-05"},
		{"vence el mismo mes (due_day > closing_day)", 5, 15, Month{2026, 10}, "2026-10-15"},
		{"due_day igual a closing_day vence el mes siguiente (estrictamente posterior)", 10, 10, Month{2026, 10}, "2026-11-10"},
		{"cruza de año", 20, 5, Month{2026, 12}, "2027-01-05"},
		{"cierre el 31 en febrero se ajusta a fin de mes", 31, 10, Month{2026, 2}, "2026-03-10"},
		{"vencimiento el 31 en un mes corto", 5, 31, Month{2026, 2}, "2026-02-28"},
		{"vencimiento el 31 en año bisiesto", 5, 31, Month{2028, 2}, "2028-02-29"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Card{ClosingDay: tc.closing, DueDay: tc.due}
			if got := ymd(c.DueDate(tc.month)); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestClosingDateClampsShortMonths(t *testing.T) {
	c := Card{ClosingDay: 31}
	if got := ymd(c.ClosingDate(Month{2026, 2})); got != "2026-02-28" {
		t.Fatalf("febrero: %s", got)
	}
	// Una compra el 28/02 todavía es del resumen que cierra ese mes (cierra el 28).
	if got := c.StatementMonthFor(at(2026, 2, 28, 12, 0)).String(); got != "2026-02" {
		t.Fatalf("compra 28/02: %s", got)
	}
	if got := c.StatementMonthFor(at(2026, 3, 1, 12, 0)).String(); got != "2026-03" {
		t.Fatalf("compra 01/03: %s", got)
	}
}

func TestPlanInstallmentsCrossesYearEnd(t *testing.T) {
	c := Card{ClosingDay: 20, DueDay: 5}
	// Compra el 25/10 (después del cierre) → primera cuota en el resumen de noviembre.
	plan := c.PlanInstallments(at(2026, 10, 25, 12, 0), decimal.RequireFromString("1200.00"), 12)
	if len(plan) != 12 {
		t.Fatalf("cuotas: %d", len(plan))
	}
	wantMonths := []string{"2026-11", "2026-12", "2027-01", "2027-02", "2027-03", "2027-04", "2027-05", "2027-06", "2027-07", "2027-08", "2027-09", "2027-10"}
	sum := decimal.Zero
	for i, in := range plan {
		if in.Number != i+1 || in.Month.String() != wantMonths[i] {
			t.Fatalf("cuota %d: %+v (esperaba %s)", i+1, in, wantMonths[i])
		}
		sum = sum.Add(in.AmountARS)
	}
	if !sum.Equal(decimal.RequireFromString("1200.00")) {
		t.Fatalf("la suma de cuotas es %s", sum)
	}
}

func TestPlanInstallmentsBeforeAndOnClosing(t *testing.T) {
	c := Card{ClosingDay: 20, DueDay: 5}
	amount := decimal.NewFromInt(300)
	before := c.PlanInstallments(at(2026, 10, 10, 9, 0), amount, 3)
	onClose := c.PlanInstallments(at(2026, 10, 20, 18, 0), amount, 3)
	after := c.PlanInstallments(at(2026, 10, 21, 9, 0), amount, 3)
	if before[0].Month.String() != "2026-10" || onClose[0].Month.String() != "2026-10" || after[0].Month.String() != "2026-11" {
		t.Fatalf("antes=%s cierre=%s después=%s", before[0].Month, onClose[0].Month, after[0].Month)
	}
	if before[2].Month.String() != "2026-12" {
		t.Fatalf("tercera cuota: %s", before[2].Month)
	}
}

func TestSplitAmountPutsRemainderInLastInstallment(t *testing.T) {
	// 100.00 / 3 = 33.33 + 33.33 + 33.34
	got := SplitAmount(decimal.RequireFromString("100.00"), 3)
	want := []string{"33.33", "33.33", "33.34"}
	for i := range want {
		if got[i].StringFixed(2) != want[i] {
			t.Fatalf("cuota %d = %s, quería %s", i+1, got[i], want[i])
		}
	}
	one := SplitAmount(decimal.RequireFromString("99.99"), 1)
	if len(one) != 1 || !one[0].Equal(decimal.RequireFromString("99.99")) {
		t.Fatalf("contado: %v", one)
	}
	// 0.05 en 12 cuotas: las primeras 11 en 0.00 y la última 0.05; la suma siempre es exacta.
	tiny := SplitAmount(decimal.RequireFromString("0.05"), 12)
	sum := decimal.Zero
	for _, v := range tiny {
		sum = sum.Add(v)
	}
	if !sum.Equal(decimal.RequireFromString("0.05")) {
		t.Fatalf("suma %s", sum)
	}
}

func TestSummaryTotalIncludesAdjustment(t *testing.T) {
	c := Card{ClosingDay: 20, DueDay: 5}
	s := c.Summary(Month{2026, 10}, decimal.RequireFromString("1000.50"), nil)
	if !s.Total.Equal(decimal.RequireFromString("1000.50")) || s.Paid || ymd(s.DueDate) != "2026-11-05" || ymd(s.ClosingDate) != "2026-10-20" {
		t.Fatalf("sin fila: %+v", s)
	}
	row := &StatementRow{Month: Month{2026, 10}, DueDate: time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC), Adjustment: decimal.RequireFromString("250"), Paid: true}
	s = c.Summary(Month{2026, 10}, decimal.RequireFromString("1000.50"), row)
	if !s.Total.Equal(decimal.RequireFromString("1250.50")) || !s.Paid || ymd(s.DueDate) != "2026-11-06" {
		t.Fatalf("con fila: %+v", s)
	}
}

func TestMonthArithmetic(t *testing.T) {
	if got := (Month{2026, 12}).AddMonths(1).String(); got != "2027-01" {
		t.Fatal(got)
	}
	if got := (Month{2026, 1}).AddMonths(-1).String(); got != "2025-12" {
		t.Fatal(got)
	}
	if _, err := ParseMonth("2026-13"); err == nil {
		t.Fatal("2026-13 debería ser inválido")
	}
}
