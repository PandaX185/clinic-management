package service

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Appointment, error)
	List(ctx context.Context, q ListQuery) ([]Appointment, int64, error)

	BookTx(ctx context.Context, params BookTxParams) (BookingResult, error)
	Transition(ctx context.Context, p TransitionParams) (*Appointment, error)
}

type TransitionParams struct {
	ID                 uuid.UUID
	ExpectedStatus     Status
	NewStatus          Status
	CancellationReason *string
	NewStartTime       *time.Time
	NewEndTime         *time.Time
}

type BookTxParams struct {
	IdempotencyKey string
	PatientID      uuid.UUID
	// PatientUser is the global user ID behind the patient; when set, the
	// booking auto-provisions their profile row in this clinic's schema.
	PatientUser *uuid.UUID
	DoctorID    uuid.UUID
	StartTime   time.Time
	EndTime     time.Time
	Notes       *string
	CreatedBy   *uuid.UUID
	RequestHash string
	TTLSeconds  int
}

type BookingResult struct {
	Appointment  *Appointment
	Replayed     bool
	StoredStatus int
	StoredBody   []byte
}

// Event is published after a committed appointment change so the
// notification worker can deliver messages asynchronously.
type Event struct {
	Type        string      `json:"type"`
	Appointment Appointment `json:"appointment"`
}

type EventPublisher interface {
	PublishAppointmentEvent(ctx context.Context, event Event)
}
