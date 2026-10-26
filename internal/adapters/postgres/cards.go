package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
	"github.com/luismoralesarg/home-wallet-api/internal/ports"
)

type CardRepo struct{ pool *pgxpool.Pool }

func NewCardRepo(pool *pgxpool.Pool) *CardRepo { return &CardRepo{pool: pool} }

// decScan acumula el primer error de parseo para no repetir chequeos en cada columna.
type decScan struct{ err error }

func (d *decScan) parse(s string) decimal.Decimal {
	v, err := decimal.NewFromString(s)
	if err != nil && d.err == nil {
		d.err = err
	}
	return v
}

// ---- Tarjetas ----

const cardSelect = `
	SELECT c.id::text, c.name, c.last4, c.closing_day, c.due_day, c.created_by::text, u.username, c.created_at
	FROM cards c JOIN users u ON u.id = c.created_by`

func scanCard(row pgx.Row) (domain.Card, error) {
	var c domain.Card
	err := row.Scan(&c.ID, &c.Name, &c.Last4, &c.ClosingDay, &c.DueDay, &c.CreatedBy.ID, &c.CreatedBy.Name, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, application.ErrNotFound
	}
	return c, mapPgError(err)
}

func (r *CardRepo) List(ctx context.Context) ([]domain.Card, error) {
	rows, err := r.pool.Query(ctx, cardSelect+` ORDER BY c.created_at, c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Card{}
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *CardRepo) Get(ctx context.Context, id string) (domain.Card, error) {
	return scanCard(r.pool.QueryRow(ctx, cardSelect+` WHERE c.id = $1`, id))
}

func (r *CardRepo) Create(ctx context.Context, c domain.Card) (domain.Card, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO cards (name, last4, closing_day, due_day, created_by) VALUES ($1, $2, $3, $4, $5) RETURNING id::text`,
		c.Name, c.Last4, c.ClosingDay, c.DueDay, c.CreatedBy.ID).Scan(&id)
	if err != nil {
		return c, mapPgError(err)
	}
	return r.Get(ctx, id)
}

