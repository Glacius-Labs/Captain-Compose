package command

import "github.com/glacius-labs/captain-compose/internal/core/model"

type CreateDeploymentCommand struct {
	Deployment model.Deployment
}

func (CreateDeploymentCommand) IsCommand() {}
