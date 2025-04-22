package deployment

import (
	"context"
)

type Runtime interface {
	List(ctx context.Context) ([]Deployment, error)
	Get(ctx context.Context, name string) (Deployment, error)
	Deploy(ctx context.Context, deployment Deployment) error
	Remove(ctx context.Context, name string) error
}
