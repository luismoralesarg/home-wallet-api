package application

import (
	"context"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
	"github.com/luismoralesarg/home-wallet-api/internal/ports"
)

type SubscriptionService struct {
	repo ports.SubscriptionRepository
	fx   Converter
	now  func() time.Time
}

func NewSubscriptionService(repo ports.SubscriptionRepository, fx Converter) *SubscriptionService {
	return &SubscriptionService{repo: repo, fx: fx, now: time.Now}
}

// WithClock reemplaza el reloj (para tests).
func (s *SubscriptionService) WithClock(now func() time.Time) *SubscriptionService {
	s.now = now
	return s
}

// SubscriptionView valúa una suscripción en ARS a la cotización de hoy.
type SubscriptionView struct {
	Subscription   domain.Subscription
	FxRate         decimal.Decimal
	AmountARS      decimal.Decimal
	NextChargeDate time.Time
}

type SubscriptionList struct {
	Items           []SubscriptionView
	MonthlyTotalARS decimal.Decimal // solo las activas
}

type SubscriptionInput struct {
	Name      string
	Amount    decimal.Decimal
	Currency  domain.Currency
	ChargeDay int
}

type SubscriptionPatch struct {
	Name      *string
	Amount    *decimal.Decimal
	Currency  *domain.Currency
	ChargeDay *int
}

func validateSubscription(sub domain.Subscription) error {
	switch {
	case strings.TrimSpace(sub.Name) == "":
		return invalid("el nombre es obligatorio")
	case !sub.Currency.Valid():
		return invalid("currency debe ser ARS o USD")
	case !sub.Amount.IsPositive():
		return invalid("el monto debe ser mayor a cero")
	case !sub.Amount.Equal(sub.Amount.Round(2)):
		return invalid("el monto admite hasta 2 decimales")
	case sub.ChargeDay < 1 || sub.ChargeDay > 31:
		return invalid("charge_day debe estar entre 1 y 31")
	}
	return nil
}

func (s *SubscriptionService) Create(ctx context.Context, userID string, in SubscriptionInput) (SubscriptionView, error) {
	sub := domain.Subscription{
		Name: strings.TrimSpace(in.Name), Amount: in.Amount, Currency: in.Currency, ChargeDay: in.ChargeDay,
		Active: true, CreatedBy: domain.UserRef{ID: userID},
	}
	if err := validateSubscription(sub); err != nil {
		return SubscriptionView{}, err
	}
	created, err := s.repo.Create(ctx, sub)
	if err != nil {
		return SubscriptionView{}, err
	}
	return s.view(ctx, created)
}

func (s *SubscriptionService) List(ctx context.Context) (SubscriptionList, error) {
	subs, err := s.repo.List(ctx)
	if err != nil {
		return SubscriptionList{}, err
	}
	out := SubscriptionList{Items: make([]SubscriptionView, 0, len(subs)), MonthlyTotalARS: decimal.Zero}
	for _, sub := range subs {
		v, err := s.view(ctx, sub)
		if err != nil {
			return SubscriptionList{}, err
		}
		out.Items = append(out.Items, v)
		if sub.Active {
			out.MonthlyTotalARS = out.MonthlyTotalARS.Add(v.AmountARS)
		}
	}
	return out, nil
}

func (s *SubscriptionService) Get(ctx context.Context, id string) (SubscriptionView, error) {
	sub, err := s.repo.Get(ctx, id)
	if err != nil {
		return SubscriptionView{}, err
	}
	return s.view(ctx, sub)
}

func (s *SubscriptionService) Update(ctx context.Context, id string, p SubscriptionPatch) (SubscriptionView, error) {
	sub, err := s.repo.Get(ctx, id)
	if err != nil {
		return SubscriptionView{}, err
	}
	if p.Name != nil {
		sub.Name = strings.TrimSpace(*p.Name)
	}
	if p.Amount != nil {
		sub.Amount = *p.Amount
	}
	if p.Currency != nil {
		sub.Currency = *p.Currency
	}
	if p.ChargeDay != nil {
		sub.ChargeDay = *p.ChargeDay
	}
	if err := validateSubscription(sub); err != nil {
		return SubscriptionView{}, err
	}
	updated, err := s.repo.Update(ctx, sub)
	if err != nil {
		return SubscriptionView{}, err
	}
	return s.view(ctx, updated)
}

// SetActive pausa (false) o reactiva (true) una suscripción.
func (s *SubscriptionService) SetActive(ctx context.Context, id string, active bool) (SubscriptionView, error) {
	sub, err := s.repo.Get(ctx, id)
	if err != nil {
		return SubscriptionView{}, err
	}
	sub.Active = active
	updated, err := s.repo.Update(ctx, sub)
	if err != nil {
		return SubscriptionView{}, err
	}
	return s.view(ctx, updated)
}

func (s *SubscriptionService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *SubscriptionService) view(ctx context.Context, sub domain.Subscription) (SubscriptionView, error) {
	now := s.now()
	fx, ars, err := s.fx.ConvertToARS(ctx, sub.Amount, sub.Currency, now)
	if err != nil {
		return SubscriptionView{}, err
	}
	return SubscriptionView{Subscription: sub, FxRate: fx, AmountARS: ars, NextChargeDate: sub.NextChargeDate(now)}, nil
}
