package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

// Commitments son los pagos fijos de un mes, en ARS.
type Commitments struct {
	Cards         decimal.Decimal // resúmenes de tarjeta con VENCIMIENTO en el mes
	Loans         decimal.Decimal
	Vehicle       decimal.Decimal
	Subscriptions decimal.Decimal
}

func (c Commitments) Total() decimal.Decimal {
	return c.Cards.Add(c.Loans).Add(c.Vehicle).Add(c.Subscriptions)
}

// DailyInput son los datos del mes para calcular cuánto se puede gastar hoy.
// Los gastos son solo los variables (efectivo/cuenta): las compras con tarjeta
// no entran acá, impactan cuando vence su resumen (Commitments.Cards).
type DailyInput struct {
	Today               time.Time // día en Mendoza (00:00 UTC)
	Income              decimal.Decimal
	Commitments         Commitments
	SpentUntilYesterday decimal.Decimal
	SpentToday          decimal.Decimal
}

type DailyIndicator struct {
	Date                time.Time
	Limit               decimal.Decimal // límite de hoy; negativo si el mes ya se pasó del presupuesto
	Spent               decimal.Decimal
	Remaining           decimal.Decimal // Limit - Spent; negativo si se pasó
	OverLimit           bool
	OverAmount          decimal.Decimal // cuánto se pasó (0 si no)
	Income              decimal.Decimal
	Commitments         Commitments
	VariableBudget      decimal.Decimal
	SpentUntilYesterday decimal.Decimal
	RemainingDays       int
}

// RemainingDays cuenta los días que quedan del mes, hoy incluido.
func RemainingDays(today time.Time) int {
	m := MonthOfDate(today)
	last := m.AddMonths(1).Start().AddDate(0, 0, -1).Day()
	return last - today.Day() + 1
}

// ComputeDaily aplica la regla del indicador:
//
//	presupuesto_variable = ingresos_del_mes − compromisos_del_mes
//	limite_hoy = (presupuesto_variable − gastado_hasta_ayer) / dias_restantes
//
// El límite se fija con lo gastado hasta ayer, así no se mueve mientras se cargan gastos del día.
// Si hoy se supera, el límite de mañana baja solo; si se gasta menos, sube.
func ComputeDaily(in DailyInput) DailyIndicator {
	days := RemainingDays(in.Today)
	budget := in.Income.Sub(in.Commitments.Total())
	limit := budget.Sub(in.SpentUntilYesterday).DivRound(decimal.NewFromInt(int64(days)), 2)
	remaining := limit.Sub(in.SpentToday)
	out := DailyIndicator{
		Date: in.Today, Limit: limit, Spent: in.SpentToday, Remaining: remaining,
		Income: in.Income, Commitments: in.Commitments, VariableBudget: budget,
		SpentUntilYesterday: in.SpentUntilYesterday, RemainingDays: days,
		OverAmount: decimal.Zero,
	}
	if remaining.IsNegative() {
		out.OverLimit = true
		out.OverAmount = remaining.Neg()
	}
	return out
}
