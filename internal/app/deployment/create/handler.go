package create

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
	d := deployment.Deployment{Name: cmd.Name}

	if err := h.runtime.Deploy(ctx, d, cmd.Payload); err != nil {
		event := deployment.NewCreationFailedEvent(cmd.Name, err)

		_ = h.publisher.Publish(ctx, event)
		return err
	}

	event := deployment.NewCreatedEvent(cmd.Name)

	return h.publisher.Publish(ctx, event)
}
