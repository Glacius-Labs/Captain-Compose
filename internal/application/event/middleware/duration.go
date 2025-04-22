package middleware

import (
	"context"
	"time"

	applicationEvent "github.com/glacius-labs/captain-compose/internal/application/event"
	"github.com/glacius-labs/captain-compose/internal/core/event"
)

func Duration(log func(event event.Event, duration time.Duration)) applicationEvent.Middleware {
	return func(next applicationEvent.Handler) applicationEvent.Handler {
		return applicationEvent.HandlerFunc(func(ctx context.Context, event event.Event) error {
			start := time.Now()
			err := next.Handle(ctx, event)
			log(event, time.Since(start))
			return err
		})
	}
}
