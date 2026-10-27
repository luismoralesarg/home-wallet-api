package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type commitmentsDTO struct {
	CardsARS         string `json:"cards_ars"`
	LoansARS         string `json:"loans_ars"`
	VehicleARS       string `json:"vehicle_ars"`
	SubscriptionsARS string `json:"subscriptions_ars"`
	TotalARS         string `json:"total_ars"`
}

func toCommitmentsDTO(c domain.Commitments) commitmentsDTO {
	return commitmentsDTO{
		CardsARS: money(c.Cards), LoansARS: money(c.Loans), VehicleARS: money(c.Vehicle),
		SubscriptionsARS: money(c.Subscriptions), TotalARS: money(c.Total()),
	}
}

type dailyBreakdownDTO struct {
	IncomeMonthARS                 string         `json:"income_month_ars"`
	Commitments                    commitmentsDTO `json:"commitments"`
	VariableBudgetARS              string         `json:"variable_budget_ars"`
	VariableSpentUntilYesterdayARS string         `json:"variable_spent_until_yesterday_ars"`
	RemainingDays                  int            `json:"remaining_days"`
}

type dailyIndicatorDTO struct {
	Date        string            `json:"date"`
	LimiteHoy   string            `json:"limite_hoy"`
	GastadoHoy  string            `json:"gastado_hoy"`
	RestanteHoy string            `json:"restante_hoy"`
	OverLimit   bool              `json:"over_limit"`
	OverAmount  string            `json:"over_amount"`
	Breakdown   dailyBreakdownDTO `json:"breakdown"`
}

type categorySpendDTO struct {
	CategoryID            string `json:"category_id"`
	Name                  string `json:"name"`
	TotalARS              string `json:"total_ars"`
	PreviousMonthTotalARS string `json:"previous_month_total_ars"`
}

type monthlySummaryDTO struct {
	Month                    string             `json:"month"`
	SpendByCategory          []categorySpendDTO `json:"spend_by_category"`
	IncomeARS                string             `json:"income_ars"`
	ExpensesARS              string             `json:"expenses_ars"`
	PreviousMonthIncomeARS   string             `json:"previous_month_income_ars"`
	PreviousMonthExpensesARS string             `json:"previous_month_expenses_ars"`
	Commitments              commitmentsDTO     `json:"commitments"`
	CardFutureInstallments   string             `json:"card_future_installments_ars"`
}

type upcomingPaymentDTO struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Date      string `json:"date"`
	AmountARS string `json:"amount_ars"`
	RefID     string `json:"ref_id,omitempty"`
	Month     string `json:"month,omitempty"`
}

type dashboardHandler struct{ svc *application.DashboardService }

func (h *dashboardHandler) daily(c *fiber.Ctx) error {
	d, err := h.svc.Daily(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(dailyIndicatorDTO{
		Date: dateStr(d.Date), LimiteHoy: money(d.Limit), GastadoHoy: money(d.Spent), RestanteHoy: money(d.Remaining),
		OverLimit: d.OverLimit, OverAmount: money(d.OverAmount),
		Breakdown: dailyBreakdownDTO{
			IncomeMonthARS: money(d.Income), Commitments: toCommitmentsDTO(d.Commitments), VariableBudgetARS: money(d.VariableBudget),
			VariableSpentUntilYesterdayARS: money(d.SpentUntilYesterday), RemainingDays: d.RemainingDays,
		},
	})
}

func (h *dashboardHandler) summary(c *fiber.Ctx) error {
	m := h.svc.CurrentMonth()
	if v := c.Query("month"); v != "" {
		parsed, err := domain.ParseMonth(v)
		if err != nil {
			return newError(fiber.StatusBadRequest, "validation_error", "month debe tener formato YYYY-MM")
		}
		m = parsed
	}
	s, err := h.svc.Summary(c.Context(), m)
	if err != nil {
		return err
	}
	cats := make([]categorySpendDTO, len(s.SpendByCategory))
	for i, cs := range s.SpendByCategory {
		cats[i] = categorySpendDTO{CategoryID: cs.CategoryID, Name: cs.Name, TotalARS: money(cs.Total), PreviousMonthTotalARS: money(cs.PreviousTotal)}
	}
	return c.JSON(monthlySummaryDTO{
		Month: s.Month.String(), SpendByCategory: cats, IncomeARS: money(s.Income), ExpensesARS: money(s.Expenses),
		PreviousMonthIncomeARS: money(s.PreviousIncome), PreviousMonthExpensesARS: money(s.PreviousExpenses),
		Commitments: toCommitmentsDTO(s.Commitments), CardFutureInstallments: money(s.CardFutureInstallment),
	})
}

func (h *dashboardHandler) upcoming(c *fiber.Ctx) error {
	items, err := h.svc.Upcoming(c.Context(), c.QueryInt("limit", 20))
	if err != nil {
		return err
	}
	out := make([]upcomingPaymentDTO, len(items))
	for i, p := range items {
		dto := upcomingPaymentDTO{Type: string(p.Type), Name: p.Name, Date: dateStr(p.Date), AmountARS: money(p.AmountARS), RefID: p.RefID}
		if p.Month != nil {
			dto.Month = p.Month.String()
		}
		out[i] = dto
	}
	return c.JSON(out)
}
