package middleware

import (
	"context"

	applicationEvent "github.com/glacius-labs/captain-compose/internal/application/event"
	"github.com/glacius-labs/captain-compose/internal/core/event"
)

func Logging(log func(event event.Event, err error)) applicationEvent.Middleware {
	return func(next applicationEvent.Handler) applicationEvent.Handler {
		return applicationEvent.HandlerFunc(func(ctx context.Context, event event.Event) error {
			err := next.Handle(ctx, event)
			log(event, err)
			return err
		})
	}
}
