package middleware

import (
	"context"
	"fmt"

	"github.com/glacius-labs/captain-compose/internal/application/event"
)

func Recover(log func(event event.Event, err error)) event.Middleware {
	return func(next event.Handler) event.Handler {
		return event.HandlerFunc(func(ctx context.Context, event event.Event) (err error) {
			defer func() {
				if r := recover(); r != nil {
					var panicErr error
					switch v := r.(type) {
					case error:
						panicErr = fmt.Errorf("panic recovered in event handler: %w", v)
					default:
						panicErr = fmt.Errorf("panic recovered in event handler: %v", v)
					}
					log(event, panicErr)
				}
			}()
			return next.Handle(ctx, event)
		})
	}
}
