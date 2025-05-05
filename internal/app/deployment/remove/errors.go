package remove

import "fmt"

type RemovalFailed struct {
	Err error
}

func NewRemovalFailed(err error) error {
	return RemovalFailed{Err: err}
}

func (e RemovalFailed) Error() string {
	return fmt.Sprintf("removal failed: %v", e.Err)
}

func (e RemovalFailed) Unwrap() error {
	return e.Err
}
