package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
	"github.com/luismoralesarg/home-wallet-api/internal/ports"
)

type TransactionRepo struct{ pool *pgxpool.Pool }

func NewTransactionRepo(pool *pgxpool.Pool) *TransactionRepo { return &TransactionRepo{pool: pool} }

const txSelect = `
	SELECT t.id::text, t.kind, t.amount::text, t.currency, t.fx_rate::text, t.amount_ars::text,
	       t.category_id::text, t.description, t.occurred_at, t.created_by::text, u.username, t.created_at
	FROM transactions t JOIN users u ON u.id = t.created_by`

func scanTransaction(row pgx.Row) (domain.Transaction, error) {
	var (
		t                    domain.Transaction
		amount, fx, amountAR string
	)
	err := row.Scan(&t.ID, &t.Kind, &amount, &t.Currency, &fx, &amountAR,
		&t.CategoryID, &t.Description, &t.OccurredAt, &t.CreatedBy.ID, &t.CreatedBy.Name, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, application.ErrNotFound
	}
	if err != nil {
		return t, mapPgError(err)
	}
	for dst, src := range map[*decimal.Decimal]string{&t.Amount: amount, &t.FxRate: fx, &t.AmountARS: amountAR} {
		if *dst, err = decimal.NewFromString(src); err != nil {
			return t, err
		}
	}
	return t, nil
}

func (r *TransactionRepo) Get(ctx context.Context, id string) (domain.Transaction, error) {
	return scanTransaction(r.pool.QueryRow(ctx, txSelect+` WHERE t.id = $1`, id))
}

func (r *TransactionRepo) Create(ctx context.Context, t domain.Transaction) (domain.Transaction, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO transactions (kind, amount, currency, fx_rate, amount_ars, category_id, description, created_by, occurred_at)
		VALUES ($1, $2::numeric, $3, $4::numeric, $5::numeric, $6, $7, $8, $9)
		RETURNING id::text`,
		t.Kind, t.Amount.String(), t.Currency, t.FxRate.String(), t.AmountARS.String(),
		t.CategoryID, t.Description, t.CreatedBy.ID, t.OccurredAt).Scan(&id)
	if err != nil {
		return t, mapPgError(err)
	}
	return r.Get(ctx, id)
}

func (r *TransactionRepo) Update(ctx context.Context, t domain.Transaction) (domain.Transaction, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE transactions
		SET amount = $2::numeric, currency = $3, fx_rate = $4::numeric, amount_ars = $5::numeric,
		    category_id = $6, description = $7, occurred_at = $8
		WHERE id = $1`,
		t.ID, t.Amount.String(), t.Currency, t.FxRate.String(), t.AmountARS.String(),
		t.CategoryID, t.Description, t.OccurredAt)
	if err != nil {
		return t, mapPgError(err)
	}
	if tag.RowsAffected() == 0 {
		return t, application.ErrNotFound
	}
	return r.Get(ctx, t.ID)
}

func (r *TransactionRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM transactions WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (r *TransactionRepo) List(ctx context.Context, f ports.TransactionFilter) ([]domain.Transaction, int, error) {
	var (
		where []string
		args  []any
	)
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.From != nil {
		add("t.occurred_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("t.occurred_at < $%d", *f.To)
	}
	if f.Kind != nil {
		add("t.kind = $%d", string(*f.Kind))
	}
	if f.CategoryID != nil {
		add("t.category_id = $%d", *f.CategoryID)
	}
	if f.UserID != nil {
		add("t.created_by = $%d", *f.UserID)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM transactions t`+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.Limit, f.Offset)
	rows, err := r.pool.Query(ctx,
		txSelect+clause+fmt.Sprintf(` ORDER BY t.occurred_at DESC, t.created_at DESC, t.id LIMIT $%d OFFSET $%d`, len(args)-1, len(args)),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []domain.Transaction{}
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, t)
	}
	return items, total, rows.Err()
}
