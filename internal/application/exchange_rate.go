package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
	"github.com/luismoralesarg/home-wallet-api/internal/ports"
	"github.com/luismoralesarg/home-wallet-api/internal/tz"
)

var (
	ErrNoExchangeRate  = errors.New("no hay cotización disponible")
	ErrInvalidCurrency = errors.New("moneda inválida")
)

type ExchangeRateService struct {
	repo    ports.ExchangeRateRepository
	fetcher ports.RateFetcher
	now     func() time.Time
}

func NewExchangeRateService(repo ports.ExchangeRateRepository, fetcher ports.RateFetcher) *ExchangeRateService {
	return &ExchangeRateService{repo: repo, fetcher: fetcher, now: time.Now}
}

// Refresh consulta al proveedor y guarda la cotización del día.
func (s *ExchangeRateService) Refresh(ctx context.Context) (domain.ExchangeRate, error) {
	buy, sell, err := s.fetcher.FetchOfficial(ctx)
	if err != nil {
		return domain.ExchangeRate{}, fmt.Errorf("consultando cotización: %w", err)
	}
	now := s.now()
	r := domain.ExchangeRate{Date: tz.DateOf(now), Buy: buy, Sell: sell, FetchedAt: now}
	if err := s.repo.Upsert(ctx, r); err != nil {
		return domain.ExchangeRate{}, err
	}
	return r, nil
}

// EnsureToday garantiza que exista la cotización de hoy (se usa al arrancar).
func (s *ExchangeRateService) EnsureToday(ctx context.Context) error {
	today := tz.DateOf(s.now())
	if r, ok, err := s.repo.LatestOnOrBefore(ctx, today); err != nil {
		return err
	} else if ok && r.Date.Equal(today) {
		return nil
	}
	_, err := s.Refresh(ctx)
	return err
}

// RefreshOrLog es el job diario: si el proveedor falla, loguea y se queda con la última guardada.
func (s *ExchangeRateService) RefreshOrLog(ctx context.Context) {
	if _, err := s.Refresh(ctx); err != nil {
		slog.Error("no se pudo actualizar la cotización; se usa la última guardada", "err", err)
	}
}

// RateFor devuelve la cotización que corresponde a una fecha: la de ese día o,
// si no hay, la más reciente anterior. Para hoy intenta refrescar antes de caer a la última.
func (s *ExchangeRateService) RateFor(ctx context.Context, date time.Time) (domain.ExchangeRate, error) {
	day := tz.DateOf(date)
	latest, ok, err := s.repo.LatestOnOrBefore(ctx, day)
	if err != nil {
		return domain.ExchangeRate{}, err
	}
	if ok && latest.Date.Equal(day) {
		return latest, nil
	}
	if day.Equal(tz.DateOf(s.now())) {
		if r, err := s.Refresh(ctx); err == nil {
			return r, nil
		} else {
			slog.Error("no se pudo obtener la cotización de hoy; se usa la última guardada", "err", err)
		}
	}
	if ok {
		return latest, nil
	}
	return domain.ExchangeRate{}, ErrNoExchangeRate
}

// Today es la cotización de hoy (o la última disponible).
func (s *ExchangeRateService) Today(ctx context.Context) (domain.ExchangeRate, error) {
	return s.RateFor(ctx, s.now())
}

// ConvertToARS convierte un monto a ARS con la venta del día de `date`.
// Devuelve la cotización usada (1 para ARS) y el monto en ARS, para guardarlos en el movimiento.
// Los demás módulos deben reutilizar este método.
func (s *ExchangeRateService) ConvertToARS(ctx context.Context, amount decimal.Decimal, currency domain.Currency, date time.Time) (fx, ars decimal.Decimal, err error) {
	switch currency {
	case domain.ARS:
		fx, ars = domain.ToARS(amount, currency, nil)
		return fx, ars, nil
	case domain.USD:
		r, err := s.RateFor(ctx, date)
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		fx, ars = domain.ToARS(amount, currency, &r)
		return fx, ars, nil
	default:
		return decimal.Zero, decimal.Zero, ErrInvalidCurrency
	}
}
