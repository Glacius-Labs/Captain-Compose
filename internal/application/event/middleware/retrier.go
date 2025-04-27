package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/glacius-labs/captain-compose/internal/application/event"
)

type retrier struct {
	next    event.Publisher
	retries int
	delay   time.Duration
	logger  *slog.Logger
}

func NewRetrier(next event.Publisher, retries int, delay time.Duration, logger *slog.Logger) *retrier {
	if next == nil {
		panic("next cannot be nil")
	}

	if logger == nil {
		panic("logger cannot be nil")
	}

	return &retrier{
		next:    next,
		retries: retries,
		delay:   delay,
	}
}

func (r *retrier) Name() string {
	return r.next.Name()
}

func (r *retrier) Publish(ctx context.Context, e event.Event) error {
	var err error
	for attempt := 0; attempt <= r.retries; attempt++ {
		err = r.next.Publish(ctx, e)
		if err == nil {
			return nil
		}

		r.logger.Error("Publish failed, retrying",
			"publisher", r.Name(),
			"event_id", e.Identifier(),
			"event_type", e.Type(),
			"attempt", attempt+1,
			"error", err,
		)

		if r.delay > 0 && attempt < r.retries {
			select {
			case <-time.After(r.delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return err
}
