package event

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/core/event"
)

type HandlerFunc func(ctx context.Context, event event.Event) error

func (f HandlerFunc) Handle(ctx context.Context, event event.Event) error {
	return f(ctx, event)
}
