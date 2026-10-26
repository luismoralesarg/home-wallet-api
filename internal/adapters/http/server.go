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
	Categories   *application.CategoryService
	Transactions *application.TransactionService
	Cards        *application.CardService
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

	cats := &categoryHandler{svc: d.Categories}
	protected.Get("/categories", cats.list)
	protected.Post("/categories", cats.create)
	protected.Get("/categories/:id", cats.get)
	protected.Patch("/categories/:id", cats.update)
	protected.Delete("/categories/:id", cats.delete)

	txs := &transactionHandler{svc: d.Transactions}
	protected.Get("/transactions", txs.list)
	protected.Post("/transactions", txs.create)
	protected.Get("/transactions/:id", txs.get)
	protected.Patch("/transactions/:id", txs.update)
	protected.Delete("/transactions/:id", txs.delete)

	cards := &cardHandler{svc: d.Cards}
	protected.Get("/cards", cards.list)
	protected.Post("/cards", cards.create)
	protected.Get("/cards/:id", cards.get)
	protected.Patch("/cards/:id", cards.update)
	protected.Delete("/cards/:id", cards.delete)
	protected.Post("/cards/:id/purchases", cards.createPurchase)
	protected.Get("/cards/:id/purchases/:purchaseId", cards.getPurchase)
	protected.Patch("/cards/:id/purchases/:purchaseId", cards.updatePurchase)
	protected.Delete("/cards/:id/purchases/:purchaseId", cards.deletePurchase)
	protected.Get("/cards/:id/statements/:month", cards.statement)
	protected.Put("/cards/:id/statements/:month/adjustment", cards.setAdjustment)
	protected.Post("/cards/:id/statements/:month/pay", cards.pay)
	protected.Get("/card-purchases", cards.listPurchases)
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
