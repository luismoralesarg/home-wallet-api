package application

import (
	"context"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
	"github.com/luismoralesarg/home-wallet-api/internal/ports"
)

type LoanService struct {
	repo ports.LoanRepository
	fx   Converter
	now  func() time.Time
}

func NewLoanService(repo ports.LoanRepository, fx Converter) *LoanService {
	return &LoanService{repo: repo, fx: fx, now: time.Now}
}

// WithClock reemplaza el reloj (para tests).
func (s *LoanService) WithClock(now func() time.Time) *LoanService {
	s.now = now
	return s
}

// LoanView es un préstamo con sus cuotas valuadas en ARS.
//
// Las cuotas de un préstamo en USD se valúan a la cotización de hoy mientras están sin pagar
// (es lo que se va a terminar pagando) y quedan fijas a la cotización del día en que se marcan pagadas.
type LoanView struct {
	Loan              domain.Loan
	FxRate            decimal.Decimal // cotización de hoy usada (1 en ARS)
	BaseAmountARS     decimal.Decimal
	Current           *domain.LoanInstallment // cuota del mes actual
	RemainingCount    int                     // cuotas sin pagar desde el mes actual
	RemainingTotalARS decimal.Decimal
	Installments      []domain.LoanInstallment
}

type LoanInput struct {
	Name       string
	Kind       domain.LoanKind
	BaseAmount decimal.Decimal
	Currency   domain.Currency
	StartMonth domain.Month
	EndMonth   domain.Month
}

func (s *LoanService) Create(ctx context.Context, userID string, in LoanInput) (LoanView, error) {
	name := strings.TrimSpace(in.Name)
	switch {
	case name == "":
		return LoanView{}, invalid("el nombre es obligatorio")
	case !in.Kind.Valid():
		return LoanView{}, invalid("kind debe ser loan o vehicle")
	case !in.Currency.Valid():
		return LoanView{}, invalid("currency debe ser ARS o USD")
	case !in.BaseAmount.IsPositive():
		return LoanView{}, invalid("la cuota base debe ser mayor a cero")
	case !in.BaseAmount.Equal(in.BaseAmount.Round(2)):
		return LoanView{}, invalid("la cuota base admite hasta 2 decimales")
	case in.EndMonth.Before(in.StartMonth):
		return LoanView{}, invalid("end_month no puede ser anterior a start_month")
	}
	months := domain.MonthsBetween(in.StartMonth, in.EndMonth)
	if len(months) > domain.MaxLoanInstallments {
		return LoanView{}, invalid("el préstamo no puede tener más de %d cuotas", domain.MaxLoanInstallments)
	}
	_, baseARS, err := s.fx.ConvertToARS(ctx, in.BaseAmount, in.Currency, s.now())
	if err != nil {
		return LoanView{}, err
	}
	if baseARS.GreaterThanOrEqual(maxARS) {
		return LoanView{}, invalid("el monto es demasiado grande")
	}
	installments := make([]domain.LoanInstallment, len(months))
	for i, m := range months {
		installments[i] = domain.LoanInstallment{Month: m, Amount: in.BaseAmount, AmountARS: baseARS}
	}
	l, err := s.repo.Create(ctx, domain.Loan{
		Name: name, Kind: in.Kind, Currency: in.Currency, BaseAmount: in.BaseAmount,
		StartMonth: in.StartMonth, EndMonth: in.EndMonth, CreatedBy: domain.UserRef{ID: userID},
	}, installments)
	if err != nil {
		return LoanView{}, err
	}
	return s.view(ctx, l)
}

func (s *LoanService) List(ctx context.Context) ([]LoanView, error) {
	loans, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]LoanView, 0, len(loans))
	for _, l := range loans {
		v, err := s.view(ctx, l)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *LoanService) Get(ctx context.Context, id string) (LoanView, error) {
	l, err := s.repo.Get(ctx, id)
	if err != nil {
		return LoanView{}, err
	}
	return s.view(ctx, l)
}

func (s *LoanService) Rename(ctx context.Context, id string, name *string) (LoanView, error) {
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return LoanView{}, invalid("el nombre es obligatorio")
		}
		if _, err := s.repo.Rename(ctx, id, n); err != nil {
			return LoanView{}, err
		}
	}
	return s.Get(ctx, id)
}

func (s *LoanService) Delete(ctx context.Context, id string) error { return s.repo.Delete(ctx, id) }

type InstallmentPatch struct {
	Amount *decimal.Decimal
	Paid   *bool
}

