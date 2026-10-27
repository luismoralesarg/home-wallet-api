package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
	"github.com/luismoralesarg/home-wallet-api/internal/ports"
)

type DashboardRepo struct{ pool *pgxpool.Pool }

func NewDashboardRepo(pool *pgxpool.Pool) *DashboardRepo { return &DashboardRepo{pool: pool} }

func (r *DashboardRepo) SumTransactions(ctx context.Context, kind domain.Kind, from, to time.Time) (decimal.Decimal, error) {
	var sum string
	err := r.pool.QueryRow(ctx, `
		SELECT coalesce(sum(amount_ars), 0)::text FROM transactions
		WHERE kind = $1 AND occurred_at >= $2 AND occurred_at < $3`, string(kind), from, to).Scan(&sum)
	if err != nil {
		return decimal.Zero, err
	}
	return decimal.NewFromString(sum)
}

func (r *DashboardRepo) StatementsDue(ctx context.Context, from, to time.Time) ([]ports.DueStatement, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.card_id::text, c.name, s.month, s.due_date,
		       (coalesce(i.total, 0) + s.previous_balance_adjustment_ars)::text, s.paid
		FROM card_statements s
		JOIN cards c ON c.id = s.card_id
		LEFT JOIN (
			SELECT p.card_id, ci.statement_month, sum(ci.amount_ars) AS total
			FROM card_purchase_installments ci JOIN card_purchases p ON p.id = ci.purchase_id
			GROUP BY p.card_id, ci.statement_month
		) i ON i.card_id = s.card_id AND i.statement_month = s.month
		WHERE s.due_date >= $1 AND s.due_date < $2
		ORDER BY s.due_date, c.name, s.month`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ports.DueStatement{}
	for rows.Next() {
		var (
			ds         ports.DueStatement
			month, due time.Time
			total      string
		)
		if err := rows.Scan(&ds.CardID, &ds.CardName, &month, &due, &total, &ds.Paid); err != nil {
			return nil, err
		}
		var d decScan
		ds.Month = domain.MonthOfDate(month)
		ds.DueDate = time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, time.UTC)
		ds.TotalARS = d.parse(total)
		if d.err != nil {
			return nil, d.err
		}
		out = append(out, ds)
	}
	return out, rows.Err()
}

func (r *DashboardRepo) ExpenseByCategory(ctx context.Context, from, to time.Time) ([]ports.CategoryTotal, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id::text, c.name, sum(x.amount_ars)::text
		FROM (
			SELECT category_id, amount_ars FROM transactions
			WHERE kind = 'expense' AND occurred_at >= $1 AND occurred_at < $2
			UNION ALL
			SELECT category_id, amount_ars FROM card_purchases
			WHERE purchased_at >= $1 AND purchased_at < $2
		) x JOIN categories c ON c.id = x.category_id
		GROUP BY c.id, c.name
		ORDER BY sum(x.amount_ars) DESC, c.name`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ports.CategoryTotal{}
	for rows.Next() {
		var (
			ct    ports.CategoryTotal
			total string
		)
		if err := rows.Scan(&ct.CategoryID, &ct.Name, &total); err != nil {
			return nil, err
		}
		var d decScan
		ct.Total = d.parse(total)
		if d.err != nil {
			return nil, d.err
		}
		out = append(out, ct)
	}
	return out, rows.Err()
}
