package application

import (
	"context"
	"errors"
	"time"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
	"github.com/luismoralesarg/home-wallet-api/internal/ports"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrNotFound           = errors.New("not found")
)

type AuthService struct {
	users  ports.UserRepository
	tokens ports.TokenIssuer
}

func NewAuthService(users ports.UserRepository, tokens ports.TokenIssuer) *AuthService {
	return &AuthService{users: users, tokens: tokens}
}

type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	User      domain.User
}

func (s *AuthService) Login(ctx context.Context, username, password string) (LoginResult, error) {
	if username == "" || password == "" {
		return LoginResult{}, ErrInvalidCredentials
	}
	id, ok, err := s.users.Authenticate(ctx, username, password)
	if err != nil {
		return LoginResult{}, err
	}
	if !ok {
		return LoginResult{}, ErrInvalidCredentials
	}
	user, err := s.users.GetByID(ctx, id)
	if err != nil {
		return LoginResult{}, err
	}
	token, exp, err := s.tokens.Issue(id)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: token, ExpiresAt: exp, User: user}, nil
}

func (s *AuthService) Me(ctx context.Context, userID string) (domain.User, error) {
	return s.users.GetByID(ctx, userID)
}

// VerifyToken devuelve el id de usuario del token o ErrInvalidCredentials.
func (s *AuthService) VerifyToken(token string) (string, error) {
	id, err := s.tokens.Parse(token)
	if err != nil {
		return "", ErrInvalidCredentials
	}
	return id, nil
}
