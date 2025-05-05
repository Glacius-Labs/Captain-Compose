package create

import (
	"context"
	"errors"

	"github.com/glacius-labs/captain-compose/internal/app/deployment/shared"
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
	depl := deployment.Deployment{Name: cmd.Name}

	if err := h.runtime.Deploy(ctx, depl, cmd.Payload); err != nil {
		event := deployment.NewCreationFailedEvent(cmd.Name, err)

		if pubErr := h.publisher.Publish(ctx, event); pubErr != nil {
			wrapped := shared.NewPublishEventFailed(pubErr)
			return NewDeploymentFailed(errors.Join(err, wrapped))
		}

		return NewDeploymentFailed(err)
	}

	event := deployment.NewCreatedEvent(cmd.Name)
	if err := h.publisher.Publish(ctx, event); err != nil {
		return shared.NewPublishEventFailed(err)
	}

	return nil
}