func (r *CardRepo) Update(ctx context.Context, c domain.Card) (domain.Card, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE cards SET name = $2, last4 = $3, closing_day = $4, due_day = $5 WHERE id = $1`,
		c.ID, c.Name, c.Last4, c.ClosingDay, c.DueDay)
	if err != nil {
		return c, mapPgError(err)
	}
	if tag.RowsAffected() == 0 {
		return c, application.ErrNotFound
	}
	return r.Get(ctx, c.ID)
}

func (r *CardRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM cards WHERE id = $1`, id)
	if err != nil {
		return mapPgError(err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (r *CardRepo) HasPurchases(ctx context.Context, id string) (bool, error) {
	var has bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM card_purchases WHERE card_id = $1)`, id).Scan(&has)
	return has, err
}

// ---- Compras ----

// Cada compra trae el mes y el vencimiento del resumen de su primera cuota.
const purchaseSelect = `
	SELECT p.id::text, p.card_id::text, p.amount::text, p.currency, p.fx_rate::text, p.amount_ars::text,
	       p.installments, p.category_id::text, p.description, p.purchased_at,
	       p.created_by::text, u.username, p.created_at, fi.statement_month, st.due_date
	FROM card_purchases p
	JOIN users u ON u.id = p.created_by
	JOIN card_purchase_installments fi ON fi.purchase_id = p.id AND fi.installment_number = 1
	JOIN card_statements st ON st.card_id = p.card_id AND st.month = fi.statement_month`

func scanPurchase(row pgx.Row) (domain.CardPurchase, error) {
	var (
		p                   domain.CardPurchase
		amount, fx, ars     string
		firstMonth, dueDate time.Time
	)
	err := row.Scan(&p.ID, &p.CardID, &amount, &p.Currency, &fx, &ars, &p.Installments, &p.CategoryID, &p.Description,
		&p.PurchasedAt, &p.CreatedBy.ID, &p.CreatedBy.Name, &p.CreatedAt, &firstMonth, &dueDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, application.ErrNotFound
	}
	if err != nil {
		return p, mapPgError(err)
	}
	var d decScan
	p.Amount, p.FxRate, p.AmountARS = d.parse(amount), d.parse(fx), d.parse(ars)
	p.FirstStatement = domain.MonthOfDate(firstMonth)
	p.StatementDueDate = time.Date(dueDate.Year(), dueDate.Month(), dueDate.Day(), 0, 0, 0, 0, time.UTC)
	return p, d.err
}

func saveSchedule(ctx context.Context, tx pgx.Tx, cardID, purchaseID string, plan []domain.Installment, dueDates map[domain.Month]time.Time) error {
	for _, in := range plan {
		if _, err := tx.Exec(ctx, `
			INSERT INTO card_purchase_installments (purchase_id, installment_number, statement_month, amount_ars)
			VALUES ($1, $2, $3, $4::numeric)`,
			purchaseID, in.Number, in.Month.Start(), in.AmountARS.String()); err != nil {
			return err
		}
	}
	for m, due := range dueDates {
		if _, err := tx.Exec(ctx, `
			INSERT INTO card_statements (card_id, month, due_date) VALUES ($1, $2, $3)
			ON CONFLICT (card_id, month) DO NOTHING`, cardID, m.Start(), due); err != nil {
			return err
		}
	}
	return nil
}

func (r *CardRepo) CreatePurchase(ctx context.Context, p domain.CardPurchase, plan []domain.Installment, dueDates map[domain.Month]time.Time) (domain.CardPurchase, error) {
	var id string
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO card_purchases (card_id, amount, currency, fx_rate, amount_ars, installments, category_id, description, created_by, purchased_at)
			VALUES ($1, $2::numeric, $3, $4::numeric, $5::numeric, $6, $7, $8, $9, $10) RETURNING id::text`,
			p.CardID, p.Amount.String(), p.Currency, p.FxRate.String(), p.AmountARS.String(), p.Installments,
			p.CategoryID, p.Description, p.CreatedBy.ID, p.PurchasedAt).Scan(&id); err != nil {
			return err
		}
		return saveSchedule(ctx, tx, p.CardID, id, plan, dueDates)
	})
	if err != nil {
		return p, mapPgError(err)
	}
	return r.GetPurchase(ctx, p.CardID, id)
}

func (r *CardRepo) UpdatePurchase(ctx context.Context, p domain.CardPurchase, plan []domain.Installment, dueDates map[domain.Month]time.Time) (domain.CardPurchase, error) {
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE card_purchases
			SET amount = $3::numeric, currency = $4, fx_rate = $5::numeric, amount_ars = $6::numeric,
			    installments = $7, category_id = $8, description = $9, purchased_at = $10
			WHERE id = $1 AND card_id = $2`,
			p.ID, p.CardID, p.Amount.String(), p.Currency, p.FxRate.String(), p.AmountARS.String(),
			p.Installments, p.CategoryID, p.Description, p.PurchasedAt)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return application.ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM card_purchase_installments WHERE purchase_id = $1`, p.ID); err != nil {
			return err
		}
		return saveSchedule(ctx, tx, p.CardID, p.ID, plan, dueDates)
	})
	if err != nil {
		return p, mapPgError(err)
	}
	return r.GetPurchase(ctx, p.CardID, p.ID)
}

func (r *CardRepo) GetPurchase(ctx context.Context, cardID, id string) (domain.CardPurchase, error) {
	return scanPurchase(r.pool.QueryRow(ctx, purchaseSelect+` WHERE p.id = $1 AND p.card_id = $2`, id, cardID))
}

