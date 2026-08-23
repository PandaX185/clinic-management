package notification

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
)

// Worker runs a durable JetStream consumer, invoking Handler for every
// message. Bounded redelivery comes from the stream's MaxDeliver; the
// handler decides per-attempt whether an error is retryable.
type Worker struct {
	js     jetstream.JetStream
	handle func(ctx context.Context, msgID uuid.UUID) error
	logger Logger
}

func NewWorker(js jetstream.JetStream, handle func(context.Context, uuid.UUID) error, logger Logger) *Worker {
	return &Worker{js: js, handle: handle, logger: logger}
}

func (w *Worker) Run(ctx context.Context) error {
	consumer, err := w.js.CreateOrUpdateConsumer(ctx, "NOTIFICATIONS", jetstream.ConsumerConfig{
		Durable:       "notification-worker",
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    MaxAttempts,
		FilterSubject: subjectSend,
	})
	if err != nil {
		return err
	}

	_, err = consumer.Consume(func(msg jetstream.Msg) {
		w.process(ctx, msg)
	})
	if err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}

func (w *Worker) process(ctx context.Context, msg jetstream.Msg) {
	msgIDStr := msg.Headers().Get(jetstream.MsgIDHeader)
	msgID, err := uuid.Parse(msgIDStr)
	if err != nil {
		w.logger.Error("discarding message without parseable id", "raw", msgIDStr)
		_ = msg.Term()
		return
	}

	err = w.handle(ctx, msgID)
	switch {
	case err == nil:
		_ = msg.DoubleAck(ctx)
	case errors.As(err, new(*RetryableError)):
		var re *RetryableError
		errors.As(err, &re)
		if nakErr := msg.NakWithDelay(re.RetryIn); nakErr != nil {
			w.logger.Error("nak failed", "err", nakErr)
		}
		w.logger.Warn("delivery attempt failed; will retry", "msg_id", msgID, "err", err)
	default:
		w.logger.Error("terminal processing failure", "msg_id", msgID, "err", err)
		_ = msg.Term()
	}
}
