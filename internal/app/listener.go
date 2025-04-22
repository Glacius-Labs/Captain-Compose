package app

import "context"

type Listener interface {
	Start(ctx context.Context)
}
