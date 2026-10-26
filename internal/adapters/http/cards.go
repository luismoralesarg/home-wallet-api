package http

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type cardDTO struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Last4      string     `json:"last4"`
	ClosingDay int        `json:"closing_day"`
	DueDay     int        `json:"due_day"`
	CreatedBy  userRefDTO `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
}

func toCardDTO(c domain.Card) cardDTO {
	return cardDTO{ID: c.ID, Name: c.Name, Last4: c.Last4, ClosingDay: c.ClosingDay, DueDay: c.DueDay, CreatedBy: toUserRef(c.CreatedBy), CreatedAt: c.CreatedAt}
}

type statementSummaryDTO struct {
	Month             string     `json:"month"`
	ClosingDate       string     `json:"closing_date"`
	DueDate           string     `json:"due_date"`
	InstallmentsTotal string     `json:"installments_total_ars"`
	Adjustment        string     `json:"adjustment_ars"`
	Total             string     `json:"total_ars"`
	Paid              bool       `json:"paid"`
	PaidAt            *time.Time `json:"paid_at"`
}

func toStatementSummaryDTO(s domain.StatementSummary) statementSummaryDTO {
	return statementSummaryDTO{
		Month: s.Month.String(), ClosingDate: dateStr(s.ClosingDate), DueDate: dateStr(s.DueDate),
		InstallmentsTotal: money(s.InstallmentsTotal), Adjustment: money(s.Adjustment), Total: money(s.Total),
		Paid: s.Paid, PaidAt: s.PaidAt,
	}
}

func toStatementSummaries(in []domain.StatementSummary) []statementSummaryDTO {
	out := make([]statementSummaryDTO, len(in))
	for i, s := range in {
		out[i] = toStatementSummaryDTO(s)
	}
	return out
}

type cardOverviewDTO struct {
	cardDTO
	OpenStatement         statementSummaryDTO   `json:"open_statement"`
	ClosedUnpaid          []statementSummaryDTO `json:"closed_unpaid_statements"`
	Upcoming              []statementSummaryDTO `json:"upcoming_statements"`
	FutureInstallmentsARS string                `json:"future_installments_ars"`
}

func toCardOverviewDTO(o application.CardOverview) cardOverviewDTO {
	return cardOverviewDTO{
		cardDTO:               toCardDTO(o.Card),
		OpenStatement:         toStatementSummaryDTO(o.OpenStatement),
		ClosedUnpaid:          toStatementSummaries(o.ClosedUnpaid),
		Upcoming:              toStatementSummaries(o.Upcoming),
		FutureInstallmentsARS: money(o.FutureInstallmentsARS),
	}
}

type cardPurchaseDTO struct {
	ID               string     `json:"id"`
	CardID           string     `json:"card_id"`
	Amount           string     `json:"amount"`
	Currency         string     `json:"currency"`
	FxRate           string     `json:"fx_rate"`
	AmountARS        string     `json:"amount_ars"`
	Installments     int        `json:"installments"`
	CategoryID       string     `json:"category_id"`
	Description      string     `json:"description"`
	PurchasedAt      time.Time  `json:"purchased_at"`
	StatementDueDate string     `json:"statement_due_date"`
	CreatedBy        userRefDTO `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
}

func toCardPurchaseDTO(p domain.CardPurchase) cardPurchaseDTO {
	return cardPurchaseDTO{
		ID: p.ID, CardID: p.CardID, Amount: money(p.Amount), Currency: string(p.Currency), FxRate: money(p.FxRate),
		AmountARS: money(p.AmountARS), Installments: p.Installments, CategoryID: p.CategoryID, Description: p.Description,
		PurchasedAt: p.PurchasedAt, StatementDueDate: dateStr(p.StatementDueDate), CreatedBy: toUserRef(p.CreatedBy), CreatedAt: p.CreatedAt,
	}
}

