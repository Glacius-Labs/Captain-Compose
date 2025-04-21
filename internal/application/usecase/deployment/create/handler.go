package createdeployment

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/application/command"
	"github.com/glacius-labs/captain-compose/internal/core/event"
	"github.com/glacius-labs/captain-compose/internal/core/runtime"
)

type handler struct {
	runtime   runtime.Runtime
	publisher event.Publisher
}

func NewHandler(runtime runtime.Runtime, publisher event.Publisher) *handler {
	return &handler{runtime: runtime, publisher: publisher}
}

func (h *handler) CommandType() command.Type {
	return CommandType
}

func (h *handler) Handle(ctx context.Context, cmd command.Command) error {
	createCmd, ok := cmd.(*Command)
	if !ok {
		return command.NewCommandTypeMismatchError(h.CommandType(), cmd.Type())
	}

	if err := h.runtime.Deploy(ctx, createCmd.Deployment); err != nil {
		if pubErr := h.publisher.Publish(ctx, event.NewDeploymentFailedEvent(createCmd.Deployment.Name, err)); pubErr != nil {
			// Ignoring error as it is unlikely with in-memory publishing.
		}
		return err
	}

	if err := h.publisher.Publish(ctx, event.NewDeploymentCreatedEvent(createCmd.Deployment.Name)); err != nil {
		return err
	}

	return nil
}
