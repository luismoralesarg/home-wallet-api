package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type ExchangeRateRepo struct{ pool *pgxpool.Pool }

func NewExchangeRateRepo(pool *pgxpool.Pool) *ExchangeRateRepo { return &ExchangeRateRepo{pool: pool} }

func (r *ExchangeRateRepo) Upsert(ctx context.Context, er domain.ExchangeRate) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO exchange_rates (date, currency, official_buy, official_sell, fetched_at)
		VALUES ($1, 'USD', $2, $3, $4)
		ON CONFLICT (date) DO UPDATE
		SET official_buy = EXCLUDED.official_buy,
		    official_sell = EXCLUDED.official_sell,
		    fetched_at = EXCLUDED.fetched_at`,
		er.Date, er.Buy.String(), er.Sell.String(), er.FetchedAt)
	return err
}

func (r *ExchangeRateRepo) LatestOnOrBefore(ctx context.Context, date time.Time) (domain.ExchangeRate, bool, error) {
	var (
		er        domain.ExchangeRate
		buy, sell string
	)
	err := r.pool.QueryRow(ctx, `
		SELECT date, official_buy::text, official_sell::text, fetched_at
		FROM exchange_rates WHERE date <= $1 ORDER BY date DESC LIMIT 1`, date).
		Scan(&er.Date, &buy, &sell, &er.FetchedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return er, false, nil
	}
	if err != nil {
		return er, false, err
	}
	if er.Buy, err = decimal.NewFromString(buy); err != nil {
		return er, false, err
	}
	if er.Sell, err = decimal.NewFromString(sell); err != nil {
		return er, false, err
	}
	er.Date = time.Date(er.Date.Year(), er.Date.Month(), er.Date.Day(), 0, 0, 0, 0, time.UTC)
	return er, true, nil
}
