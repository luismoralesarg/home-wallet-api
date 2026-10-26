package application

import (
	"errors"
	"fmt"
)

var ErrConflict = errors.New("conflict")

// ValidationError es un error de entrada del usuario (HTTP 400).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(format string, a ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, a...)}
}
