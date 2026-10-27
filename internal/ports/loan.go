package ports

import (
	"context"

	"github.com/luismoralesarg/home-wallet-api/internal/domain"
)

type LoanRepository interface {
	// Create guarda el préstamo y todas sus cuotas en una transacción.
	Create(ctx context.Context, l domain.Loan, installments []domain.LoanInstallment) (domain.Loan, error)
	List(ctx context.Context) ([]domain.Loan, error)
	Get(ctx context.Context, id string) (domain.Loan, error) // application.ErrNotFound
	Rename(ctx context.Context, id, name string) (domain.Loan, error)
	Delete(ctx context.Context, id string) error
	Installments(ctx context.Context, loanID string) ([]domain.LoanInstallment, error) // ordenadas por mes
	// SaveInstallments actualiza (monto, monto en ARS, pagada) las cuotas indicadas, en una transacción.
	SaveInstallments(ctx context.Context, loanID string, installments []domain.LoanInstallment) error
}

type SubscriptionRepository interface {
	List(ctx context.Context) ([]domain.Subscription, error)
	Get(ctx context.Context, id string) (domain.Subscription, error)
	Create(ctx context.Context, s domain.Subscription) (domain.Subscription, error)
	Update(ctx context.Context, s domain.Subscription) (domain.Subscription, error)
	Delete(ctx context.Context, id string) error
}
