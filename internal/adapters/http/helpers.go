package http

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
	"github.com/luismoralesarg/home-wallet-api/internal/tz"
)

// pathID devuelve el :id; un id que no es UUID no puede existir, así que es 404.
func pathID(c *fiber.Ctx) (string, error) {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return "", newError(fiber.StatusNotFound, "not_found", "No encontrado")
	}
	return id, nil
}

func queryUUID(c *fiber.Ctx, name string) (*string, error) {
	v := c.Query(name)
	if v == "" {
		return nil, nil
	}
	if _, err := uuid.Parse(v); err != nil {
		return nil, newError(fiber.StatusBadRequest, "validation_error", name+" inválido")
	}
	return &v, nil
}

// parseMonth interpreta YYYY-MM como el primer instante de ese mes en Mendoza.
func parseMonth(v string) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01", v, tz.Mendoza)
	if err != nil {
		return time.Time{}, newError(fiber.StatusBadRequest, "validation_error", "month debe tener formato YYYY-MM")
	}
	return t, nil
}

func parseMoney(field, v string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(v)
	if err != nil {
		return d, newError(fiber.StatusBadRequest, "validation_error", field+" debe ser un decimal en string, ej. \"1234.50\"")
	}
	return d, nil
}

func money(d decimal.Decimal) string { return d.StringFixed(2) }

type userRefDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func toUserRef(u domain.UserRef) userRefDTO { return userRefDTO{ID: u.ID, Name: u.Name} }
