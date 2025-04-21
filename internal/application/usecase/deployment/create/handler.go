package createdeployment

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/application/command"
	"github.com/glacius-labs/captain-compose/internal/core/event"
	"github.com/glacius-labs/captain-compose/internal/core/runtime"
)

type handler struct {
	runtime    runtime.Runtime
	dispatcher event.Dispatcher
}

func NewHandler(runtime runtime.Runtime, dispatcher event.Dispatcher) *handler {
	return &handler{runtime: runtime, dispatcher: dispatcher}
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
		h.dispatcher.Dispatch(ctx, event.NewDeploymentFailedEvent(createCmd.Deployment.Name, err))
		return err
	}

	h.dispatcher.Dispatch(ctx, event.NewDeploymentCreatedEvent(createCmd.Deployment))

	return nil
}
