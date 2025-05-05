package shared

import "fmt"

type PublishEventFailed struct {
	Err error
}

func NewPublishEventFailed(err error) error {
	return PublishEventFailed{Err: err}
}

func (e PublishEventFailed) Error() string {
	return fmt.Sprintf("failed to publish event: %v", e.Err)
}

func (e PublishEventFailed) Unwrap() error {
	return e.Err
}
