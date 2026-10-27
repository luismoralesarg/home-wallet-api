package application

import (
	"context"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
	"github.com/luismoralesarg/home-wallet-api/internal/ports"
	"github.com/luismoralesarg/home-wallet-api/internal/tz"
)

// upcomingHorizonDays es hasta dónde mira "próximos pagos" (lo vencido sin pagar siempre figura).
const upcomingHorizonDays = 60

type DashboardService struct {
	repo  ports.DashboardRepository
	cards *CardService
	loans *LoanService
	subs  *SubscriptionService
	now   func() time.Time
}

func NewDashboardService(repo ports.DashboardRepository, cards *CardService, loans *LoanService, subs *SubscriptionService) *DashboardService {
	return &DashboardService{repo: repo, cards: cards, loans: loans, subs: subs, now: time.Now}
}

// WithClock reemplaza el reloj (para tests).
func (s *DashboardService) WithClock(now func() time.Time) *DashboardService {
	s.now = now
	return s
}

// CurrentMonth es el mes de hoy en hora Mendoza.
func (s *DashboardService) CurrentMonth() domain.Month { return domain.MonthOf(s.now()) }

// monthBounds devuelve [inicio, fin) del mes como instantes en hora Mendoza.
func monthBounds(m domain.Month) (time.Time, time.Time) {
	return time.Date(m.Year, m.Mon, 1, 0, 0, 0, 0, tz.Mendoza), time.Date(m.Year, m.Mon+1, 1, 0, 0, 0, 0, tz.Mendoza)
}

// Commitments suma los compromisos de un mes, en ARS.
func (s *DashboardService) Commitments(ctx context.Context, m domain.Month) (domain.Commitments, error) {
	c := domain.Commitments{Cards: decimal.Zero, Loans: decimal.Zero, Vehicle: decimal.Zero}

	due, err := s.repo.StatementsDue(ctx, m.Start(), m.AddMonths(1).Start())
	if err != nil {
		return c, err
	}
	for _, st := range due { // pagados o no: el mes ya los tiene comprometidos
		c.Cards = c.Cards.Add(st.TotalARS)
	}

	loans, err := s.loans.List(ctx)
	if err != nil {
		return c, err
	}
	for _, l := range loans {
		for _, in := range l.Installments {
			if in.Month != m {
				continue
			}
			if l.Loan.Kind == domain.LoanKindVehicle {
				c.Vehicle = c.Vehicle.Add(in.AmountARS)
			} else {
				c.Loans = c.Loans.Add(in.AmountARS)
			}
		}
	}

	subs, err := s.subs.List(ctx)
	if err != nil {
		return c, err
	}
	c.Subscriptions = subs.MonthlyTotalARS
	return c, nil
}

// Daily calcula el indicador de hoy. Gastos variables = transacciones de gasto en efectivo/cuenta.
func (s *DashboardService) Daily(ctx context.Context) (domain.DailyIndicator, error) {
	now := s.now()
	today := tz.DateOf(now)
	month := domain.MonthOfDate(today)
	monthStart, monthEnd := monthBounds(month)
	todayStart := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, tz.Mendoza)
	tomorrowStart := time.Date(today.Year(), today.Month(), today.Day()+1, 0, 0, 0, 0, tz.Mendoza)

	income, err := s.repo.SumTransactions(ctx, domain.Income, monthStart, monthEnd)
	if err != nil {
		return domain.DailyIndicator{}, err
	}
	untilYesterday, err := s.repo.SumTransactions(ctx, domain.Expense, monthStart, todayStart)
	if err != nil {
		return domain.DailyIndicator{}, err
	}
	spentToday, err := s.repo.SumTransactions(ctx, domain.Expense, todayStart, tomorrowStart)
	if err != nil {
		return domain.DailyIndicator{}, err
	}
	commitments, err := s.Commitments(ctx, month)
	if err != nil {
		return domain.DailyIndicator{}, err
	}
	return domain.ComputeDaily(domain.DailyInput{
		Today: today, Income: income, Commitments: commitments,
		SpentUntilYesterday: untilYesterday, SpentToday: spentToday,
	}), nil
}

type CategorySpend struct {
	CategoryID    string
	Name          string
	Total         decimal.Decimal
	PreviousTotal decimal.Decimal
}

type MonthlySummary struct {
	Month                 domain.Month
	SpendByCategory       []CategorySpend
	Income                decimal.Decimal
	Expenses              decimal.Decimal
	PreviousIncome        decimal.Decimal
	PreviousExpenses      decimal.Decimal
	Commitments           domain.Commitments
	CardFutureInstallment decimal.Decimal
}

