package application

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
	"github.com/luismoralesarg/home-wallet-api/internal/ports"
)

var last4Re = regexp.MustCompile(`^\d{4}$`)

type CardService struct {
	repo       ports.CardRepository
	categories ports.CategoryRepository
	fx         Converter
	now        func() time.Time
}

func NewCardService(repo ports.CardRepository, categories ports.CategoryRepository, fx Converter) *CardService {
	return &CardService{repo: repo, categories: categories, fx: fx, now: time.Now}
}

// WithClock reemplaza el reloj (para tests).
func (s *CardService) WithClock(now func() time.Time) *CardService {
	s.now = now
	return s
}

// ---- Tarjetas ----

type CardInput struct {
	Name       string
	Last4      string
	ClosingDay int
	DueDay     int
}

type CardPatch struct {
	Name       *string
	Last4      *string
	ClosingDay *int
	DueDay     *int
}

func validateCard(c domain.Card) error {
	if strings.TrimSpace(c.Name) == "" {
		return invalid("el nombre es obligatorio")
	}
	if c.Last4 != "" && !last4Re.MatchString(c.Last4) {
		return invalid("last4 debe tener 4 dígitos")
	}
	if c.ClosingDay < 1 || c.ClosingDay > 31 || c.DueDay < 1 || c.DueDay > 31 {
		return invalid("closing_day y due_day deben estar entre 1 y 31")
	}
	return nil
}

func (s *CardService) Create(ctx context.Context, userID string, in CardInput) (domain.Card, error) {
	c := domain.Card{Name: strings.TrimSpace(in.Name), Last4: in.Last4, ClosingDay: in.ClosingDay, DueDay: in.DueDay, CreatedBy: domain.UserRef{ID: userID}}
	if err := validateCard(c); err != nil {
		return domain.Card{}, err
	}
	return s.repo.Create(ctx, c)
}

// Update: cambiar closing_day/due_day aplica a las compras nuevas; los resúmenes y cuotas
// ya generados conservan sus fechas.
func (s *CardService) Update(ctx context.Context, id string, p CardPatch) (domain.Card, error) {
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.Card{}, err
	}
	if p.Name != nil {
		c.Name = strings.TrimSpace(*p.Name)
	}
	if p.Last4 != nil {
		c.Last4 = *p.Last4
	}
	if p.ClosingDay != nil {
		c.ClosingDay = *p.ClosingDay
	}
	if p.DueDay != nil {
		c.DueDay = *p.DueDay
	}
	if err := validateCard(c); err != nil {
		return domain.Card{}, err
	}
	return s.repo.Update(ctx, c)
}

func (s *CardService) Delete(ctx context.Context, id string) error {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return err
	}
	has, err := s.repo.HasPurchases(ctx, id)
	if err != nil {
		return err
	}
	if has {
		return ErrConflict // borrá primero las compras de la tarjeta
	}
	return s.repo.Delete(ctx, id)
}

// ---- Vista general ----

type CardOverview struct {
	Card                  domain.Card
	OpenStatement         domain.StatementSummary
	ClosedUnpaid          []domain.StatementSummary // ya cerrados, con saldo y sin pagar
	Upcoming              []domain.StatementSummary // los 3 siguientes al abierto
	FutureInstallmentsARS decimal.Decimal           // cuotas de resúmenes que todavía no cerraron
}

func (s *CardService) List(ctx context.Context) ([]CardOverview, error) {
	cards, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CardOverview, 0, len(cards))
	for _, c := range cards {
		o, err := s.overview(ctx, c)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

func (s *CardService) Get(ctx context.Context, id string) (CardOverview, error) {
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return CardOverview{}, err
	}
	return s.overview(ctx, c)
}

func (s *CardService) overview(ctx context.Context, c domain.Card) (CardOverview, error) {
	totals, err := s.repo.InstallmentTotals(ctx, c.ID)
	if err != nil {
		return CardOverview{}, err
	}
	rows, err := s.repo.StatementRows(ctx, c.ID)
	if err != nil {
		return CardOverview{}, err
	}
	summary := func(m domain.Month) domain.StatementSummary {
		var row *domain.StatementRow
		if r, ok := rows[m]; ok {
			row = &r
		}
		return c.Summary(m, totals[m], row)
	}

	open := c.OpenMonth(s.now())
	o := CardOverview{Card: c, OpenStatement: summary(open), ClosedUnpaid: []domain.StatementSummary{}, FutureInstallmentsARS: decimal.Zero}

	months := map[domain.Month]struct{}{}
	for m := range totals {
		months[m] = struct{}{}
	}
	for m := range rows {
		months[m] = struct{}{}
	}
	for m := range months {
		switch {
		case m.Before(open):
			if sm := summary(m); !sm.Paid && !sm.Total.IsZero() {
				o.ClosedUnpaid = append(o.ClosedUnpaid, sm)
			}
		default:
			o.FutureInstallmentsARS = o.FutureInstallmentsARS.Add(totals[m])
		}
	}
	sort.Slice(o.ClosedUnpaid, func(i, j int) bool { return o.ClosedUnpaid[i].Month.Before(o.ClosedUnpaid[j].Month) })
	for i := 1; i <= 3; i++ {
		o.Upcoming = append(o.Upcoming, summary(open.AddMonths(i)))
	}
	return o, nil
}

