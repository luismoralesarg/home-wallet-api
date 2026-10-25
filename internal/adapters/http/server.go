package http

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/luismoralesarg/home-wallet-api/internal/application"
)

const userIDKey = "userID"

type Deps struct {
	Auth         *application.AuthService
	ExchangeRate *application.ExchangeRateService
}

func NewApp(d Deps) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler:          errorHandler,
		DisableStartupMessage: true,
	})
	app.Use(recover.New())

	app.Get("/healthz", func(c *fiber.Ctx) error { return c.SendString("ok") })

	h := &authHandler{svc: d.Auth}
	app.Post("/auth/login", h.login)

	protected := app.Group("", jwtMiddleware(d.Auth))
	protected.Get("/me", h.me)

	fx := &exchangeRateHandler{svc: d.ExchangeRate}
	protected.Get("/exchange-rate/today", fx.today)
	return app
}

func jwtMiddleware(auth *application.AuthService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get(fiber.HeaderAuthorization)
		token, found := strings.CutPrefix(header, "Bearer ")
		if !found || token == "" {
			return newError(fiber.StatusUnauthorized, "unauthorized", "Falta el token")
		}
		id, err := auth.VerifyToken(token)
		if err != nil {
			return newError(fiber.StatusUnauthorized, "unauthorized", "Token inválido o vencido")
		}
		c.Locals(userIDKey, id)
		return c.Next()
	}
}

func currentUserID(c *fiber.Ctx) string { return c.Locals(userIDKey).(string) }
