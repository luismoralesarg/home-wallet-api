package http

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/luismoralesarg/home-wallet-api/internal/application"
)

type exchangeRateDTO struct {
	Date         string    `json:"date"`
	Currency     string    `json:"currency"`
	OfficialBuy  string    `json:"official_buy"`
	OfficialSell string    `json:"official_sell"`
	FetchedAt    time.Time `json:"fetched_at"`
}

type exchangeRateHandler struct {
	svc *application.ExchangeRateService
}

func (h *exchangeRateHandler) today(c *fiber.Ctx) error {
	r, err := h.svc.Today(c.Context())
	if err != nil {
		if err == application.ErrNoExchangeRate {
			return newError(fiber.StatusServiceUnavailable, "exchange_rate_unavailable", "No hay cotización disponible")
		}
		return err
	}
	return c.JSON(exchangeRateDTO{
		Date:         r.Date.Format("2006-01-02"),
		Currency:     "USD",
		OfficialBuy:  r.Buy.StringFixed(2),
		OfficialSell: r.Sell.StringFixed(2),
		FetchedAt:    r.FetchedAt,
	})
}
