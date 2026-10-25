package http

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type userDTO struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

func toUserDTO(u domain.User) userDTO { return userDTO{ID: u.ID, Username: u.Username} }

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      userDTO   `json:"user"`
}

type authHandler struct{ svc *application.AuthService }

func (h *authHandler) login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return newError(fiber.StatusBadRequest, "invalid_body", "Body inválido")
	}
	res, err := h.svc.Login(c.Context(), req.Username, req.Password)
	if err != nil {
		return err
	}
	return c.JSON(loginResponse{Token: res.Token, ExpiresAt: res.ExpiresAt, User: toUserDTO(res.User)})
}

func (h *authHandler) me(c *fiber.Ctx) error {
	u, err := h.svc.Me(c.Context(), currentUserID(c))
	if err != nil {
		return err
	}
	return c.JSON(toUserDTO(u))
}