// UpdateInstallment ajusta el monto de una cuota puntual y/o la marca como pagada o no pagada.
func (s *LoanService) UpdateInstallment(ctx context.Context, id string, month domain.Month, p InstallmentPatch) (domain.LoanInstallment, error) {
	l, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.LoanInstallment{}, err
	}
	all, err := s.repo.Installments(ctx, id)
	if err != nil {
		return domain.LoanInstallment{}, err
	}
	var inst *domain.LoanInstallment
	for i := range all {
		if all[i].Month == month {
			inst = &all[i]
		}
	}
	if inst == nil {
		return domain.LoanInstallment{}, ErrNotFound
	}
	if p.Amount != nil {
		if !p.Amount.IsPositive() || !p.Amount.Equal(p.Amount.Round(2)) {
			return domain.LoanInstallment{}, invalid("el monto debe ser mayor a cero y tener hasta 2 decimales")
		}
		inst.Amount = *p.Amount
	}
	wasPaid := inst.Paid
	if p.Paid != nil {
		inst.Paid = *p.Paid
	}
	// El valor en ARS se fija a la cotización de hoy cuando cambia el monto o la cuota se marca pagada.
	if p.Amount != nil || (inst.Paid && !wasPaid) {
		_, ars, err := s.fx.ConvertToARS(ctx, inst.Amount, l.Currency, s.now())
		if err != nil {
			return domain.LoanInstallment{}, err
		}
		if ars.GreaterThanOrEqual(maxARS) {
			return domain.LoanInstallment{}, invalid("el monto es demasiado grande")
		}
		inst.AmountARS = ars
	}
	if err := s.repo.SaveInstallments(ctx, id, []domain.LoanInstallment{*inst}); err != nil {
		return domain.LoanInstallment{}, err
	}
	return *inst, nil
}

type AdjustInput struct {
	FromMonth domain.Month
	Percent   *decimal.Decimal
	Amount    *decimal.Decimal
}

// Adjust aplica un aumento (porcentaje o monto fijo) a las cuotas sin pagar desde un mes.
func (s *LoanService) Adjust(ctx context.Context, id string, in AdjustInput) (LoanView, error) {
	if (in.Percent == nil) == (in.Amount == nil) {
		return LoanView{}, invalid("indicá percent o amount, uno solo")
	}
	if in.Percent != nil && (in.Percent.IsNegative() || !in.Percent.Equal(in.Percent.Round(2))) {
		return LoanView{}, invalid("percent debe ser mayor o igual a cero, con hasta 2 decimales")
	}
	if in.Amount != nil && !in.Amount.Equal(in.Amount.Round(2)) {
		return LoanView{}, invalid("amount admite hasta 2 decimales")
	}
	l, err := s.repo.Get(ctx, id)
	if err != nil {
		return LoanView{}, err
	}
	all, err := s.repo.Installments(ctx, id)
	if err != nil {
		return LoanView{}, err
	}
	var changed []domain.LoanInstallment
	for _, inst := range all {
		if inst.Paid || inst.Month.Before(in.FromMonth) {
			continue
		}
		inst.Amount = domain.Adjust(inst.Amount, in.Percent, in.Amount)
		if !inst.Amount.IsPositive() {
			return LoanView{}, invalid("el ajuste dejaría la cuota de %s en cero o negativa", inst.Month)
		}
		_, ars, err := s.fx.ConvertToARS(ctx, inst.Amount, l.Currency, s.now())
		if err != nil {
			return LoanView{}, err
		}
		if ars.GreaterThanOrEqual(maxARS) {
			return LoanView{}, invalid("el monto es demasiado grande")
		}
		inst.AmountARS = ars
		changed = append(changed, inst)
	}
	if len(changed) == 0 {
		return LoanView{}, invalid("no hay cuotas sin pagar desde %s", in.FromMonth)
	}
	if err := s.repo.SaveInstallments(ctx, id, changed); err != nil {
		return LoanView{}, err
	}
	return s.view(ctx, l)
}

// view valúa las cuotas y calcula cuota del mes, restantes y total restante.
func (s *LoanService) view(ctx context.Context, l domain.Loan) (LoanView, error) {
	all, err := s.repo.Installments(ctx, l.ID)
	if err != nil {
		return LoanView{}, err
	}
	fx, baseARS, err := s.fx.ConvertToARS(ctx, l.BaseAmount, l.Currency, s.now())
	if err != nil {
		return LoanView{}, err
	}
	v := LoanView{Loan: l, FxRate: fx, BaseAmountARS: baseARS, RemainingTotalARS: decimal.Zero}
	current := domain.MonthOf(s.now())
	for i := range all {
		inst := &all[i]
		if l.Currency == domain.USD && !inst.Paid {
			inst.AmountARS = inst.Amount.Mul(fx).Round(2)
		}
		if inst.Month == current {
			c := *inst
			v.Current = &c
		}
		if !inst.Paid && !inst.Month.Before(current) {
			v.RemainingCount++
			v.RemainingTotalARS = v.RemainingTotalARS.Add(inst.AmountARS)
		}
	}
	v.Installments = all
	return v, nil
}
