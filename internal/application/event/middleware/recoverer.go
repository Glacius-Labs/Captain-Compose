package middleware

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/glacius-labs/captain-compose/internal/application/event"
)

type recoverer struct {
	next   event.Publisher
	logger *slog.Logger
}

func NewRecoverer(next event.Publisher, logger *slog.Logger) *recoverer {
	if next == nil {
		panic("next cannot be nil")
	}

	if logger == nil {
		panic("logger cannot be nil")
	}

	return &recoverer{
		next:   next,
		logger: logger,
	}
}

func (r *recoverer) Name() string {
	return r.next.Name()
}

func (r *recoverer) Publish(ctx context.Context, e event.Event) error {
	defer func(name string, logger *slog.Logger) {
		if r := recover(); r != nil {
			var panicErr error
			switch v := r.(type) {
			case error:
				panicErr = fmt.Errorf("panic recovered in publisher: %w", v)
			default:
				panicErr = fmt.Errorf("panic recovered in publisher: %v", v)
			}
			logger.Error("Recovered from panic in publisher",
				"publisher", name,
				"event_id", e.Identifier().String(),
				"event_type", e.Type(),
				"error", panicErr,
			)
		}
	}(r.Name(), r.logger)

	return r.next.Publish(ctx, e)
}
