package event

import (
	"context"
	"time"
)

func RetryMiddleware(retries int, delay time.Duration) Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, event Event) error {
			var err error
			for i := 0; i <= retries; i++ {
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
