package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/glacius-labs/captain-compose/internal/application/event"
)

type durationLogger struct {
	next   event.Publisher
	logger *slog.Logger
}

func NewDurationLogger(next event.Publisher, logger *slog.Logger) *durationLogger {
	if next == nil {
		panic("next cannot be nil")
	}

	if logger == nil {
		panic("logger cannot be nil")
	}

	return &durationLogger{
		next:   next,
		logger: logger,
	}
}

func (d *durationLogger) Name() string {
	return d.next.Name()
}

func (d *durationLogger) Publish(ctx context.Context, e event.Event) error {
	start := time.Now()

	defer func() {
		d.logger.Debug("publish duration",
			"publisher", d.Name(),
			"event_id", e.Identifier(),
			"event_type", e.Type(),
			"duration", time.Since(start),
		)
	}()

	return d.next.Publish(ctx, e)
}
