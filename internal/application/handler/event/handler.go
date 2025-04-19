package eventhandler

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/core/event"
)

type Handler interface {
	Handle(ctx context.Context, event event.Event) error
}
