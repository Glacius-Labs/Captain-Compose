package middleware

import (
	"context"
	"fmt"

	"github.com/glacius-labs/captain-compose/internal/application/event"
)

func Recover(log func(event event.Event, err error)) event.Middleware {
	return func(next event.Handler) event.Handler {
		return event.HandlerFunc(func(ctx context.Context, event event.Event) error {
			defer func() {
				if r := recover(); r != nil {
					log(event, fmt.Errorf("panic in event handler: %v", r))
				}
			}()
			return next.Handle(ctx, event)
		})
	}
}
