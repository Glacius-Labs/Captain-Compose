package event

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

type CompositePublisher struct {
	publishers []Publisher
	logger     *slog.Logger
}

func NewCompositePublisher(logger *slog.Logger, publishers ...Publisher) *CompositePublisher {
	if logger == nil {
		panic("logger cannot be nil")
	}

	if len(publishers) == 0 {
		panic("at least one publisher must be provided")
	}

	for _, publisher := range publishers {
		if publisher == nil {
			panic("nil publisher provided")
		}
	}

	return &CompositePublisher{
		publishers: publishers,
		logger:     logger,
	}
}

func (p *CompositePublisher) Name() string {
	names := make([]string, len(p.publishers))
	for i, publisher := range p.publishers {
		names[i] = publisher.Name()
	}
	return fmt.Sprintf("CompositePublisher (%s)", strings.Join(names, ", "))
}

func (p *CompositePublisher) Publish(ctx context.Context, event Event) error {
	var errs []error

	for _, publisher := range p.publishers {
		if err := publisher.Publish(ctx, event); err != nil {
			p.logger.Error("failed to publish event",
				"publisher", publisher.Name(),
				"event_id", event.Identifier(),
				"event_type", event.Type(),
				"error", err)
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("composite publisher encountered %d error(s): %w", len(errs), errors.Join(errs...))
	}

	return nil
}
