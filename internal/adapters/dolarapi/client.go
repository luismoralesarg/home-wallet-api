// Package dolarapi consulta el dólar oficial en https://dolarapi.com.
package dolarapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
)

const DefaultBaseURL = "https://dolarapi.com"

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: 10 * time.Second}}
}

// json.Number evita pasar por float64: el valor va directo a decimal.
type response struct {
	Compra json.Number `json:"compra"`
	Venta  json.Number `json:"venta"`
}

func (c *Client) FetchOfficial(ctx context.Context) (buy, sell decimal.Decimal, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/dolares/oficial", nil)
	if err != nil {
		return buy, sell, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return buy, sell, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return buy, sell, fmt.Errorf("dolarapi respondió %d", resp.StatusCode)
	}
	var body response
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return buy, sell, fmt.Errorf("respuesta inválida: %w", err)
	}
	if buy, err = decimal.NewFromString(body.Compra.String()); err != nil {
		return buy, sell, fmt.Errorf("compra inválida: %w", err)
	}
	if sell, err = decimal.NewFromString(body.Venta.String()); err != nil {
		return buy, sell, fmt.Errorf("venta inválida: %w", err)
	}
	if !buy.IsPositive() || !sell.IsPositive() {
		return buy, sell, errors.New("cotización no positiva")
	}
	return buy.Round(2), sell.Round(2), nil
}
