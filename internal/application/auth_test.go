package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luismoralesarg/home-wallet-api/internal/adapters/jwt"
	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type fakeUsers struct {
	id, pass string
	authErr  error
}

func (f fakeUsers) Authenticate(_ context.Context, username, password string) (string, bool, error) {
	if f.authErr != nil {
		return "", false, f.authErr
	}
	if username == "luis" && password == f.pass {
		return f.id, true, nil
	}
	return "", false, nil
}

func (f fakeUsers) GetByID(_ context.Context, id string) (domain.User, error) {
	if id != f.id {
		return domain.User{}, application.ErrNotFound
	}
	return domain.User{ID: id, Username: "luis"}, nil
}

func newSvc(u fakeUsers) *application.AuthService {
	return application.NewAuthService(u, jwt.New("0123456789abcdef-secret", time.Hour))
}

func TestLogin(t *testing.T) {
	svc := newSvc(fakeUsers{id: "u-1", pass: "ok"})

	t.Run("credenciales válidas emiten un token que identifica al usuario", func(t *testing.T) {
		res, err := svc.Login(context.Background(), "luis", "ok")
		if err != nil {
			t.Fatal(err)
		}
		if res.User.Username != "luis" || res.Token == "" || !res.ExpiresAt.After(time.Now()) {
			t.Fatalf("resultado inesperado: %+v", res)
		}
		id, err := svc.VerifyToken(res.Token)
		if err != nil || id != "u-1" {
			t.Fatalf("VerifyToken = %q, %v", id, err)
		}
	})

	for name, tc := range map[string][2]string{
		"password incorrecta": {"luis", "mal"},
		"usuario inexistente": {"otro", "ok"},
		"campos vacíos":       {"", ""},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.Login(context.Background(), tc[0], tc[1]); !errors.Is(err, application.ErrInvalidCredentials) {
				t.Fatalf("err = %v, quería ErrInvalidCredentials", err)
			}
		})
	}

	t.Run("error del repo se propaga", func(t *testing.T) {
		boom := errors.New("db caída")
		_, err := newSvc(fakeUsers{authErr: boom}).Login(context.Background(), "luis", "ok")
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestVerifyToken(t *testing.T) {
	svc := newSvc(fakeUsers{id: "u-1", pass: "ok"})
	if _, err := svc.VerifyToken("basura"); !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatalf("token basura: %v", err)
	}
	other := application.NewAuthService(fakeUsers{id: "u-1", pass: "ok"}, jwt.New("otro-secret-distinto-123", time.Hour))
	res, _ := other.Login(context.Background(), "luis", "ok")
	if _, err := svc.VerifyToken(res.Token); !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatalf("token firmado con otro secret debería fallar: %v", err)
	}
	expired := application.NewAuthService(fakeUsers{id: "u-1", pass: "ok"}, jwt.New("0123456789abcdef-secret", -time.Minute))
	res, _ = expired.Login(context.Background(), "luis", "ok")
	if _, err := svc.VerifyToken(res.Token); !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatalf("token vencido debería fallar: %v", err)
	}
}
