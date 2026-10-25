package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type memRates struct {
	byDate map[string]domain.ExchangeRate
}

func (m *memRates) Upsert(_ context.Context, r domain.ExchangeRate) error {
	m.byDate[r.Date.Format("2006-01-02")] = r
	return nil
}

func (m *memRates) LatestOnOrBefore(_ context.Context, d time.Time) (domain.ExchangeRate, bool, error) {
	var best domain.ExchangeRate
	found := false
	for _, r := range m.byDate {
		if !r.Date.After(d) && (!found || r.Date.After(best.Date)) {
			best, found = r, true
		}
	}
	return best, found, nil
}

type fakeFetcher struct {
	buy, sell string
	err       error
	calls     int
}

func (f *fakeFetcher) FetchOfficial(context.Context) (decimal.Decimal, decimal.Decimal, error) {
	f.calls++
	if f.err != nil {
		return decimal.Zero, decimal.Zero, f.err
	}
	return decimal.RequireFromString(f.buy), decimal.RequireFromString(f.sell), nil
}

func newFx(now time.Time, f *fakeFetcher) (*ExchangeRateService, *memRates) {
	repo := &memRates{byDate: map[string]domain.ExchangeRate{}}
	s := NewExchangeRateService(repo, f)
	s.now = func() time.Time { return now }
	return s, repo
}

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestConvertToARS(t *testing.T) {
	// 12:00 en Mendoza (UTC-3) del 15/10/2026.
	now := time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)
	ctx := context.Background()

	t.Run("ARS no consulta cotización y fx = 1", func(t *testing.T) {
		f := &fakeFetcher{err: errors.New("no debería llamarse")}
		s, _ := newFx(now, f)
		fx, ars, err := s.ConvertToARS(ctx, decimal.RequireFromString("1500.50"), domain.ARS, now)
		if err != nil || !fx.Equal(decimal.NewFromInt(1)) || !ars.Equal(decimal.RequireFromString("1500.50")) || f.calls != 0 {
			t.Fatalf("fx=%v ars=%v err=%v calls=%d", fx, ars, err, f.calls)
		}
	})

	t.Run("USD usa la venta del día y redondea a centavos", func(t *testing.T) {
		s, repo := newFx(now, &fakeFetcher{err: errors.New("no debería llamarse")})
		_ = repo.Upsert(ctx, domain.ExchangeRate{Date: day("2026-10-15"), Buy: decimal.RequireFromString("1400"), Sell: decimal.RequireFromString("1450.33")})
		fx, ars, err := s.ConvertToARS(ctx, decimal.RequireFromString("10.015"), domain.USD, now)
		if err != nil {
			t.Fatal(err)
		}
		// 10.015 * 1450.33 = 14525.05495 -> 14525.05
		if !fx.Equal(decimal.RequireFromString("1450.33")) || !ars.Equal(decimal.RequireFromString("14525.05")) {
			t.Fatalf("fx=%v ars=%v", fx, ars)
		}
	})

	t.Run("USD de un día pasado usa la cotización de ese día, no la de hoy", func(t *testing.T) {
		s, repo := newFx(now, &fakeFetcher{buy: "1", sell: "9999"})
		_ = repo.Upsert(ctx, domain.ExchangeRate{Date: day("2026-10-10"), Sell: decimal.RequireFromString("1000")})
		_ = repo.Upsert(ctx, domain.ExchangeRate{Date: day("2026-10-15"), Sell: decimal.RequireFromString("1500")})
		fx, _, err := s.ConvertToARS(ctx, decimal.NewFromInt(1), domain.USD, time.Date(2026, 10, 10, 18, 0, 0, 0, time.UTC))
		if err != nil || !fx.Equal(decimal.NewFromInt(1000)) {
			t.Fatalf("fx=%v err=%v", fx, err)
		}
	})

	t.Run("hoy sin cotización guardada la trae del proveedor y la persiste", func(t *testing.T) {
		f := &fakeFetcher{buy: "1380", sell: "1420"}
		s, repo := newFx(now, f)
		fx, ars, err := s.ConvertToARS(ctx, decimal.NewFromInt(2), domain.USD, now)
		if err != nil || !fx.Equal(decimal.NewFromInt(1420)) || !ars.Equal(decimal.NewFromInt(2840)) {
			t.Fatalf("fx=%v ars=%v err=%v", fx, ars, err)
		}
		if _, ok := repo.byDate["2026-10-15"]; !ok {
			t.Fatal("no se guardó la cotización del día")
		}
	})

	t.Run("si el proveedor falla, usa la última guardada", func(t *testing.T) {
		s, repo := newFx(now, &fakeFetcher{err: errors.New("dolarapi caída")})
		_ = repo.Upsert(ctx, domain.ExchangeRate{Date: day("2026-10-13"), Sell: decimal.RequireFromString("1390")})
		fx, _, err := s.ConvertToARS(ctx, decimal.NewFromInt(1), domain.USD, now)
		if err != nil || !fx.Equal(decimal.NewFromInt(1390)) {
			t.Fatalf("fx=%v err=%v", fx, err)
		}
	})

	t.Run("sin cotización y sin proveedor devuelve ErrNoExchangeRate", func(t *testing.T) {
		s, _ := newFx(now, &fakeFetcher{err: errors.New("caída")})
		if _, _, err := s.ConvertToARS(ctx, decimal.NewFromInt(1), domain.USD, now); !errors.Is(err, ErrNoExchangeRate) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("moneda inválida", func(t *testing.T) {
		s, _ := newFx(now, &fakeFetcher{})
		if _, _, err := s.ConvertToARS(ctx, decimal.NewFromInt(1), "EUR", now); !errors.Is(err, ErrInvalidCurrency) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestRefreshUsesMendozaDay(t *testing.T) {
	// 01:00 UTC del 16/10 = 22:00 del 15/10 en Mendoza.
	now := time.Date(2026, 10, 16, 1, 0, 0, 0, time.UTC)
	s, repo := newFx(now, &fakeFetcher{buy: "1", sell: "2"})
	r, err := s.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Date.Format("2006-01-02"); got != "2026-10-15" {
		t.Fatalf("fecha = %s, quería 2026-10-15", got)
	}
	if _, ok := repo.byDate["2026-10-15"]; !ok {
		t.Fatal("no se guardó")
	}
}

func TestEnsureToday(t *testing.T) {
	now := time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)
	f := &fakeFetcher{buy: "1", sell: "2"}
	s, repo := newFx(now, f)
	_ = repo.Upsert(context.Background(), domain.ExchangeRate{Date: day("2026-10-15"), Sell: decimal.NewFromInt(5)})
	if err := s.EnsureToday(context.Background()); err != nil || f.calls != 0 {
		t.Fatalf("no debería consultar si ya hay cotización de hoy (err=%v calls=%d)", err, f.calls)
	}
	s2, _ := newFx(now, f)
	if err := s2.EnsureToday(context.Background()); err != nil || f.calls != 1 {
		t.Fatalf("debería consultar una vez (err=%v calls=%d)", err, f.calls)
	}
}
