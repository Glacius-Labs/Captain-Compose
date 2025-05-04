package remove

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
)

type Handler struct {
	runtime   deployment.Runtime
	publisher deployment.Publisher
}

func NewHandler(runtime deployment.Runtime, publisher deployment.Publisher) *Handler {
	return &Handler{runtime: runtime, publisher: publisher}
}

func (h *Handler) Handle(ctx context.Context, cmd Command) error {
	if err := h.runtime.Remove(ctx, cmd.Name); err != nil {
		event := deployment.NewRemovalFailedEvent(cmd.Name, err)

		_ = h.publisher.Publish(ctx, event)
		return err
	}

	event := deployment.NewRemovedEvent(cmd.Name)

	return h.publisher.Publish(ctx, event)
}
