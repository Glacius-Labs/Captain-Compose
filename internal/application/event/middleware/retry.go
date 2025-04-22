package middleware

import (
	"context"
	"time"

	applicationEvent "github.com/glacius-labs/captain-compose/internal/application/event"
	"github.com/glacius-labs/captain-compose/internal/core/event"
)

func Retry(retries int, delay time.Duration) applicationEvent.Middleware {
	return func(next applicationEvent.Handler) applicationEvent.Handler {
		return applicationEvent.HandlerFunc(func(ctx context.Context, event event.Event) error {
			var err error
			for range retries {
				err = next.Handle(ctx, event)
				if err == nil {
					return nil
				}
				if delay > 0 {
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
