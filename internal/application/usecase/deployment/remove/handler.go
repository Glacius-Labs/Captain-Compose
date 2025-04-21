package removedeployment

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
	removeCmd, ok := cmd.(*Command)
	if !ok {
		return command.NewCommandTypeMismatchError(h.CommandType(), cmd.Type())
	}

	if err := h.runtime.Remove(ctx, removeCmd.Name); err != nil {
		if pubErr := h.publisher.Publish(ctx, event.NewDeploymentFailedEvent(removeCmd.Name, err)); pubErr != nil {
			// Ignoring error as it is unlikely with in-memory publishing.
		}
		return err
	}

	if err := h.publisher.Publish(ctx, event.NewDeploymentRemovedEvent(removeCmd.Name)); err != nil {
		return err
	}

	return nil
}
