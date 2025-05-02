package mqtt

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/glacius-labs/captain-compose/internal/application/command"
	createdeployment "github.com/glacius-labs/captain-compose/internal/interface/mqtt/deployment/create"
	removedeployment "github.com/glacius-labs/captain-compose/internal/interface/mqtt/deployment/remove"
	"github.com/glacius-labs/captain-compose/internal/interface/mqtt/message"
)

type Listener struct {
	topic      string
	subscriber message.Subscriber
	decoders   []message.Decoder
	router     command.Router
	logger     *slog.Logger
}

func NewListener(
	topic string,
	subscriber message.Subscriber,
	router command.Router,
	logger *slog.Logger,
) *Listener {
	decoders := []message.Decoder{
		createdeployment.NewDecoder(),
		removedeployment.NewDecoder(),
	}

	return &Listener{
		topic:      topic,
		subscriber: subscriber,
		decoders:   decoders,
		router:     router,
		logger:     logger,
	}
}

func (l *Listener) Start(ctx context.Context) error {
	err := l.subscriber.Subscribe(ctx, l.topic, func(msgCtx context.Context, msg []byte) {
		env, err := message.DecodeEnvelope(msg)
		if err != nil {
			l.logger.Warn("Failed to decode envelope", "error", err)
			return
		}

		for _, decoder := range l.decoders {
			if decoder.CanDecode(env.Type) {
				cmd, err := decoder.Decode(env)
				if err != nil {
					l.logger.Warn("Failed to decode command", "error", err, "type", env.Type)
					return
				}

				if err := l.router.Dispatch(msgCtx, cmd); err != nil {
					l.logger.Error("Failed to dispatch command", "error", err, "type", cmd.Type())
				}
				return
			}

		}
	})

	if err != nil {
		return fmt.Errorf("subscription failed: %w", err)
	}

	<-ctx.Done()
	l.logger.Info("Listener shutting down")
	return nil
}
