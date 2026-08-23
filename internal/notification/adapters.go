package notification

import (
	"context"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// JetAdapter adapts the platform JetStream client to the JetPublisher port.
// Nats-Msg-Id enables server-side deduplication (FR-NOT-05).
type JetAdapter struct {
	js jetstream.JetStream
}

func NewJetAdapter(js jetstream.JetStream) *JetAdapter {
	return &JetAdapter{js: js}
}

func (a *JetAdapter) Publish(ctx context.Context, subject string, payload []byte, msgID string) error {
	msg := nats.NewMsg(subject)
	msg.Data = payload
	msg.Header.Set(jetstream.MsgIDHeader, msgID)
	_, err := a.js.PublishMsg(ctx, msg)
	return err
}

// LogProvider is the default replaceable provider: it logs deliveries
// instead of calling an external gateway.
type LogProvider struct {
	Logger Logger
}

func (p *LogProvider) Send(_ context.Context, msg Message) error {
	p.Logger.Info("notification delivered",
		"msg_id", msg.ID,
		"channel", msg.Channel,
		"recipient", msg.Recipient,
	)
	return nil
}

var _ = uuid.Nil
