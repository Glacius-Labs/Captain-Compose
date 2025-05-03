package command

import (
	"errors"
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

var (
	ErrPublishAfterSuccess = errors.New("event publish failed after successful execution")

	ErrExecutionAndPublish = errors.New("execution and event publish both failed")
)

func WrapExecutionAndPublishErrors(execErr, pubErr error) error {
	return errors.Join(
		fmt.Errorf("execution failed: %w", execErr),
		fmt.Errorf("failed to publish failure event: %w", pubErr),
	)
}

func WrapPublishAfterSuccess(pubErr error) error {
	return fmt.Errorf("%w: %v", ErrPublishAfterSuccess, pubErr)
}
