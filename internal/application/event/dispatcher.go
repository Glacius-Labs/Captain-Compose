package event

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/core/event"
)

type Dispatcher interface {
	Dispatch(ctx context.Context, event event.Event)
}
