package middleware

import (
	"context"
	"time"

	"github.com/glacius-labs/captain-compose/internal/application/event"
)

func Duration(log func(event event.Event, duration time.Duration)) event.Middleware {
	return func(next event.Handler) event.Handler {
		return event.HandlerFunc(func(ctx context.Context, event event.Event) error {
			start := time.Now()
			err := next.Handle(ctx, event)
			log(event, time.Since(start))
			return err
		})
	}
}