func (r *CardRepo) DeletePurchase(ctx context.Context, cardID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM card_purchases WHERE id = $1 AND card_id = $2`, id, cardID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (r *CardRepo) ListPurchases(ctx context.Context, f ports.CardPurchaseFilter) ([]domain.CardPurchase, int, error) {
	var (
		where []string
		args  []any
	)
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.From != nil {
		add("p.purchased_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("p.purchased_at < $%d", *f.To)
	}
	if f.CardID != nil {
		add("p.card_id = $%d", *f.CardID)
	}
	if f.CategoryID != nil {
		add("p.category_id = $%d", *f.CategoryID)
	}
	if f.UserID != nil {
		add("p.created_by = $%d", *f.UserID)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM card_purchases p`+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := r.pool.Query(ctx,
		purchaseSelect+clause+fmt.Sprintf(` ORDER BY p.purchased_at DESC, p.created_at DESC, p.id LIMIT $%d OFFSET $%d`, len(args)-1, len(args)),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []domain.CardPurchase{}
	for rows.Next() {
		p, err := scanPurchase(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, p)
	}
	return items, total, rows.Err()
}

// ---- Resúmenes ----

func (r *CardRepo) InstallmentTotals(ctx context.Context, cardID string) (map[domain.Month]decimal.Decimal, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT i.statement_month, sum(i.amount_ars)::text
		FROM card_purchase_installments i JOIN card_purchases p ON p.id = i.purchase_id
		WHERE p.card_id = $1 GROUP BY i.statement_month`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[domain.Month]decimal.Decimal{}
	for rows.Next() {
		var (
			m   time.Time
			sum string
		)
		if err := rows.Scan(&m, &sum); err != nil {
			return nil, err
		}
		var d decScan
		out[domain.MonthOfDate(m)] = d.parse(sum)
		if d.err != nil {
			return nil, d.err
		}
	}
	return out, rows.Err()
}

func (r *CardRepo) StatementRows(ctx context.Context, cardID string) (map[domain.Month]domain.StatementRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT month, due_date, previous_balance_adjustment_ars::text, paid, paid_at
		FROM card_statements WHERE card_id = $1`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[domain.Month]domain.StatementRow{}
	for rows.Next() {
		var (
			m, due time.Time
			adj    string
			row    domain.StatementRow
		)
		if err := rows.Scan(&m, &due, &adj, &row.Paid, &row.PaidAt); err != nil {
			return nil, err
		}
		var d decScan
		row.CardID, row.Month = cardID, domain.MonthOfDate(m)
		row.DueDate = time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, time.UTC)
		row.Adjustment = d.parse(adj)
		if d.err != nil {
			return nil, d.err
		}
		out[row.Month] = row
	}
	return out, rows.Err()
}

func (r *CardRepo) StatementItems(ctx context.Context, cardID string, m domain.Month) ([]domain.StatementItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id::text, p.description, p.purchased_at, i.installment_number, p.installments, i.amount_ars::text,
		       p.amount::text, p.currency, p.category_id::text, p.created_by::text, u.username
		FROM card_purchase_installments i
		JOIN card_purchases p ON p.id = i.purchase_id
		JOIN users u ON u.id = p.created_by
		WHERE p.card_id = $1 AND i.statement_month = $2
		ORDER BY p.purchased_at, p.id, i.installment_number`, cardID, m.Start())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.StatementItem{}
	for rows.Next() {
		var (
			it       domain.StatementItem
			ars, amt string
		)
		if err := rows.Scan(&it.PurchaseID, &it.Description, &it.PurchasedAt, &it.InstallmentNum, &it.Installments, &ars,
			&amt, &it.OriginalCurrency, &it.CategoryID, &it.CreatedBy.ID, &it.CreatedBy.Name); err != nil {
			return nil, err
		}
		var d decScan
		it.AmountARS, it.OriginalAmount = d.parse(ars), d.parse(amt)
		if d.err != nil {
			return nil, d.err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *CardRepo) SetAdjustment(ctx context.Context, cardID string, m domain.Month, dueDate time.Time, adjustment decimal.Decimal) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO card_statements (card_id, month, due_date, previous_balance_adjustment_ars)
		VALUES ($1, $2, $3, $4::numeric)
		ON CONFLICT (card_id, month) DO UPDATE SET previous_balance_adjustment_ars = EXCLUDED.previous_balance_adjustment_ars`,
		cardID, m.Start(), dueDate, adjustment.String())
	return err
}

func (r *CardRepo) MarkPaid(ctx context.Context, cardID string, m domain.Month, dueDate time.Time, paidAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO card_statements (card_id, month, due_date, paid, paid_at) VALUES ($1, $2, $3, true, $4)
		ON CONFLICT (card_id, month) DO UPDATE
		SET paid = true, paid_at = COALESCE(card_statements.paid_at, EXCLUDED.paid_at)`,
		cardID, m.Start(), dueDate, paidAt)
	return err
}
