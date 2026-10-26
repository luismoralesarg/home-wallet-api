package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type SubscriptionRepo struct{ pool *pgxpool.Pool }

func NewSubscriptionRepo(pool *pgxpool.Pool) *SubscriptionRepo { return &SubscriptionRepo{pool: pool} }

const subSelect = `
	SELECT s.id::text, s.name, s.amount::text, s.currency, s.charge_day, s.active, s.created_by::text, u.username, s.created_at
	FROM subscriptions s JOIN users u ON u.id = s.created_by`

func scanSubscription(row pgx.Row) (domain.Subscription, error) {
	var (
		s      domain.Subscription
		amount string
	)
	err := row.Scan(&s.ID, &s.Name, &amount, &s.Currency, &s.ChargeDay, &s.Active, &s.CreatedBy.ID, &s.CreatedBy.Name, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, application.ErrNotFound
	}
	if err != nil {
		return s, mapPgError(err)
	}
	var d decScan
	s.Amount = d.parse(amount)
	return s, d.err
}

func (r *SubscriptionRepo) List(ctx context.Context) ([]domain.Subscription, error) {
	rows, err := r.pool.Query(ctx, subSelect+` ORDER BY s.active DESC, s.charge_day, s.name, s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Subscription{}
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *SubscriptionRepo) Get(ctx context.Context, id string) (domain.Subscription, error) {
	return scanSubscription(r.pool.QueryRow(ctx, subSelect+` WHERE s.id = $1`, id))
}

func (r *SubscriptionRepo) Create(ctx context.Context, s domain.Subscription) (domain.Subscription, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO subscriptions (name, amount, currency, charge_day, active, created_by)
		VALUES ($1, $2::numeric, $3, $4, $5, $6) RETURNING id::text`,
		s.Name, s.Amount.String(), s.Currency, s.ChargeDay, s.Active, s.CreatedBy.ID).Scan(&id)
	if err != nil {
		return s, mapPgError(err)
	}
	return r.Get(ctx, id)
}

func (r *SubscriptionRepo) Update(ctx context.Context, s domain.Subscription) (domain.Subscription, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE subscriptions SET name = $2, amount = $3::numeric, currency = $4, charge_day = $5, active = $6
		WHERE id = $1`, s.ID, s.Name, s.Amount.String(), s.Currency, s.ChargeDay, s.Active)
	if err != nil {
		return s, mapPgError(err)
	}
	if tag.RowsAffected() == 0 {
		return s, application.ErrNotFound
	}
	return r.Get(ctx, s.ID)
}

func (r *SubscriptionRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM subscriptions WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return application.ErrNotFound
	}
	return nil
}
