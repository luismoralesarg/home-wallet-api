package http

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type categoryDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
}

func toCategoryDTO(c domain.Category) categoryDTO {
	return categoryDTO{ID: c.ID, Name: c.Name, Kind: string(c.Kind), CreatedAt: c.CreatedAt}
}

type categoryHandler struct{ svc *application.CategoryService }

func (h *categoryHandler) list(c *fiber.Ctx) error {
	var kind *domain.Kind
	if k := c.Query("kind"); k != "" {
		kk := domain.Kind(k)
		kind = &kk
	}
	cats, err := h.svc.List(c.Context(), kind)
	if err != nil {
		return err
	}
	out := make([]categoryDTO, len(cats))
	for i, cat := range cats {
		out[i] = toCategoryDTO(cat)
	}
	return c.JSON(out)
}

func (h *categoryHandler) get(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	cat, err := h.svc.Get(c.Context(), id)
	if err != nil {
		return err
	}
	return c.JSON(toCategoryDTO(cat))
}

func (h *categoryHandler) create(c *fiber.Ctx) error {
	var req struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	if err := c.BodyParser(&req); err != nil {
		return newError(fiber.StatusBadRequest, "invalid_body", "Body inválido")
	}
	cat, err := h.svc.Create(c.Context(), req.Name, domain.Kind(req.Kind))
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(toCategoryDTO(cat))
}

func (h *categoryHandler) update(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	var req struct {
		Name *string `json:"name"`
	}
	if err := c.BodyParser(&req); err != nil {
		return newError(fiber.StatusBadRequest, "invalid_body", "Body inválido")
	}
	cat, err := h.svc.Update(c.Context(), id, req.Name)
	if err != nil {
		return err
	}
	return c.JSON(toCategoryDTO(cat))
}

func (h *categoryHandler) delete(c *fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Context(), id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
