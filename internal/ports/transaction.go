package ports

import (
	"context"
	"time"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type CategoryRepository interface {
	List(ctx context.Context, kind *domain.Kind) ([]domain.Category, error)
	Get(ctx context.Context, id string) (domain.Category, error)            // application.ErrNotFound
	Create(ctx context.Context, c domain.Category) (domain.Category, error) // application.ErrConflict si el nombre existe
	Update(ctx context.Context, id, name string) (domain.Category, error)
	Delete(ctx context.Context, id string) error // application.ErrConflict si está en uso
}

// TransactionFilter: From/To definen [From, To) sobre occurred_at.
type TransactionFilter struct {
	From, To   *time.Time
	Kind       *domain.Kind
	CategoryID *string
	UserID     *string
	Limit      int
	Offset     int
}

type TransactionRepository interface {
	List(ctx context.Context, f TransactionFilter) (items []domain.Transaction, total int, err error)
	Get(ctx context.Context, id string) (domain.Transaction, error)
	Create(ctx context.Context, t domain.Transaction) (domain.Transaction, error)
	Update(ctx context.Context, t domain.Transaction) (domain.Transaction, error)
	Delete(ctx context.Context, id string) error
}
