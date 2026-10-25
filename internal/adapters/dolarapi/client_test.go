package dolarapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luismoralesarg/home-wallet-api/internal/adapters/dolarapi"
)

func serve(t *testing.T, status int, body string) *dolarapi.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/dolares/oficial" {
			t.Errorf("path inesperado: %s", r.URL.Path)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return dolarapi.New(srv.URL)
}

func TestFetchOfficial(t *testing.T) {
	c := serve(t, 200, `{"moneda":"USD","casa":"oficial","compra":1385.5,"venta":1435,"fechaActualizacion":"2026-10-15T15:00:00.000Z"}`)
	buy, sell, err := c.FetchOfficial(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if buy.StringFixed(2) != "1385.50" || sell.StringFixed(2) != "1435.00" {
		t.Fatalf("buy=%s sell=%s", buy, sell)
	}
}

func TestFetchOfficialErrors(t *testing.T) {
	for name, c := range map[string]*dolarapi.Client{
		"status 500":      serve(t, 500, `{}`),
		"json inválido":   serve(t, 200, `no es json`),
		"sin venta":       serve(t, 200, `{"compra":1000}`),
		"venta en cero":   serve(t, 200, `{"compra":1000,"venta":0}`),
		"venta no número": serve(t, 200, `{"compra":1000,"venta":"abc"}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := c.FetchOfficial(context.Background()); err == nil {
				t.Fatal("esperaba error")
			}
		})
	}
}
