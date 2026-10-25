package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type UserRepo struct{ pool *pgxpool.Pool }

func NewUserRepo(pool *pgxpool.Pool) *UserRepo { return &UserRepo{pool: pool} }

func (r *UserRepo) Authenticate(ctx context.Context, username, password string) (string, bool, error) {
	var id *string
	if err := r.pool.QueryRow(ctx, `SELECT fn_login($1, $2)::text`, username, password).Scan(&id); err != nil {
		return "", false, err
	}
	if id == nil {
		return "", false, nil
	}
	return *id, true, nil
}

func (r *UserRepo) GetByID(ctx context.Context, id string) (domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx, `SELECT id::text, username FROM users WHERE id = $1`, id).Scan(&u.ID, &u.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, application.ErrNotFound
	}
	return u, err
}
