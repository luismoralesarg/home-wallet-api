package http

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/luismoralesarg/home-wallet-api/internal/application"
)

// ErrorBody es el schema Error de openapi.yaml.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string { return e.Message }

func newError(status int, code, msg string) *apiError {
	return &apiError{Status: status, Code: code, Message: msg}
}

func errorHandler(c *fiber.Ctx, err error) error {
	var ae *apiError
	var fe *fiber.Error
	var ve *application.ValidationError
	switch {
	case errors.As(err, &ae):
	case errors.As(err, &ve):
		ae = newError(fiber.StatusBadRequest, "validation_error", ve.Msg)
	case errors.Is(err, application.ErrConflict):
		ae = newError(fiber.StatusConflict, "conflict", "La operación entra en conflicto con datos existentes")
	case errors.Is(err, application.ErrNoExchangeRate):
		ae = newError(fiber.StatusServiceUnavailable, "exchange_rate_unavailable", "No hay cotización disponible")
	case errors.Is(err, application.ErrInvalidCredentials):
		ae = newError(fiber.StatusUnauthorized, "invalid_credentials", "Usuario o contraseña incorrectos")
	case errors.Is(err, application.ErrNotFound):
		ae = newError(fiber.StatusNotFound, "not_found", "No encontrado")
	case errors.As(err, &fe):
		ae = newError(fe.Code, "http_error", fe.Message)
	default:
		slog.Error("error interno", "err", err, "path", c.Path())
		ae = newError(fiber.StatusInternalServerError, "internal_error", "Error interno")
	}
	return c.Status(ae.Status).JSON(ErrorBody{Code: ae.Code, Message: ae.Message})
}
