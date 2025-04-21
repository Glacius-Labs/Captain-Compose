package command

import (
	"fmt"
)

type CommandTypeMismatchError struct {
	Expected Type
	Actual   Type
}

func (e CommandTypeMismatchError) Error() string {
	return fmt.Sprintf("command type mismatch: expected %q, got %q", e.Expected, e.Actual)
}

func NewCommandTypeMismatchError(expected, actual Type) error {
	return CommandTypeMismatchError{
		Expected: expected,
		Actual:   actual,
	}
}
