// Package service owns the clinic queue use cases: checking patients in,
// calling them from the line, and completing/skipping/cancelling entries.
// Position is derived from (priority DESC, checked_in_at ASC) among active
// entries — it is never stored. Persistence goes through the Repository port
// implemented by internal/queue/repo.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Queue entry lifecycle statuses.
const (
	StatusWaiting    = "waiting"
	StatusCalled     = "called"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusSkipped    = "skipped"
	StatusCancelled  = "cancelled"
)

// Entry is one check-in in a clinic's queue.
type Entry struct {
	ID            uuid.UUID
	ProfileID     uuid.UUID
	AppointmentID *uuid.UUID
	PatientName   string
	Status        string
	Priority      int32
	CheckedInAt   time.Time
	CalledAt      *time.Time
	StartedAt     *time.Time
	CompletedAt   *time.Time
	// Position is the 1-based spot in the active line; 0 when the entry is
	// not currently active. ActiveTotal is the current line length.
	Position    int64
	ActiveTotal int64
}

// ListQuery filters the active queue.
type ListQuery struct {
	From   *time.Time // only entries checked in at/after this instant
	Limit  int32
	Offset int32
}

// Repository is the persistence port for the clinic queue. The active tenant
// schema is selected inside the implementation.
type Repository interface {
	CreateEntry(ctx context.Context, profileID uuid.UUID, appointmentID *uuid.UUID, priority int32) (*Entry, error)
	GetEntry(ctx context.Context, id uuid.UUID) (*Entry, error)
	ListActive(ctx context.Context, q ListQuery) ([]Entry, int64, error)
	ListForProfile(ctx context.Context, profileID uuid.UUID) ([]Entry, error)
	Position(ctx context.Context, priority int32, checkedInAt time.Time) (int64, error)
	Transition(ctx context.Context, id uuid.UUID, target string) (*Entry, error)
}

// validTransitions is the status machine of a queue entry.
var validTransitions = map[string]map[string]bool{
	StatusWaiting:    {StatusCalled: true, StatusSkipped: true, StatusCancelled: true},
	StatusCalled:     {StatusWaiting: true, StatusInProgress: true, StatusSkipped: true, StatusCancelled: true},
	StatusInProgress: {StatusCompleted: true, StatusCancelled: true},
}

// CanTransition reports whether the status machine allows from -> to.
func CanTransition(from, to string) bool {
	return validTransitions[from][to]
}

// Active reports whether a status participates in the line.
func (e *Entry) Active() bool {
	return e.Status == StatusWaiting || e.Status == StatusCalled || e.Status == StatusInProgress
}
