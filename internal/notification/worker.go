package notification

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	natsclient "github.com/PandaX185/clinic-management/internal/platform/nats"
	schedsvc "github.com/PandaX185/clinic-management/internal/scheduling/service"
)

// Worker dequeues appointment events from the notifications JetStream stream
// and hands each to a Notifier. Messages that fail to decode or deliver are
// moved to the dead-letter subject so the original message can be acked and
// the stream stays bounded.
type Worker struct {
	client   *natsclient.Client
	notifier Notifier
	log      DLogger
}

// DLogger is the worker-facing log surface. *slog.Logger satisfies it.
type DLogger interface {
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

func NewWorker(c *natsclient.Client, notifier Notifier, log DLogger) *Worker {
	return &Worker{client: c, notifier: notifier, log: log}
}

// Run blocks until ctx is cancelled: it guarantees a durable consumer on the
// notifications subject, then consumes messages until shutdown.
func (w *Worker) Run(ctx context.Context) error {
	consumer, err := w.client.Jet.CreateOrUpdateConsumer(ctx, natsclient.StreamName, jetstream.ConsumerConfig{
		Name:          natsclient.ConsumerName,
		Durable:       natsclient.ConsumerName,
		FilterSubject: natsclient.SubjectNotify,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    5,
	})
	if err != nil {
		return fmt.Errorf("create notification consumer: %w", err)
	}

	consumeCtx, cancel := context.WithCancel(ctx)
	cc, err := consumer.Consume(func(msg jetstream.Msg) {
		w.handle(consumeCtx, msg)
	})
	if err != nil {
		cancel()
		return fmt.Errorf("start notification consumer: %w", err)
	}

	<-ctx.Done()
	cc.Stop()
	cancel()
	return nil
}

func (w *Worker) handle(ctx context.Context, msg jetstream.Msg) {
	if err := ctx.Err(); err != nil {
		_ = msg.Nak()
		return
	}
	if err := HandleMessage(ctx, msg.Data(), w.notifier); err != nil {
		w.log.Error("notification handling failed; moving to DLQ", "error", err.Error())
		w.maybeToDLQ(ctx, msg.Data())
		_ = msg.Ack()
		return
	}
	_ = msg.Ack()
}

// maybeToDLQ routes an undeliverable message to the dead-letter subject. The
// subject is pre-declared by the stream, so failures here are only logged.
func (w *Worker) maybeToDLQ(ctx context.Context, data []byte) {
	select {
	case <-ctx.Done():
		return
	default:
	}
	if err := w.client.Publish(ctx, natsclient.DLQSubject, data); err != nil {
		w.log.Warn("failed to move message to DLQ", "error", err.Error())
	}
}

// HandleMessage decodes a raw stream payload into a notification and delivers
// it. Package-level so it is unit-testable without a NATS server.
func HandleMessage(ctx context.Context, data []byte, notifier Notifier) error {
	var e schedsvc.Event
	if err := json.Unmarshal(data, &e); err != nil {
		return fmt.Errorf("decode appointment event: %w", err)
	}
	n, err := BuildNotification(e)
	if err != nil {
		return err
	}
	if err := notifier.Deliver(ctx, n); err != nil {
		return fmt.Errorf("deliver notification: %w", err)
	}
	return nil
}
