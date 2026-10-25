package http_test

import (
	"context"
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/luismoralesarg/home-wallet-api/internal/adapters/http"
	"github.com/luismoralesarg/home-wallet-api/internal/adapters/jwt"
	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type users struct{}

func (users) Authenticate(_ context.Context, u, p string) (string, bool, error) {
	return "u-1", u == "luis" && p == "ok", nil
}
func (users) GetByID(context.Context, string) (domain.User, error) {
	return domain.User{ID: "u-1", Username: "luis"}, nil
}

func do(t *testing.T, method, path, body, token string) (int, map[string]any) {
	t.Helper()
	svc := application.NewAuthService(users{}, jwt.New("0123456789abcdef-secret", time.Hour))
	app := http.NewApp(http.Deps{Auth: svc})
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestLoginAndMe(t *testing.T) {
	status, body := do(t, "POST", "/auth/login", `{"username":"luis","password":"ok"}`, "")
	if status != nethttp.StatusOK {
		t.Fatalf("login status %d: %v", status, body)
	}
	token := body["token"].(string)

	status, body = do(t, "GET", "/me", "", token)
	if status != 200 || body["username"] != "luis" {
		t.Fatalf("/me: %d %v", status, body)
	}
}

func TestErrorsUseCodeMessageSchema(t *testing.T) {
	cases := []struct {
		name, method, path, body, token string
		status                          int
		code                            string
	}{
		{"login inválido", "POST", "/auth/login", `{"username":"luis","password":"mal"}`, "", 401, "invalid_credentials"},
		{"body roto", "POST", "/auth/login", `{`, "", 400, "invalid_body"},
		{"sin token", "GET", "/me", "", "", 401, "unauthorized"},
		{"token inválido", "GET", "/me", "", "x.y.z", 401, "unauthorized"},
		{"ruta inexistente sin token", "GET", "/nope", "", "", 401, "unauthorized"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := do(t, tc.method, tc.path, tc.body, tc.token)
			if status != tc.status || body["code"] != tc.code || body["message"] == "" {
				t.Fatalf("got %d %v", status, body)
			}
		})
	}
}

func TestUnknownRouteWithTokenIs404(t *testing.T) {
	_, body := do(t, "POST", "/auth/login", `{"username":"luis","password":"ok"}`, "")
	status, out := do(t, "GET", "/nope", "", body["token"].(string))
	if status != 404 || out["code"] != "http_error" {
		t.Fatalf("got %d %v", status, out)
	}
}
