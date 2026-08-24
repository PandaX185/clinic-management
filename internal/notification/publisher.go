package notification

import (
	"context"

	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/appointment"
)

// Provider abstracts an external delivery system (email/SMS gateway).
// Treated as a replaceable dependency per SRS section 2.2.
type Provider interface {
	Send(ctx context.Context, msg Message) error
}

type PublisherDeps struct {
	JetPublisher JetPublisher
	Store        Store
}

// JetPublisher is the narrow NATS surface the service needs.
type JetPublisher interface {
	Publish(ctx context.Context, subject string, payload []byte, msgID string) error
}

// Store persists notification records and tracks delivery state.
type Store interface {
	CreatePending(ctx context.Context, msg Message) (uuid.UUID, error)
	GetByMsgID(ctx context.Context, id uuid.UUID) (*Record, error)
	MarkStatus(ctx context.Context, id uuid.UUID, status string, lastErr *string) error
}

type Record struct {
	ID        uuid.UUID
	Status    string
	Attempts  int32
	Channel   Channel
	Recipient string
	Subject   string
	Body      string
}

var _ appointment.EventPublisher = (*EventForwarder)(nil)

// EventForwarder adapts appointment lifecycle events into durable
// notification messages published onto the JetStream work queue.
type EventForwarder struct {
	deps PublisherDeps
}

func NewEventForwarder(deps PublisherDeps) *EventForwarder {
	return &EventForwarder{deps: deps}
}

func (f *EventForwarder) PublishAppointmentEvent(ctx context.Context, event appointment.Event) {
	msg := buildMessage(event)
	msg.ID = uuid.New()
	id, err := f.deps.Store.CreatePending(ctx, *msg)
	if err != nil {
		return
	}
	payload, _ := jsonMarshal(msg)
	if pubErr := f.deps.JetPublisher.Publish(ctx, subjectSend, payload, id.String()); pubErr != nil {
		return
	}
}
