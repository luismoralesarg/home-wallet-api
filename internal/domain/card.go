package domain

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/tz"
)

const MaxInstallments = 60

type Card struct {
	ID         string
	Name       string
	Last4      string
	ClosingDay int
	DueDay     int
	CreatedBy  UserRef
	CreatedAt  time.Time
}

// ClosingDate es la fecha de cierre del resumen del mes `m`.
func (c Card) ClosingDate(m Month) time.Time { return m.Day(c.ClosingDay) }

// DueDate es el vencimiento del resumen que cierra en `m`: el primer due_day posterior al cierre.
func (c Card) DueDate(m Month) time.Time {
	closing := c.ClosingDate(m)
	due := m.Day(c.DueDay)
	if !due.After(closing) {
		due = m.AddMonths(1).Day(c.DueDay)
	}
	return due
}

// StatementMonthFor devuelve el resumen (por su mes de cierre) al que va una compra hecha en `at`:
// hasta el día de cierre inclusive cierra ese mes; después, el siguiente.
func (c Card) StatementMonthFor(at time.Time) Month {
	d := tz.DateOf(at)
	m := MonthOfDate(d)
	if d.After(c.ClosingDate(m)) {
		return m.AddMonths(1)
	}
	return m
}

// OpenMonth es el resumen que hoy todavía está acumulando compras.
func (c Card) OpenMonth(now time.Time) Month { return c.StatementMonthFor(now) }

// Installment es una cuota asignada a un resumen.
type Installment struct {
	Number    int
	Month     Month
	AmountARS decimal.Decimal
}

// PlanInstallments reparte amountARS en n cuotas y asigna cada una a su resumen:
// la primera al resumen de la compra, las siguientes un mes más cada una.
func (c Card) PlanInstallments(purchasedAt time.Time, amountARS decimal.Decimal, n int) []Installment {
	first := c.StatementMonthFor(purchasedAt)
	amounts := SplitAmount(amountARS, n)
	plan := make([]Installment, n)
	for i := range plan {
		plan[i] = Installment{Number: i + 1, Month: first.AddMonths(i), AmountARS: amounts[i]}
	}
	return plan
}

// SplitAmount divide total en n partes de centavos exactos; el sobrante va a la última cuota.
func SplitAmount(total decimal.Decimal, n int) []decimal.Decimal {
	base := total.Div(decimal.NewFromInt(int64(n))).Truncate(2)
	out := make([]decimal.Decimal, n)
	sum := decimal.Zero
	for i := 0; i < n-1; i++ {
		out[i] = base
		sum = sum.Add(base)
	}
	out[n-1] = total.Sub(sum)
	return out
}

type CardPurchase struct {
	ID           string
	CardID       string
	Amount       decimal.Decimal
	Currency     Currency
	FxRate       decimal.Decimal
	AmountARS    decimal.Decimal
	Installments int
	CategoryID   string
	Description  string
	PurchasedAt  time.Time
	CreatedBy    UserRef
	CreatedAt    time.Time

	// Derivados (los completa el repositorio): resumen y vencimiento de la primera cuota.
	FirstStatement   Month
	StatementDueDate time.Time
}

// StatementRow es lo que se guarda de un resumen; el total se calcula.
type StatementRow struct {
	CardID     string
	Month      Month
	DueDate    time.Time
	Adjustment decimal.Decimal
	Paid       bool
	PaidAt     *time.Time
}

type StatementSummary struct {
	Month             Month
	ClosingDate       time.Time
	DueDate           time.Time
	InstallmentsTotal decimal.Decimal
	Adjustment        decimal.Decimal
	Total             decimal.Decimal
	Paid              bool
	PaidAt            *time.Time
}

// Summary arma el resumen de un mes. `row` es nil si todavía no se guardó nada para ese mes.
func (c Card) Summary(m Month, installmentsTotal decimal.Decimal, row *StatementRow) StatementSummary {
	s := StatementSummary{
		Month:             m,
		ClosingDate:       c.ClosingDate(m),
		DueDate:           c.DueDate(m),
		InstallmentsTotal: installmentsTotal,
		Adjustment:        decimal.Zero,
	}
	if row != nil {
		s.DueDate, s.Adjustment, s.Paid, s.PaidAt = row.DueDate, row.Adjustment, row.Paid, row.PaidAt
	}
	s.Total = s.InstallmentsTotal.Add(s.Adjustment)
	return s
}

type StatementItem struct {
	PurchaseID       string
	Description      string
	PurchasedAt      time.Time
	InstallmentNum   int
	Installments     int
	AmountARS        decimal.Decimal
	OriginalAmount   decimal.Decimal
	OriginalCurrency Currency
	CategoryID       string
	CreatedBy        UserRef
}

type StatementDetail struct {
	CardID string
	StatementSummary
	Items []StatementItem
}