// ---- Compras ----

type PurchaseInput struct {
	Amount       decimal.Decimal
	Currency     domain.Currency
	Installments int
	CategoryID   string
	Description  string
	PurchasedAt  *time.Time // por defecto, ahora
}

type PurchasePatch struct {
	Amount       *decimal.Decimal
	Currency     *domain.Currency
	Installments *int
	CategoryID   *string
	Description  *string
	PurchasedAt  *time.Time
}

func (s *CardService) CreatePurchase(ctx context.Context, userID, cardID string, in PurchaseInput) (domain.CardPurchase, error) {
	card, err := s.repo.Get(ctx, cardID)
	if err != nil {
		return domain.CardPurchase{}, err
	}
	if in.Installments == 0 {
		in.Installments = 1
	}
	at := s.now()
	if in.PurchasedAt != nil {
		at = *in.PurchasedAt
	}
	p := domain.CardPurchase{
		CardID: cardID, Amount: in.Amount, Currency: in.Currency, Installments: in.Installments,
		CategoryID: in.CategoryID, Description: in.Description, PurchasedAt: at,
		CreatedBy: domain.UserRef{ID: userID},
	}
	plan, dueDates, err := s.prepare(ctx, card, &p)
	if err != nil {
		return domain.CardPurchase{}, err
	}
	return s.repo.CreatePurchase(ctx, p, plan, dueDates)
}

// UpdatePurchase recalcula cotización y cuotas completas, igual que una compra nueva.
func (s *CardService) UpdatePurchase(ctx context.Context, cardID, id string, patch PurchasePatch) (domain.CardPurchase, error) {
	card, err := s.repo.Get(ctx, cardID)
	if err != nil {
		return domain.CardPurchase{}, err
	}
	p, err := s.repo.GetPurchase(ctx, cardID, id)
	if err != nil {
		return domain.CardPurchase{}, err
	}
	if patch.Amount != nil {
		p.Amount = *patch.Amount
	}
	if patch.Currency != nil {
		p.Currency = *patch.Currency
	}
	if patch.Installments != nil {
		p.Installments = *patch.Installments
	}
	if patch.CategoryID != nil {
		p.CategoryID = *patch.CategoryID
	}
	if patch.Description != nil {
		p.Description = *patch.Description
	}
	if patch.PurchasedAt != nil {
		p.PurchasedAt = *patch.PurchasedAt
	}
	plan, dueDates, err := s.prepare(ctx, card, &p)
	if err != nil {
		return domain.CardPurchase{}, err
	}
	return s.repo.UpdatePurchase(ctx, p, plan, dueDates)
}

func (s *CardService) GetPurchase(ctx context.Context, cardID, id string) (domain.CardPurchase, error) {
	return s.repo.GetPurchase(ctx, cardID, id)
}

func (s *CardService) DeletePurchase(ctx context.Context, cardID, id string) error {
	return s.repo.DeletePurchase(ctx, cardID, id)
}

type ListPurchasesQuery struct {
	Month      *time.Time // primer instante del mes en Mendoza
	CardID     *string
	CategoryID *string
	UserID     *string
	Limit      int
	Offset     int
}

type PurchasePage struct {
	Items         []domain.CardPurchase
	Total         int
	Limit, Offset int
}

