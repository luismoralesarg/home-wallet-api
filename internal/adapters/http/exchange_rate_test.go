package http_test

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/adapters/http"
	"github.com/luismoralesarg/home-wallet-api/internal/adapters/jwt"
	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
	"github.com/luismoralesarg/home-wallet-api/internal/tz"
)

type rateRepo struct{ rates []domain.ExchangeRate }

func (r *rateRepo) Upsert(_ context.Context, er domain.ExchangeRate) error {
	r.rates = append(r.rates, er)
	return nil
}
func (r *rateRepo) LatestOnOrBefore(_ context.Context, d time.Time) (domain.ExchangeRate, bool, error) {
	for _, x := range r.rates {
		if !x.Date.After(d) {
			return x, true, nil
		}
	}
	return domain.ExchangeRate{}, false, nil
}

type noFetcher struct{}

func (noFetcher) FetchOfficial(context.Context) (decimal.Decimal, decimal.Decimal, error) {
	return decimal.Zero, decimal.Zero, io.ErrUnexpectedEOF
}

func getRate(t *testing.T, repo *rateRepo) (int, string) {
	t.Helper()
	jwts := jwt.New("0123456789abcdef-secret", time.Hour)
	app := http.NewApp(http.Deps{
		Auth:         application.NewAuthService(users{}, jwts),
		ExchangeRate: application.NewExchangeRateService(repo, noFetcher{}),
	})
	tok, _, _ := jwts.Issue("u-1")
	req := httptest.NewRequest("GET", "/exchange-rate/today", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestExchangeRateToday(t *testing.T) {
	repo := &rateRepo{rates: []domain.ExchangeRate{{
		Date: tz.DateOf(time.Now()), Buy: decimal.RequireFromString("1385.5"), Sell: decimal.RequireFromString("1435"), FetchedAt: time.Now(),
	}}}
	status, body := getRate(t, repo)
	if status != 200 || !strings.Contains(body, `"official_sell":"1435.00"`) || !strings.Contains(body, `"official_buy":"1385.50"`) {
		t.Fatalf("%d %s", status, body)
	}
}

func TestExchangeRateTodayUnavailable(t *testing.T) {
	status, body := getRate(t, &rateRepo{})
	if status != 503 || !strings.Contains(body, `"code":"exchange_rate_unavailable"`) {
		t.Fatalf("%d %s", status, body)
	}
}
