package commandhandler

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/application/command"
)

type RemoveDeploymentHandler struct {
}

func (h *RemoveDeploymentHandler) Handle(ctx context.Context, cmd command.RemoveDeploymentCommand) error {
	panic("implement me")
}
