package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/appointment"
)

const (
	subjectSend = "notifications.send"
	subjectDLQ  = "notifications.dlq"
)

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

// buildMessage derives the concrete notification content from an appointment
// event. Recipient resolution is intentionally simplistic for now: the
// notification targets the patient of record via email channel.
func buildMessage(event appointment.Event) *Message {
	appt := event.Appointment
	subject := fmt.Sprintf("Appointment %s", event.Type)
	body := fmt.Sprintf(
		"Your appointment %s is scheduled for %s (status: %s).",
		appt.ID, appt.StartTime.Format(time.RFC1123), appt.Status,
	)
	return &Message{
		AppointmentID: appt.ID,
		Channel:       ChannelEmail,
		Recipient:     "patient@example.invalid",
		Subject:       subject,
		Body:          body,
	}
}

var _ slogHandler = (*noopLog)(nil)

type slogHandler interface{}
type noopLog struct{}

// Consumer processes notification messages with idempotency, bounded retries,
// and a dead-letter path (FR-NOT-02..05).
type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

type Consumer struct {
	store    Store
	jet      JetPublisher
	provider Provider
	logger   Logger
	retry    time.Duration
}

func NewConsumer(store Store, jet JetPublisher, provider Provider, logger Logger, retryDelay time.Duration) *Consumer {
	return &Consumer{store: store, jet: jet, provider: provider, logger: logger, retry: retryDelay}
}

// Handle processes one delivery attempt for a message ID.
// Idempotency: messages whose record already reached terminal state are acked.
func (c *Consumer) Handle(ctx context.Context, msgID uuid.UUID) error {
	rec, err := c.store.GetByMsgID(ctx, msgID)
	if err != nil {
		return fmt.Errorf("lookup notification %s: %w", msgID, err)
	}
	if rec.Status == StatusSent || rec.Status == StatusDeadLetter {
		c.logger.Info("notification already processed", "msg_id", msgID, "status", rec.Status)
		return nil
	}

	sendErr := c.provider.Send(ctx, Message{ID: msgID})
	if sendErr == nil {
		return c.store.MarkStatus(ctx, rec.ID, StatusSent, nil)
	}

	attempts := rec.Attempts + 1
	if int(attempts) >= MaxAttempts {
		err := c.store.MarkStatus(ctx, rec.ID, StatusDeadLetter, strPtr(sendErr.Error()))
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"msg_id": msgID.String(), "error": sendErr.Error()})
		if err := c.jet.Publish(ctx, subjectDLQ, payload, msgID.String()); err != nil {
			c.logger.Error("failed to publish dead letter", "msg_id", msgID, "err", err)
		}
		c.logger.Warn("notification moved to DLQ", "msg_id", msgID)
		return nil
	}

	if merr := c.store.MarkStatus(ctx, rec.ID, StatusFailed, strPtr(sendErr.Error())); merr != nil {
		return merr
	}
	return &RetryableError{Cause: sendErr, RetryIn: c.retry}
}

type RetryableError struct {
	Cause   error
	RetryIn time.Duration
}

func (e *RetryableError) Error() string { return e.Cause.Error() }
func (e *RetryableError) Unwrap() error { return e.Cause }

func strPtr(s string) *string { return &s }