type statementItemDTO struct {
	PurchaseID       string     `json:"purchase_id"`
	Description      string     `json:"description"`
	PurchasedAt      time.Time  `json:"purchased_at"`
	InstallmentNum   int        `json:"installment_number"`
	Installments     int        `json:"installments"`
	AmountARS        string     `json:"amount_ars"`
	OriginalAmount   string     `json:"original_amount"`
	OriginalCurrency string     `json:"original_currency"`
	CategoryID       string     `json:"category_id"`
	CreatedBy        userRefDTO `json:"created_by"`
}

type statementDetailDTO struct {
	statementSummaryDTO
	CardID string             `json:"card_id"`
	Items  []statementItemDTO `json:"items"`
}

func toStatementDetailDTO(d domain.StatementDetail) statementDetailDTO {
	items := make([]statementItemDTO, len(d.Items))
	for i, it := range d.Items {
		items[i] = statementItemDTO{
			PurchaseID: it.PurchaseID, Description: it.Description, PurchasedAt: it.PurchasedAt, InstallmentNum: it.InstallmentNum,
			Installments: it.Installments, AmountARS: money(it.AmountARS), OriginalAmount: money(it.OriginalAmount),
			OriginalCurrency: string(it.OriginalCurrency), CategoryID: it.CategoryID, CreatedBy: toUserRef(it.CreatedBy),
		}
	}
	return statementDetailDTO{statementSummaryDTO: toStatementSummaryDTO(d.StatementSummary), CardID: d.CardID, Items: items}
}

type cardHandler struct{ svc *application.CardService }

func (h *cardHandler) list(c *fiber.Ctx) error {
	overviews, err := h.svc.List(c.Context())
	if err != nil {
		return err
	}
	out := make([]cardOverviewDTO, len(overviews))
	for i, o := range overviews {
		out[i] = toCardOverviewDTO(o)
	}
	return c.JSON(out)
}

func (h *cardHandler) get(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	o, err := h.svc.Get(c.Context(), id)
	if err != nil {
		return err
	}
	return c.JSON(toCardOverviewDTO(o))
}

func (h *cardHandler) create(c *fiber.Ctx) error {
	var req struct {
		Name       string `json:"name"`
		Last4      string `json:"last4"`
		ClosingDay int    `json:"closing_day"`
		DueDay     int    `json:"due_day"`
	}
	if err := c.BodyParser(&req); err != nil {
		return newError(fiber.StatusBadRequest, "invalid_body", "Body inválido")
	}
	card, err := h.svc.Create(c.Context(), currentUserID(c), application.CardInput{Name: req.Name, Last4: req.Last4, ClosingDay: req.ClosingDay, DueDay: req.DueDay})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(toCardDTO(card))
}

func (h *cardHandler) update(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	var req struct {
		Name       *string `json:"name"`
		Last4      *string `json:"last4"`
		ClosingDay *int    `json:"closing_day"`
		DueDay     *int    `json:"due_day"`
	}
	if err := c.BodyParser(&req); err != nil {
		return newError(fiber.StatusBadRequest, "invalid_body", "Body inválido")
	}
	card, err := h.svc.Update(c.Context(), id, application.CardPatch{Name: req.Name, Last4: req.Last4, ClosingDay: req.ClosingDay, DueDay: req.DueDay})
	if err != nil {
		return err
	}
	return c.JSON(toCardDTO(card))
}

func (h *cardHandler) delete(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Context(), id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *cardHandler) createPurchase(c *fiber.Ctx) error {
	cardID, err := pathID(c)
	if err != nil {
		return err
	}
	var req struct {
		Amount       string     `json:"amount"`
		Currency     string     `json:"currency"`
		Installments int        `json:"installments"`
		CategoryID   string     `json:"category_id"`
		Description  string     `json:"description"`
		PurchasedAt  *time.Time `json:"purchased_at"`
	}
	if err := c.BodyParser(&req); err != nil {
		return newError(fiber.StatusBadRequest, "invalid_body", "Body inválido")
	}
	amount, err := parseMoney("amount", req.Amount)
	if err != nil {
		return err
	}
	p, err := h.svc.CreatePurchase(c.Context(), currentUserID(c), cardID, application.PurchaseInput{
		Amount: amount, Currency: domain.Currency(req.Currency), Installments: req.Installments,
		CategoryID: req.CategoryID, Description: req.Description, PurchasedAt: req.PurchasedAt,
	})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(toCardPurchaseDTO(p))
}

