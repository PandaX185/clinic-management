package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func Connect(natsURL string) (*nats.Conn, jetstream.JetStream, error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to NATS: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, nil, fmt.Errorf("create JetStream context: %w", err)
	}

	return nc, js, nil
}

func SetupStreams(js jetstream.JetStream) error {
	ctx := context.Background()

	// Notification stream (valid name - no dots allowed in stream names)
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "CLINIC_NOTIFICATIONS",
		Subjects: []string{"clinic.notification.*"},
		Retention: jetstream.WorkQueuePolicy,
		MaxMsgs: -1,
		Discard: jetstream.DiscardNew,
		Storage: jetstream.FileStorage,
		Replicas: 1,
	})
	if err != nil {
		// Check if stream already exists
		_, getErr := js.Stream(ctx, "CLINIC_NOTIFICATIONS")
		if getErr != nil {
			return fmt.Errorf("create notification stream: %w", err)
		}
	}
	_ = stream

	// DLQ stream - use different subject pattern to avoid overlap
	dlqStream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "CLINIC_NOTIFICATIONS_DLQ",
		Subjects: []string{"clinic.dlq.*"},  // Different subject pattern - no overlap
		Retention: jetstream.LimitsPolicy,
		Storage: jetstream.FileStorage,
		Replicas: 1,
		MaxAge: 30 * 24 * time.Hour, // 30 days
	})
	if err != nil {
		// Check if stream already exists
		_, getErr := js.Stream(ctx, "CLINIC_NOTIFICATIONS_DLQ")
		if getErr != nil {
			return fmt.Errorf("create DLQ stream: %w", err)
		}
	}
	_ = dlqStream

	// Notification consumer
	consumer, err := js.CreateConsumer(ctx, "CLINIC_NOTIFICATIONS", jetstream.ConsumerConfig{
		Durable:       "notification-worker",
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    5,
		AckWait:       30 * time.Second,
		DeliverPolicy: jetstream.DeliverAllPolicy,
	})
	if err != nil {
		// Check if consumer already exists
		_, getErr := js.Consumer(ctx, "CLINIC_NOTIFICATIONS", "notification-worker")
		if getErr != nil {
			return fmt.Errorf("create notification consumer: %w", err)
		}
	}
	_ = consumer

	return nil
}