func (s *CardService) ListPurchases(ctx context.Context, q ListPurchasesQuery) (PurchasePage, error) {
	if q.Limit <= 0 {
		q.Limit = DefaultPageSize
	}
	if q.Limit > MaxPageSize {
		q.Limit = MaxPageSize
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	f := ports.CardPurchaseFilter{CardID: q.CardID, CategoryID: q.CategoryID, UserID: q.UserID, Limit: q.Limit, Offset: q.Offset}
	if q.Month != nil {
		from := *q.Month
		to := from.AddDate(0, 1, 0)
		f.From, f.To = &from, &to
	}
	items, total, err := s.repo.ListPurchases(ctx, f)
	if err != nil {
		return PurchasePage{}, err
	}
	return PurchasePage{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}

// prepare valida la compra, completa fx_rate/amount_ars y arma el plan de cuotas con
// los vencimientos de los resúmenes involucrados.
func (s *CardService) prepare(ctx context.Context, card domain.Card, p *domain.CardPurchase) ([]domain.Installment, map[domain.Month]time.Time, error) {
	if !p.Currency.Valid() {
		return nil, nil, invalid("currency debe ser ARS o USD")
	}
	if !p.Amount.IsPositive() {
		return nil, nil, invalid("el monto debe ser mayor a cero")
	}
	if !p.Amount.Equal(p.Amount.Round(2)) {
		return nil, nil, invalid("el monto admite hasta 2 decimales")
	}
	if p.Installments < 1 || p.Installments > domain.MaxInstallments {
		return nil, nil, invalid("las cuotas deben estar entre 1 y %d", domain.MaxInstallments)
	}
	cat, err := s.categories.Get(ctx, p.CategoryID)
	if err != nil {
		if err == ErrNotFound {
			return nil, nil, invalid("la categoría no existe")
		}
		return nil, nil, err
	}
	if cat.Kind != domain.Expense {
		return nil, nil, invalid("las compras con tarjeta usan categorías de gasto")
	}
	fx, ars, err := s.fx.ConvertToARS(ctx, p.Amount, p.Currency, p.PurchasedAt)
	if err != nil {
		return nil, nil, err
	}
	if ars.Abs().GreaterThanOrEqual(maxARS) {
		return nil, nil, invalid("el monto es demasiado grande")
	}
	p.FxRate, p.AmountARS = fx, ars

	plan := card.PlanInstallments(p.PurchasedAt, ars, p.Installments)
	dueDates := make(map[domain.Month]time.Time, len(plan))
	for _, in := range plan {
		dueDates[in.Month] = card.DueDate(in.Month)
	}
	return plan, dueDates, nil
}

// ---- Resúmenes ----

func (s *CardService) Statement(ctx context.Context, cardID string, m domain.Month) (domain.StatementDetail, error) {
	card, err := s.repo.Get(ctx, cardID)
	if err != nil {
		return domain.StatementDetail{}, err
	}
	return s.detail(ctx, card, m)
}

func (s *CardService) detail(ctx context.Context, card domain.Card, m domain.Month) (domain.StatementDetail, error) {
	totals, err := s.repo.InstallmentTotals(ctx, card.ID)
	if err != nil {
		return domain.StatementDetail{}, err
	}
	rows, err := s.repo.StatementRows(ctx, card.ID)
	if err != nil {
		return domain.StatementDetail{}, err
	}
	items, err := s.repo.StatementItems(ctx, card.ID, m)
	if err != nil {
		return domain.StatementDetail{}, err
	}
	var row *domain.StatementRow
	if r, ok := rows[m]; ok {
		row = &r
	}
	return domain.StatementDetail{CardID: card.ID, StatementSummary: card.Summary(m, totals[m], row), Items: items}, nil
}

// SetAdjustment fija el saldo previo a usar la app. Puede ser negativo (saldo a favor).
func (s *CardService) SetAdjustment(ctx context.Context, cardID string, m domain.Month, amount decimal.Decimal) (domain.StatementDetail, error) {
	card, err := s.repo.Get(ctx, cardID)
	if err != nil {
		return domain.StatementDetail{}, err
	}
	if !amount.Equal(amount.Round(2)) {
		return domain.StatementDetail{}, invalid("el ajuste admite hasta 2 decimales")
	}
	if amount.Abs().GreaterThanOrEqual(maxARS) {
		return domain.StatementDetail{}, invalid("el monto es demasiado grande")
	}
	if err := s.repo.SetAdjustment(ctx, cardID, m, card.DueDate(m), amount); err != nil {
		return domain.StatementDetail{}, err
	}
	return s.detail(ctx, card, m)
}

// Pay marca el resumen como pagado (idempotente: conserva la primera fecha de pago).
func (s *CardService) Pay(ctx context.Context, cardID string, m domain.Month) (domain.StatementDetail, error) {
	card, err := s.repo.Get(ctx, cardID)
	if err != nil {
		return domain.StatementDetail{}, err
	}
	if err := s.repo.MarkPaid(ctx, cardID, m, card.DueDate(m), s.now()); err != nil {
		return domain.StatementDetail{}, err
	}
	return s.detail(ctx, card, m)
}
