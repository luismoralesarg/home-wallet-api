package http

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type subscriptionDTO struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Amount         string     `json:"amount"`
	Currency       string     `json:"currency"`
	FxRate         string     `json:"fx_rate"`
	AmountARS      string     `json:"amount_ars"`
	ChargeDay      int        `json:"charge_day"`
	Active         bool       `json:"active"`
	NextChargeDate string     `json:"next_charge_date"`
	CreatedBy      userRefDTO `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
}

func toSubscriptionDTO(v application.SubscriptionView) subscriptionDTO {
	s := v.Subscription
	return subscriptionDTO{
		ID: s.ID, Name: s.Name, Amount: money(s.Amount), Currency: string(s.Currency), FxRate: money(v.FxRate),
		AmountARS: money(v.AmountARS), ChargeDay: s.ChargeDay, Active: s.Active, NextChargeDate: dateStr(v.NextChargeDate),
		CreatedBy: toUserRef(s.CreatedBy), CreatedAt: s.CreatedAt,
	}
}

type subscriptionHandler struct {
	svc *application.SubscriptionService
}

func (h *subscriptionHandler) list(c *fiber.Ctx) error {
	l, err := h.svc.List(c.Context())
	if err != nil {
		return err
	}
	items := make([]subscriptionDTO, len(l.Items))
	for i, v := range l.Items {
		items[i] = toSubscriptionDTO(v)
	}
	return c.JSON(fiber.Map{"items": items, "monthly_total_ars": money(l.MonthlyTotalARS)})
}

func (h *subscriptionHandler) get(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	v, err := h.svc.Get(c.Context(), id)
	if err != nil {
		return err
	}
	return c.JSON(toSubscriptionDTO(v))
}

func (h *subscriptionHandler) create(c *fiber.Ctx) error {
	var req struct {
		Name      string `json:"name"`
		Amount    string `json:"amount"`
		Currency  string `json:"currency"`
		ChargeDay int    `json:"charge_day"`
	}
	if err := c.BodyParser(&req); err != nil {
		return newError(fiber.StatusBadRequest, "invalid_body", "Body inválido")
	}
	amount, err := parseMoney("amount", req.Amount)
	if err != nil {
		return err
	}
	v, err := h.svc.Create(c.Context(), currentUserID(c), application.SubscriptionInput{
		Name: req.Name, Amount: amount, Currency: domain.Currency(req.Currency), ChargeDay: req.ChargeDay,
	})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(toSubscriptionDTO(v))
}

func (h *subscriptionHandler) update(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	var req struct {
		Name      *string `json:"name"`
		Amount    *string `json:"amount"`
		Currency  *string `json:"currency"`
		ChargeDay *int    `json:"charge_day"`
	}
	if err := c.BodyParser(&req); err != nil {
		return newError(fiber.StatusBadRequest, "invalid_body", "Body inválido")
	}
	p := application.SubscriptionPatch{Name: req.Name, ChargeDay: req.ChargeDay}
	if req.Amount != nil {
		a, err := parseMoney("amount", *req.Amount)
		if err != nil {
			return err
		}
		p.Amount = &a
	}
	if req.Currency != nil {
		cur := domain.Currency(*req.Currency)
		p.Currency = &cur
	}
	v, err := h.svc.Update(c.Context(), id, p)
	if err != nil {
		return err
	}
	return c.JSON(toSubscriptionDTO(v))
}

func (h *subscriptionHandler) setActive(active bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := pathID(c)
		if err != nil {
			return err
		}
		v, err := h.svc.SetActive(c.Context(), id, active)
		if err != nil {
			return err
		}
		return c.JSON(toSubscriptionDTO(v))
	}
}

func (h *subscriptionHandler) delete(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Context(), id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
