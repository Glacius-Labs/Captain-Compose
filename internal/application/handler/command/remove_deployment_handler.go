package commandhandler

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/application/command"
	"github.com/glacius-labs/captain-compose/internal/core/event"
)

type RemoveDeploymentHandler struct {
	publisher event.Publisher
}

func NewRemoveDeploymentHandler(publisher event.Publisher) *RemoveDeploymentHandler {
	return &RemoveDeploymentHandler{publisher: publisher}
}

func (h *RemoveDeploymentHandler) Handle(ctx context.Context, cmd command.RemoveDeploymentCommand) error {
	// do something with the command, e.g., remove a deployment

	h.publisher.Publish(ctx, event.NewDeploymentRemovedEvent(cmd.Name))
	return nil
}
