package event

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type compositePublisher struct {
	publishers []Publisher
}

func NewCompositePublisher(publishers ...Publisher) *compositePublisher {
	if len(publishers) == 0 {
		panic("at least one publisher must be provided")
	}

	for _, publisher := range publishers {
		if publisher == nil {
			panic("nil publisher provided")
		}
	}

	return &compositePublisher{
		publishers: publishers,
	}
}

func (p *compositePublisher) Name() string {
	names := make([]string, len(p.publishers))
	for i, publisher := range p.publishers {
		names[i] = publisher.Name()
	}
	return fmt.Sprintf("CompositePublisher (%s)", strings.Join(names, ", "))
}

func (p *compositePublisher) Publish(ctx context.Context, event Event) error {
	var errs []error

	for _, publisher := range p.publishers {
		if err := publisher.Publish(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("composite publisher encountered %d error(s): %w", len(errs), errors.Join(errs...))
	}

	return nil
}
