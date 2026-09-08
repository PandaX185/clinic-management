// Package notification turns appointment lifecycle events published on the
// NATS event stream into deliverable notifications for end users.
package notification

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	schedsvc "github.com/PandaX185/lahza/internal/scheduling/service"
)

// Notification is the canonical outbound message produced from an appointment
// event. The current delivery layer is a stub (structured log + metric); the
// fields are rich enough for a future SMS/email/push channel or a persistent
// inbox without schema changes.
type Notification struct {
	ID            uuid.UUID `json:"id"`
	EventType     string    `json:"event_type"`
	Subject       string    `json:"subject"`
	Body          string    `json:"body"`
	AppointmentID uuid.UUID `json:"appointment_id"`
	PatientID     uuid.UUID `json:"patient_id"`
	DoctorID      uuid.UUID `json:"doctor_id"`
	StartsAt      time.Time `json:"starts_at"`
	EndsAt        time.Time `json:"ends_at"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

// Notifier delivers a rendered notification to the patient/staff of a clinic.
type Notifier interface {
	Deliver(ctx context.Context, n Notification) error
}

// EventBus is the minimal publish surface the publisher adapter needs. It is
// satisfied by *natsclient.Client.
type EventBus interface {
	Publish(ctx context.Context, subject string, payload []byte) error
}

// ErrorLogger is the minimal failure-reporting seam for the publisher; it is
// satisfied by *slog.Logger. Optional — nil disables reporting.
type ErrorLogger interface {
	Error(msg string, args ...any)
}

// AppointmentEventPublisher adapts the NATS event bus to the appointment
// service's EventPublisher seam. Messages land on SubjectNotify for the
// notification worker to dequeue. Publication is best-effort (it is not
// transactional with the domain write), so failures are reported through
// Log + Errors instead of being dropped silently.
type AppointmentEventPublisher struct {
	Bus     EventBus
	Subject string
	Log     ErrorLogger
	Errors  prometheus.Counter
}

func (p AppointmentEventPublisher) PublishAppointmentEvent(ctx context.Context, e schedsvc.Event) {
	payload, err := json.Marshal(e)
	if err != nil {
		p.report(err, "marshal")
		return
	}
	if err := p.Bus.Publish(ctx, p.Subject, payload); err != nil {
		p.report(err, "publish")
	}
}

func (p AppointmentEventPublisher) report(err error, stage string) {
	if p.Errors != nil {
		p.Errors.Inc()
	}
	if p.Log != nil {
		p.Log.Error("appointment event lost", "stage", stage, "subject", p.Subject, "error", err.Error())
	}
}
