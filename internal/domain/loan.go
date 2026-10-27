package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

type LoanKind string

const (
	LoanKindLoan    LoanKind = "loan"
	LoanKindVehicle LoanKind = "vehicle"
)

func (k LoanKind) Valid() bool { return k == LoanKindLoan || k == LoanKindVehicle }

// MaxLoanInstallments limita el largo de un préstamo (30 años).
const MaxLoanInstallments = 360

type Loan struct {
	ID         string
	Name       string
	Kind       LoanKind
	Currency   Currency
	BaseAmount decimal.Decimal
	StartMonth Month
	EndMonth   Month
	CreatedBy  UserRef
	CreatedAt  time.Time
}

// LoanInstallment es la cuota de un mes. Amount está en la moneda del préstamo.
// AmountARS es el valor guardado: para cuotas de un préstamo en USD solo es definitivo una vez pagada.
type LoanInstallment struct {
	Month     Month
	Amount    decimal.Decimal
	AmountARS decimal.Decimal
	Paid      bool
}

// MonthsBetween devuelve los meses de start a end, ambos inclusive.
func MonthsBetween(start, end Month) []Month {
	var out []Month
	for m := start; !m.After(end); m = m.AddMonths(1) {
		out = append(out, m)
	}
	return out
}

// Adjust devuelve el monto con un aumento por porcentaje (percent != nil) o por monto fijo.
// Redondea a centavos.
func Adjust(amount decimal.Decimal, percent, delta *decimal.Decimal) decimal.Decimal {
	if percent != nil {
		factor := decimal.NewFromInt(1).Add(percent.Div(decimal.NewFromInt(100)))
		return amount.Mul(factor).Round(2)
	}
	return amount.Add(*delta).Round(2)
}
