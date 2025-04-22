package event

import (
	"context"
	"fmt"
	"time"

	"github.com/glacius-labs/captain-compose/internal/core/event"
)

func RecoverMiddleware(log func(event event.Event, err error)) Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, event event.Event) error {
			defer func() {
				if r := recover(); r != nil {
					log(event, fmt.Errorf("panic in event handler: %v", r))
				}
			}()
			return next.Handle(ctx, event)
		})
	}
}

func LoggingMiddleware(log func(event event.Event, err error)) Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, event event.Event) error {
			err := next.Handle(ctx, event)
			log(event, err)
			return err
		})
	}
}

func DurationMiddleware(log func(event event.Event, duration time.Duration)) Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, event event.Event) error {
			start := time.Now()
			err := next.Handle(ctx, event)
			log(event, time.Since(start))
			return err
		})
	}
}