// Summary arma las métricas de un mes. "Egresos" son los gastos del mes: transacciones de gasto
// más compras con tarjeta por fecha de compra (el mismo universo que el gasto por categoría).
// Los compromisos se informan aparte.
func (s *DashboardService) Summary(ctx context.Context, m domain.Month) (MonthlySummary, error) {
	from, to := monthBounds(m)
	prevFrom, prevTo := monthBounds(m.AddMonths(-1))

	out := MonthlySummary{Month: m}
	var err error
	if out.Income, err = s.repo.SumTransactions(ctx, domain.Income, from, to); err != nil {
		return out, err
	}
	if out.PreviousIncome, err = s.repo.SumTransactions(ctx, domain.Income, prevFrom, prevTo); err != nil {
		return out, err
	}
	cur, err := s.repo.ExpenseByCategory(ctx, from, to)
	if err != nil {
		return out, err
	}
	prev, err := s.repo.ExpenseByCategory(ctx, prevFrom, prevTo)
	if err != nil {
		return out, err
	}

	// Se incluyen las categorías del mes anterior aunque este mes estén en cero, para poder comparar.
	byCat := map[string]*CategorySpend{}
	order := []string{}
	touch := func(id, name string) *CategorySpend {
		cs, ok := byCat[id]
		if !ok {
			cs = &CategorySpend{CategoryID: id, Name: name, Total: decimal.Zero, PreviousTotal: decimal.Zero}
			byCat[id] = cs
			order = append(order, id)
		}
		return cs
	}
	out.Expenses, out.PreviousExpenses = decimal.Zero, decimal.Zero
	for _, c := range cur {
		touch(c.CategoryID, c.Name).Total = c.Total
		out.Expenses = out.Expenses.Add(c.Total)
	}
	for _, c := range prev {
		touch(c.CategoryID, c.Name).PreviousTotal = c.Total
		out.PreviousExpenses = out.PreviousExpenses.Add(c.Total)
	}
	out.SpendByCategory = make([]CategorySpend, 0, len(order))
	for _, id := range order {
		out.SpendByCategory = append(out.SpendByCategory, *byCat[id])
	}
	sort.SliceStable(out.SpendByCategory, func(i, j int) bool {
		a, b := out.SpendByCategory[i], out.SpendByCategory[j]
		if c := b.Total.Cmp(a.Total); c != 0 {
			return c < 0
		}
		if c := b.PreviousTotal.Cmp(a.PreviousTotal); c != 0 {
			return c < 0
		}
		return a.Name < b.Name
	})

	if out.Commitments, err = s.Commitments(ctx, m); err != nil {
		return out, err
	}
	overviews, err := s.cards.List(ctx)
	if err != nil {
		return out, err
	}
	out.CardFutureInstallment = decimal.Zero
	for _, o := range overviews {
		out.CardFutureInstallment = out.CardFutureInstallment.Add(o.FutureInstallmentsARS)
	}
	return out, nil
}

type PaymentType string

const (
	PaymentCardStatement PaymentType = "card_statement"
	PaymentLoan          PaymentType = "loan"
	PaymentVehicle       PaymentType = "vehicle"
	PaymentSubscription  PaymentType = "subscription"
)

type UpcomingPayment struct {
	Type      PaymentType
	Name      string
	Date      time.Time
	AmountARS decimal.Decimal
	RefID     string
	Month     *domain.Month // resumen o cuota, si aplica
}

// Upcoming lista lo que viene por pagar, ordenado por fecha: resúmenes sin pagar (los vencidos primero),
// la próxima cuota sin pagar de cada préstamo y el próximo cobro de cada suscripción activa.
// Las cuotas de préstamos no tienen día de vencimiento: se informa el último día del mes.
func (s *DashboardService) Upcoming(ctx context.Context, limit int) ([]UpcomingPayment, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	now := s.now()
	today := tz.DateOf(now)
	horizon := today.AddDate(0, 0, upcomingHorizonDays)
	out := []UpcomingPayment{}

	statements, err := s.repo.StatementsDue(ctx, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), horizon.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	for _, st := range statements {
		if st.Paid || st.TotalARS.IsZero() {
			continue
		}
		m := st.Month
		out = append(out, UpcomingPayment{Type: PaymentCardStatement, Name: st.CardName, Date: st.DueDate, AmountARS: st.TotalARS, RefID: st.CardID, Month: &m})
	}

	loans, err := s.loans.List(ctx)
	if err != nil {
		return nil, err
	}
	current := domain.MonthOfDate(today)
	for _, l := range loans {
		for _, in := range l.Installments {
			if in.Paid || in.Month.Before(current) {
				continue
			}
			date := in.Month.AddMonths(1).Start().AddDate(0, 0, -1)
			if date.After(horizon) {
				break
			}
			m := in.Month
			typ := PaymentLoan
			if l.Loan.Kind == domain.LoanKindVehicle {
				typ = PaymentVehicle
			}
			out = append(out, UpcomingPayment{Type: typ, Name: l.Loan.Name, Date: date, AmountARS: in.AmountARS, RefID: l.Loan.ID, Month: &m})
			break // solo la próxima cuota sin pagar de cada préstamo
		}
	}

	subs, err := s.subs.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, v := range subs.Items {
		if v.Subscription.Active {
			out = append(out, UpcomingPayment{Type: PaymentSubscription, Name: v.Subscription.Name, Date: v.NextChargeDate, AmountARS: v.AmountARS, RefID: v.Subscription.ID})
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Date.Equal(out[j].Date) {
			return out[i].Date.Before(out[j].Date)
		}
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
