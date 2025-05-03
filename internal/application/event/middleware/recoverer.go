package middleware

import (
	"context"
	"fmt"

	"github.com/glacius-labs/captain-compose/internal/application/event"
)

type recoverer struct {
	next event.Publisher
}

func NewRecoverer(next event.Publisher) *recoverer {
	if next == nil {
		panic("next cannot be nil")
	}
	return &recoverer{next: next}
}

func (r *recoverer) Name() string {
	return r.next.Name()
}

func (r *recoverer) Publish(ctx context.Context, e event.Event) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			switch v := rec.(type) {
			case error:
				err = fmt.Errorf("panic recovered in publisher: %w", v)
			default:
				err = fmt.Errorf("panic recovered in publisher: %v", v)
			}
		}
	}()
	return r.next.Publish(ctx, e)
}
