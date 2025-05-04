package app

import (
	"github.com/glacius-labs/captain-compose/internal/app/deployment/create"
	"github.com/glacius-labs/captain-compose/internal/app/deployment/remove"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
)

type DeploymentApp struct {
	Create *create.Handler
	Remove *remove.Handler
}

type App struct {
	Deployment DeploymentApp
}

func New(runtime deployment.Runtime, publisher deployment.Publisher) *App {
	return &App{
		Deployment: DeploymentApp{
			Create: create.NewHandler(runtime, publisher),
			Remove: remove.NewHandler(runtime, publisher),
		},
	}
}
