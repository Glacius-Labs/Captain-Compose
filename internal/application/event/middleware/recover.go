package middleware

import (
	"context"
	"fmt"

	applicationEvent "github.com/glacius-labs/captain-compose/internal/application/event"
	"github.com/glacius-labs/captain-compose/internal/core/event"
)

func Recover(log func(event event.Event, err error)) applicationEvent.Middleware {
	return func(next applicationEvent.Handler) applicationEvent.Handler {
		return applicationEvent.HandlerFunc(func(ctx context.Context, event event.Event) error {
			defer func() {
				if r := recover(); r != nil {
					log(event, fmt.Errorf("panic in event handler: %v", r))
				}
			}()
			return next.Handle(ctx, event)
		})
	}
}
