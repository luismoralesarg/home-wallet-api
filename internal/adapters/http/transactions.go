package http

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type transactionDTO struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Amount      string     `json:"amount"`
	Currency    string     `json:"currency"`
	FxRate      string     `json:"fx_rate"`
	AmountARS   string     `json:"amount_ars"`
	CategoryID  string     `json:"category_id"`
	Description string     `json:"description"`
	OccurredAt  time.Time  `json:"occurred_at"`
	CreatedBy   userRefDTO `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
}

func toTransactionDTO(t domain.Transaction) transactionDTO {
	return transactionDTO{
		ID: t.ID, Kind: string(t.Kind), Amount: money(t.Amount), Currency: string(t.Currency),
		FxRate: money(t.FxRate), AmountARS: money(t.AmountARS), CategoryID: t.CategoryID,
		Description: t.Description, OccurredAt: t.OccurredAt, CreatedBy: toUserRef(t.CreatedBy), CreatedAt: t.CreatedAt,
	}
}

type transactionHandler struct {
	svc *application.TransactionService
}

func (h *transactionHandler) list(c *fiber.Ctx) error {
	q := application.ListTransactionsQuery{Limit: c.QueryInt("limit", application.DefaultPageSize), Offset: c.QueryInt("offset", 0)}
	if m := c.Query("month"); m != "" {
		t, err := parseMonth(m)
		if err != nil {
			return err
		}
		q.Month = &t
	}
	if k := c.Query("kind"); k != "" {
		kk := domain.Kind(k)
		q.Kind = &kk
	}
	var err error
	if q.CategoryID, err = queryUUID(c, "category_id"); err != nil {
		return err
	}
	if q.UserID, err = queryUUID(c, "user_id"); err != nil {
		return err
	}
	page, err := h.svc.List(c.Context(), q)
	if err != nil {
		return err
	}
	items := make([]transactionDTO, len(page.Items))
	for i, t := range page.Items {
		items[i] = toTransactionDTO(t)
	}
	return c.JSON(fiber.Map{"items": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset})
}

func (h *transactionHandler) get(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	t, err := h.svc.Get(c.Context(), id)
	if err != nil {
		return err
	}
	return c.JSON(toTransactionDTO(t))
}

func (h *transactionHandler) create(c *fiber.Ctx) error {
	var req struct {
		Kind        string     `json:"kind"`
		Amount      string     `json:"amount"`
		Currency    string     `json:"currency"`
		CategoryID  string     `json:"category_id"`
		Description string     `json:"description"`
		OccurredAt  *time.Time `json:"occurred_at"`
	}
	if err := c.BodyParser(&req); err != nil {
		return newError(fiber.StatusBadRequest, "invalid_body", "Body inválido")
	}
	amount, err := parseMoney("amount", req.Amount)
	if err != nil {
		return err
	}
	t, err := h.svc.Create(c.Context(), currentUserID(c), application.TransactionInput{
		Kind: domain.Kind(req.Kind), Amount: amount, Currency: domain.Currency(req.Currency),
		CategoryID: req.CategoryID, Description: req.Description, OccurredAt: req.OccurredAt,
	})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(toTransactionDTO(t))
}

func (h *transactionHandler) update(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	var req struct {
		Amount      *string    `json:"amount"`
		Currency    *string    `json:"currency"`
		CategoryID  *string    `json:"category_id"`
		Description *string    `json:"description"`
		OccurredAt  *time.Time `json:"occurred_at"`
	}
	if err := c.BodyParser(&req); err != nil {
		return newError(fiber.StatusBadRequest, "invalid_body", "Body inválido")
	}
	p := application.TransactionPatch{CategoryID: req.CategoryID, Description: req.Description, OccurredAt: req.OccurredAt}
	if req.Amount != nil {
		var a decimal.Decimal
		if a, err = parseMoney("amount", *req.Amount); err != nil {
			return err
		}
		p.Amount = &a
	}
	if req.Currency != nil {
		cur := domain.Currency(*req.Currency)
		p.Currency = &cur
	}
	t, err := h.svc.Update(c.Context(), id, p)
	if err != nil {
		return err
	}
	return c.JSON(toTransactionDTO(t))
}

func (h *transactionHandler) delete(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Context(), id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
