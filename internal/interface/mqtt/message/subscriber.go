package message

import "context"

type Subscriber interface {
	Subscribe(ctx context.Context, topic string, handler func(context.Context, []byte)) error
}
