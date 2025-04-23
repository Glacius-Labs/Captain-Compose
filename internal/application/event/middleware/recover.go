package middleware

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/glacius-labs/captain-compose/internal/application/event"
)

func Recover(logger *slog.Logger) func(event.HandlerFunc) event.HandlerFunc {
	return func(next event.HandlerFunc) event.HandlerFunc {
		return func(ctx context.Context, e event.Event) (err error) {
			defer func() {
				if r := recover(); r != nil {
					var panicErr error
					switch v := r.(type) {
					case error:
						panicErr = fmt.Errorf("panic recovered in event handler: %w", v)
					default:
						panicErr = fmt.Errorf("panic recovered in event handler: %v", v)
					}
					logger.Error("Recovered from panic in handler",
						"event_id", e.Identifier().String(),
						"event_type", e.Type(),
						"error", panicErr,
					)
				}
			}()
			return next(ctx, e)
		}
	}
}
