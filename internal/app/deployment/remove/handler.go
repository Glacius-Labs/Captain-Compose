package remove

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
	if err := h.runtime.Remove(ctx, cmd.Name); err != nil {
		event := deployment.NewRemovalFailedEvent(cmd.Name, err)

		if pubErr := h.publisher.Publish(ctx, event); pubErr != nil {
			wrapped := shared.NewPublishEventFailed(pubErr)
			return NewRemovalFailed(errors.Join(err, wrapped))
		}

		return NewRemovalFailed(err)
	}

	event := deployment.NewRemovedEvent(cmd.Name)
	if err := h.publisher.Publish(ctx, event); err != nil {
		return shared.NewPublishEventFailed(err)
	}

	return nil
}