func (h *cardHandler) updatePurchase(c *fiber.Ctx) error {
	cardID, err := pathID(c)
	if err != nil {
		return err
	}
	purchaseID, err := pathUUID(c, "purchaseId")
	if err != nil {
		return err
	}
	var req struct {
		Amount       *string    `json:"amount"`
		Currency     *string    `json:"currency"`
		Installments *int       `json:"installments"`
		CategoryID   *string    `json:"category_id"`
		Description  *string    `json:"description"`
		PurchasedAt  *time.Time `json:"purchased_at"`
	}
	if err := c.BodyParser(&req); err != nil {
		return newError(fiber.StatusBadRequest, "invalid_body", "Body inválido")
	}
	patch := application.PurchasePatch{Installments: req.Installments, CategoryID: req.CategoryID, Description: req.Description, PurchasedAt: req.PurchasedAt}
	if req.Amount != nil {
		var a decimal.Decimal
		if a, err = parseMoney("amount", *req.Amount); err != nil {
			return err
		}
		patch.Amount = &a
	}
	if req.Currency != nil {
		cur := domain.Currency(*req.Currency)
		patch.Currency = &cur
	}
	p, err := h.svc.UpdatePurchase(c.Context(), cardID, purchaseID, patch)
	if err != nil {
		return err
	}
	return c.JSON(toCardPurchaseDTO(p))
}

func (h *cardHandler) deletePurchase(c *fiber.Ctx) error {
	cardID, err := pathID(c)
	if err != nil {
		return err
	}
	purchaseID, err := pathUUID(c, "purchaseId")
	if err != nil {
		return err
	}
	if err := h.svc.DeletePurchase(c.Context(), cardID, purchaseID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *cardHandler) listPurchases(c *fiber.Ctx) error {
	q := application.ListPurchasesQuery{Limit: c.QueryInt("limit", application.DefaultPageSize), Offset: c.QueryInt("offset", 0)}
	if m := c.Query("month"); m != "" {
		t, err := parseMonth(m)
		if err != nil {
			return err
		}
		q.Month = &t
	}
	var err error
	if q.CardID, err = queryUUID(c, "card_id"); err != nil {
		return err
	}
	if q.CategoryID, err = queryUUID(c, "category_id"); err != nil {
		return err
	}
	if q.UserID, err = queryUUID(c, "user_id"); err != nil {
		return err
	}
	page, err := h.svc.ListPurchases(c.Context(), q)
	if err != nil {
		return err
	}
	items := make([]cardPurchaseDTO, len(page.Items))
	for i, p := range page.Items {
		items[i] = toCardPurchaseDTO(p)
	}
	return c.JSON(fiber.Map{"items": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset})
}

func (h *cardHandler) statement(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	m, err := pathMonth(c)
	if err != nil {
		return err
	}
	d, err := h.svc.Statement(c.Context(), id, m)
	if err != nil {
		return err
	}
	return c.JSON(toStatementDetailDTO(d))
}

func (h *cardHandler) setAdjustment(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	m, err := pathMonth(c)
	if err != nil {
		return err
	}
	var req struct {
		AmountARS string `json:"amount_ars"`
	}
	if err := c.BodyParser(&req); err != nil {
		return newError(fiber.StatusBadRequest, "invalid_body", "Body inválido")
	}
	amount, err := parseMoney("amount_ars", req.AmountARS)
	if err != nil {
		return err
	}
	d, err := h.svc.SetAdjustment(c.Context(), id, m, amount)
	if err != nil {
		return err
	}
	return c.JSON(toStatementDetailDTO(d))
}

func (h *cardHandler) pay(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	m, err := pathMonth(c)
	if err != nil {
		return err
	}
	d, err := h.svc.Pay(c.Context(), id, m)
	if err != nil {
		return err
	}
	return c.JSON(toStatementDetailDTO(d))
}
