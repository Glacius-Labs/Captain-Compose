package event

import (
	"context"
	"fmt"
	"time"
)

func RecoverMiddleware(log func(evt Event, err error)) Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, evt Event) error {
			defer func() {
				if r := recover(); r != nil {
					log(evt, fmt.Errorf("panic in event handler: %v", r))
				}
			}()
			return next.Handle(ctx, evt)
		})
	}
}

func LoggingMiddleware(log func(evt Event, err error)) Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, evt Event) error {
			err := next.Handle(ctx, evt)
			log(evt, err)
			return err
		})
	}
}

func DurationMiddleware(log func(evt Event, duration time.Duration)) Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, evt Event) error {
			start := time.Now()
			err := next.Handle(ctx, evt)
			log(evt, time.Since(start))
			return err
		})
	}
}
