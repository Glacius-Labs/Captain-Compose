package event

import (
	"context"
)

type Publisher interface {
	Name() string
	Publish(ctx context.Context, event Event) error
}
