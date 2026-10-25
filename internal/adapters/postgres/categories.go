package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type CategoryRepo struct{ pool *pgxpool.Pool }

func NewCategoryRepo(pool *pgxpool.Pool) *CategoryRepo { return &CategoryRepo{pool: pool} }

const categoryCols = `id::text, name, kind, created_at`

func scanCategory(row pgx.Row) (domain.Category, error) {
	var c domain.Category
	err := row.Scan(&c.ID, &c.Name, &c.Kind, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, application.ErrNotFound
	}
	return c, mapPgError(err)
}

func (r *CategoryRepo) List(ctx context.Context, kind *domain.Kind) ([]domain.Category, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+categoryCols+` FROM categories WHERE ($1::text IS NULL OR kind = $1) ORDER BY kind, name`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Category{}
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *CategoryRepo) Get(ctx context.Context, id string) (domain.Category, error) {
	return scanCategory(r.pool.QueryRow(ctx, `SELECT `+categoryCols+` FROM categories WHERE id = $1`, id))
}

func (r *CategoryRepo) Create(ctx context.Context, c domain.Category) (domain.Category, error) {
	return scanCategory(r.pool.QueryRow(ctx,
		`INSERT INTO categories (name, kind) VALUES ($1, $2) RETURNING `+categoryCols, c.Name, c.Kind))
}

func (r *CategoryRepo) Update(ctx context.Context, id, name string) (domain.Category, error) {
	return scanCategory(r.pool.QueryRow(ctx,
		`UPDATE categories SET name = $2 WHERE id = $1 RETURNING `+categoryCols, id, name))
}

func (r *CategoryRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, id)
	if err != nil {
		return mapPgError(err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrNotFound
	}
	return nil
}

// mapPgError traduce unique/foreign key violations a errores de la aplicación.
func mapPgError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23503") {
		return application.ErrConflict
	}
	return err
}
