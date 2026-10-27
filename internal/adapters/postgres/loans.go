package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type LoanRepo struct{ pool *pgxpool.Pool }

func NewLoanRepo(pool *pgxpool.Pool) *LoanRepo { return &LoanRepo{pool: pool} }

const loanSelect = `
	SELECT l.id::text, l.name, l.kind, l.currency, l.base_amount::text, l.start_month, l.end_month,
	       l.created_by::text, u.username, l.created_at
	FROM loans l JOIN users u ON u.id = l.created_by`

func scanLoan(row pgx.Row) (domain.Loan, error) {
	var (
		l          domain.Loan
		base       string
		start, end time.Time
	)
	err := row.Scan(&l.ID, &l.Name, &l.Kind, &l.Currency, &base, &start, &end, &l.CreatedBy.ID, &l.CreatedBy.Name, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, application.ErrNotFound
	}
	if err != nil {
		return l, mapPgError(err)
	}
	var d decScan
	l.BaseAmount = d.parse(base)
	l.StartMonth, l.EndMonth = domain.MonthOfDate(start), domain.MonthOfDate(end)
	return l, d.err
}

func (r *LoanRepo) Create(ctx context.Context, l domain.Loan, installments []domain.LoanInstallment) (domain.Loan, error) {
	var id string
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO loans (name, kind, currency, base_amount, start_month, end_month, created_by)
			VALUES ($1, $2, $3, $4::numeric, $5, $6, $7) RETURNING id::text`,
			l.Name, l.Kind, l.Currency, l.BaseAmount.String(), l.StartMonth.Start(), l.EndMonth.Start(), l.CreatedBy.ID).Scan(&id); err != nil {
			return err
		}
		for _, in := range installments {
			if _, err := tx.Exec(ctx, `
				INSERT INTO loan_installments (loan_id, month, amount, amount_ars, paid)
				VALUES ($1, $2, $3::numeric, $4::numeric, $5)`,
				id, in.Month.Start(), in.Amount.String(), in.AmountARS.String(), in.Paid); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return l, mapPgError(err)
	}
	return r.Get(ctx, id)
}

func (r *LoanRepo) List(ctx context.Context) ([]domain.Loan, error) {
	rows, err := r.pool.Query(ctx, loanSelect+` ORDER BY l.kind DESC, l.created_at, l.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Loan{}
	for rows.Next() {
		l, err := scanLoan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *LoanRepo) Get(ctx context.Context, id string) (domain.Loan, error) {
	return scanLoan(r.pool.QueryRow(ctx, loanSelect+` WHERE l.id = $1`, id))
}

func (r *LoanRepo) Rename(ctx context.Context, id, name string) (domain.Loan, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE loans SET name = $2 WHERE id = $1`, id, name)
	if err != nil {
		return domain.Loan{}, err
	}
	if tag.RowsAffected() == 0 {
		return domain.Loan{}, application.ErrNotFound
	}
	return r.Get(ctx, id)
}

func (r *LoanRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM loans WHERE id = $1`, id) // las cuotas caen en cascada
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (r *LoanRepo) Installments(ctx context.Context, loanID string) ([]domain.LoanInstallment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT month, amount::text, amount_ars::text, paid FROM loan_installments
		WHERE loan_id = $1 ORDER BY month`, loanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.LoanInstallment{}
	for rows.Next() {
		var (
			m        time.Time
			amt, ars string
			in       domain.LoanInstallment
		)
		if err := rows.Scan(&m, &amt, &ars, &in.Paid); err != nil {
			return nil, err
		}
		var d decScan
		in.Month, in.Amount, in.AmountARS = domain.MonthOfDate(m), d.parse(amt), d.parse(ars)
		if d.err != nil {
			return nil, d.err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

func (r *LoanRepo) SaveInstallments(ctx context.Context, loanID string, installments []domain.LoanInstallment) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		for _, in := range installments {
			tag, err := tx.Exec(ctx, `
				UPDATE loan_installments SET amount = $3::numeric, amount_ars = $4::numeric, paid = $5
				WHERE loan_id = $1 AND month = $2`,
				loanID, in.Month.Start(), in.Amount.String(), in.AmountARS.String(), in.Paid)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return application.ErrNotFound
			}
		}
		return nil
	})
}
