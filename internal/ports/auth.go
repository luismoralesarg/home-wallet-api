package ports

import (
	"context"
	"time"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type UserRepository interface {
	// Authenticate valida credenciales (fn_login). Devuelve ok=false si son inválidas.
	Authenticate(ctx context.Context, username, password string) (userID string, ok bool, err error)
	GetByID(ctx context.Context, id string) (domain.User, error)
}

type TokenIssuer interface {
	Issue(userID string) (token string, expiresAt time.Time, err error)
	Parse(token string) (userID string, err error)
}
