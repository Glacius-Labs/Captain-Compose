package middleware

import (
	"context"
	"time"

	"github.com/glacius-labs/captain-compose/internal/application/event"
)

func Retry(retries int, delay time.Duration) event.Middleware {
	return func(next event.Handler) event.Handler {
		return event.HandlerFunc(func(ctx context.Context, event event.Event) error {
			var err error
			for attempt := 0; attempt <= retries; attempt++ {
				err = next.Handle(ctx, event)
				if err == nil {
					return nil
				}

				if delay > 0 && attempt < retries {
					select {
					case <-time.After(delay):
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}
			return err
		})
	}
}
