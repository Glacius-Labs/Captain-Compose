package commandhandler

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/application/command"
	"github.com/glacius-labs/captain-compose/internal/core/event"
)

type CreateDeploymentHandler struct {
	publisher event.Publisher
}

func (h *CreateDeploymentHandler) Handle(ctx context.Context, cmd command.CreateDeploymentCommand) error {
	// do something with the command, e.g., create a deployment

	h.publisher.Publish(ctx, event.NewDeploymentCreatedEvent(cmd.Deployment.Name))
	return nil
}
