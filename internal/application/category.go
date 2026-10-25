package application

import (
	"context"
	"strings"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
	"github.com/luismoralesarg/home-wallet-api/internal/ports"
)

type CategoryService struct{ repo ports.CategoryRepository }

func NewCategoryService(repo ports.CategoryRepository) *CategoryService {
	return &CategoryService{repo: repo}
}

func (s *CategoryService) List(ctx context.Context, kind *domain.Kind) ([]domain.Category, error) {
	if kind != nil && !kind.Valid() {
		return nil, invalid("kind inválido")
	}
	return s.repo.List(ctx, kind)
}

func (s *CategoryService) Get(ctx context.Context, id string) (domain.Category, error) {
	return s.repo.Get(ctx, id)
}

func (s *CategoryService) Create(ctx context.Context, name string, kind domain.Kind) (domain.Category, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Category{}, invalid("el nombre es obligatorio")
	}
	if !kind.Valid() {
		return domain.Category{}, invalid("kind debe ser income o expense")
	}
	return s.repo.Create(ctx, domain.Category{Name: name, Kind: kind})
}

func (s *CategoryService) Update(ctx context.Context, id string, name *string) (domain.Category, error) {
	if name == nil {
		return s.repo.Get(ctx, id)
	}
	n := strings.TrimSpace(*name)
	if n == "" {
		return domain.Category{}, invalid("el nombre es obligatorio")
	}
	return s.repo.Update(ctx, id, n)
}

func (s *CategoryService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
