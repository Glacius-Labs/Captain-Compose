package commandhandler

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/application/command"
	"github.com/glacius-labs/captain-compose/internal/core/event"
	"github.com/glacius-labs/captain-compose/internal/core/runtime"
)

type RemoveDeploymentHandler struct {
	runtime   runtime.Runtime
	publisher event.Publisher
}

func NewRemoveDeploymentHandler(runtime runtime.Runtime, publisher event.Publisher) *RemoveDeploymentHandler {
	return &RemoveDeploymentHandler{runtime: runtime, publisher: publisher}
}

func (h *RemoveDeploymentHandler) Handle(ctx context.Context, cmd command.RemoveDeploymentCommand) error {
	if err := h.runtime.Remove(ctx, cmd.Name); err != nil {
		if pubErr := h.publisher.Publish(ctx, event.NewDeploymentFailedEvent(cmd.Name, err)); pubErr != nil {
			// Ignoring error as it is unlikely with in-memory publishing.
		}
		return err
	}

	if err := h.publisher.Publish(ctx, event.NewDeploymentRemovedEvent(cmd.Name)); err != nil {
		return err
	}

	return nil
}
