package domain

import "time"

type Kind string

const (
	Income  Kind = "income"
	Expense Kind = "expense"
)

func (k Kind) Valid() bool { return k == Income || k == Expense }

type Category struct {
	ID        string
	Name      string
	Kind      Kind
	CreatedAt time.Time
}
