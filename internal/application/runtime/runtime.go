package runtime

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/core/deployment"
)

type Runtime interface {
	Deploy(ctx context.Context, deployment deployment.Deployment) error
	Remove(ctx context.Context, name string) error
}
