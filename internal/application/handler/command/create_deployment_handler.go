package commandhandler

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/application/command"
	"github.com/glacius-labs/captain-compose/internal/core/event"
	"github.com/glacius-labs/captain-compose/internal/core/runtime"
)

type CreateDeploymentHandler struct {
	runtime   runtime.Runtime
	publisher event.Publisher
}

func NewCreateDeploymentHandler(runtime runtime.Runtime, publisher event.Publisher) *CreateDeploymentHandler {
	return &CreateDeploymentHandler{runtime: runtime, publisher: publisher}
}

func (h *CreateDeploymentHandler) Handle(ctx context.Context, cmd command.CreateDeploymentCommand) error {
	if err := h.runtime.Deploy(ctx, cmd.Deployment); err != nil {
		if pubErr := h.publisher.Publish(ctx, event.NewDeploymentFailedEvent(cmd.Deployment.Name, err)); pubErr != nil {
			// log the error if publishing fails
		}
		return err
	}

	if err := h.publisher.Publish(ctx, event.NewDeploymentCreatedEvent(cmd.Deployment.Name)); err != nil {
		return err
	}

	return nil
}
