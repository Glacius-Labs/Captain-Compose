package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/glacius-labs/captain-compose/internal/application/event"
)

func Duration(logger *slog.Logger) func(event.HandlerFunc) event.HandlerFunc {
	return func(next event.HandlerFunc) event.HandlerFunc {
		return func(ctx context.Context, e event.Event) error {
			start := time.Now()
			defer func() {
				logger.Debug("event duration",
					"event_id", e.Identifier(),
					"event_type", e.Type(),
					"duration", time.Since(start),
				)
			}()
			return next(ctx, e)
		}
	}
}
