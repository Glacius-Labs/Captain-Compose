package deployment

import "context"

type Runtime interface {
	List(ctx context.Context) ([]Deployment, error)
	Deploy(ctx context.Context, deployment Deployment, payload []byte) error
	Remove(ctx context.Context, name string) error
}

type Publisher interface {
	Publish(ctx context.Context, event Event) error
}
