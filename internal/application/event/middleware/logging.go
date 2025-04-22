package middleware

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/application/event"
)

func Logging(log func(event event.Event, err error)) event.Middleware {
	return func(next event.Handler) event.Handler {
		return event.HandlerFunc(func(ctx context.Context, event event.Event) error {
			err := next.Handle(ctx, event)
			log(event, err)
			return err
		})
	}
}
