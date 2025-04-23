package middleware

import (
	"context"
	"time"

	"github.com/glacius-labs/captain-compose/internal/application/event"
)

func Retry(retries int, delay time.Duration) func(event.HandlerFunc) event.HandlerFunc {
	return func(next event.HandlerFunc) event.HandlerFunc {
		return func(ctx context.Context, e event.Event) error {
			var err error
			for attempt := 0; attempt <= retries; attempt++ {
				err = next(ctx, e)
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
		}
	}
}
