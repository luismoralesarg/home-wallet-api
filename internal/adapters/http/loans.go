package http

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type loanInstallmentDTO struct {
	Month     string `json:"month"`
	Amount    string `json:"amount"`
	AmountARS string `json:"amount_ars"`
	Paid      bool   `json:"paid"`
}

func toLoanInstallmentDTO(i domain.LoanInstallment) loanInstallmentDTO {
	return loanInstallmentDTO{Month: i.Month.String(), Amount: money(i.Amount), AmountARS: money(i.AmountARS), Paid: i.Paid}
}

type loanDTO struct {
	ID                    string              `json:"id"`
	Name                  string              `json:"name"`
	Kind                  string              `json:"kind"`
	Amount                string              `json:"amount"` // cuota base
	Currency              string              `json:"currency"`
	FxRate                string              `json:"fx_rate"`
	AmountARS             string              `json:"amount_ars"`
	StartMonth            string              `json:"start_month"`
	EndMonth              string              `json:"end_month"`
	CurrentInstallment    *loanInstallmentDTO `json:"current_installment"`
	RemainingInstallments int                 `json:"remaining_installments"`
	RemainingTotalARS     string              `json:"remaining_total_ars"`
	CreatedBy             userRefDTO          `json:"created_by"`
	CreatedAt             time.Time           `json:"created_at"`
}

type loanDetailDTO struct {
	loanDTO
	Installments []loanInstallmentDTO `json:"installments"`
}

func toLoanDTO(v application.LoanView) loanDTO {
	l := v.Loan
	dto := loanDTO{
		ID: l.ID, Name: l.Name, Kind: string(l.Kind), Amount: money(l.BaseAmount), Currency: string(l.Currency),
		FxRate: money(v.FxRate), AmountARS: money(v.BaseAmountARS), StartMonth: l.StartMonth.String(), EndMonth: l.EndMonth.String(),
		RemainingInstallments: v.RemainingCount, RemainingTotalARS: money(v.RemainingTotalARS),
		CreatedBy: toUserRef(l.CreatedBy), CreatedAt: l.CreatedAt,
	}
	if v.Current != nil {
		c := toLoanInstallmentDTO(*v.Current)
		dto.CurrentInstallment = &c
	}
	return dto
}

func toLoanDetailDTO(v application.LoanView) loanDetailDTO {
	items := make([]loanInstallmentDTO, len(v.Installments))
	for i, in := range v.Installments {
		items[i] = toLoanInstallmentDTO(in)
	}
	return loanDetailDTO{loanDTO: toLoanDTO(v), Installments: items}
}

type loanHandler struct{ svc *application.LoanService }

func (h *loanHandler) list(c *fiber.Ctx) error {
	views, err := h.svc.List(c.Context())
	if err != nil {
		return err
	}
	out := make([]loanDTO, len(views))
	for i, v := range views {
		out[i] = toLoanDTO(v)
	}
	return c.JSON(out)
}

func (h *loanHandler) get(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	v, err := h.svc.Get(c.Context(), id)
	if err != nil {
		return err
	}
	return c.JSON(toLoanDetailDTO(v))
}

func (h *loanHandler) create(c *fiber.Ctx) error {
	var req struct {
		Name       string `json:"name"`
		Kind       string `json:"kind"`
		BaseAmount string `json:"base_amount"`
		Currency   string `json:"currency"`
		StartMonth string `json:"start_month"`
		EndMonth   string `json:"end_month"`
	}
	if err := c.BodyParser(&req); err != nil {
		return newError(fiber.StatusBadRequest, "invalid_body", "Body inválido")
	}
	base, err := parseMoney("base_amount", req.BaseAmount)
	if err != nil {
		return err
	}
	start, err := domain.ParseMonth(req.StartMonth)
	if err != nil {
		return newError(fiber.StatusBadRequest, "validation_error", "start_month debe tener formato YYYY-MM")
	}
	end, err := domain.ParseMonth(req.EndMonth)
	if err != nil {
		return newError(fiber.StatusBadRequest, "validation_error", "end_month debe tener formato YYYY-MM")
	}
	v, err := h.svc.Create(c.Context(), currentUserID(c), application.LoanInput{
		Name: req.Name, Kind: domain.LoanKind(req.Kind), BaseAmount: base, Currency: domain.Currency(req.Currency),
		StartMonth: start, EndMonth: end,
	})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(toLoanDetailDTO(v))
}

func (h *loanHandler) update(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	var req struct {
		Name *string `json:"name"`
	}
	if err := c.BodyParser(&req); err != nil {
		return newError(fiber.StatusBadRequest, "invalid_body", "Body inválido")
	}
	v, err := h.svc.Rename(c.Context(), id, req.Name)
	if err != nil {
		return err
	}
	return c.JSON(toLoanDTO(v))
}

func (h *loanHandler) delete(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Context(), id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *loanHandler) updateInstallment(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	month, err := pathMonth(c)
	if err != nil {
		return err
	}
	var req struct {
		Amount *string `json:"amount"`
		Paid   *bool   `json:"paid"`
	}
	if err := c.BodyParser(&req); err != nil {
		return newError(fiber.StatusBadRequest, "invalid_body", "Body inválido")
	}
	patch := application.InstallmentPatch{Paid: req.Paid}
	if req.Amount != nil {
		a, err := parseMoney("amount", *req.Amount)
		if err != nil {
			return err
		}
		patch.Amount = &a
	}
	in, err := h.svc.UpdateInstallment(c.Context(), id, month, patch)
	if err != nil {
		return err
	}
	return c.JSON(toLoanInstallmentDTO(in))
}

func (h *loanHandler) adjust(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	var req struct {
		FromMonth string  `json:"from_month"`
		Percent   *string `json:"percent"`
		Amount    *string `json:"amount"`
	}
	if err := c.BodyParser(&req); err != nil {
		return newError(fiber.StatusBadRequest, "invalid_body", "Body inválido")
	}
	from, err := domain.ParseMonth(req.FromMonth)
	if err != nil {
		return newError(fiber.StatusBadRequest, "validation_error", "from_month debe tener formato YYYY-MM")
	}
	in := application.AdjustInput{FromMonth: from}
	for field, pair := range map[string]struct {
		src *string
		dst **decimal.Decimal
	}{"percent": {req.Percent, &in.Percent}, "amount": {req.Amount, &in.Amount}} {
		if pair.src == nil {
			continue
		}
		d, err := parseMoney(field, *pair.src)
		if err != nil {
			return err
		}
		*pair.dst = &d
	}
	v, err := h.svc.Adjust(c.Context(), id, in)
	if err != nil {
		return err
	}
	return c.JSON(toLoanDetailDTO(v))
}
