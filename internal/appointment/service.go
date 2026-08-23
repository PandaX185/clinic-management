package appointment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

type AuditEntry struct {
	ActorID    *uuid.UUID
	Action     string
	EntityID   uuid.UUID
	EntityType string
	Details    []byte
}

type AuditWriter interface {
	Write(ctx context.Context, entry AuditEntry) error
}

type Service struct {
	repo      Repository
	publisher EventPublisher
	audit     AuditWriter
	ttlSecs   int
}

func NewService(repo Repository, publisher EventPublisher, audit AuditWriter, idempotencyTTL time.Duration) *Service {
	return &Service{repo: repo, publisher: publisher, audit: audit, ttlSecs: int(idempotencyTTL.Seconds())}
}

// Book creates an appointment safely under concurrency. The repository runs
// the whole flow in a single transaction; only after commit do we publish the
// notification event and write the audit entry (best-effort, non-blocking for
// correctness of the booking itself).
func (s *Service) Book(ctx context.Context, in BookInput, actorID *uuid.UUID) (BookingResult, error) {
	patientID, err := uuid.Parse(in.PatientID)
	if err != nil {
		return BookingResult{}, apperr.Invalid("invalid patient_id")
	}
	doctorID, err := uuid.Parse(in.DoctorID)
	if err != nil {
		return BookingResult{}, apperr.Invalid("invalid doctor_id")
	}
	if !in.StartTime.After(time.Now()) {
		return BookingResult{}, apperr.Invalid("start_time must be in the future")
	}
	endTime := in.StartTime.Add(time.Duration(in.DurationMinutes) * time.Minute)

	result, err := s.repo.BookTx(ctx, BookTxParams{
		IdempotencyKey: in.IdempotencyKey,
		PatientID:      patientID,
		DoctorID:       doctorID,
		StartTime:      in.StartTime.UTC(),
		EndTime:        endTime.UTC(),
		Notes:          in.Notes,
		CreatedBy:      actorID,
		RequestHash:    hashRequest(in),
		TTLSeconds:     s.ttlSecs,
	})
	if err != nil {
		return result, err
	}

	if result.Appointment != nil && s.publisher != nil {
		s.publisher.PublishAppointmentEvent(ctx, Event{
			Type:        "appointment.booked",
			Appointment: *result.Appointment,
		})
	}
	if result.Appointment != nil && s.audit != nil {
		s.audit.Write(ctx, AuditEntry{
			ActorID:    actorID,
			Action:     "appointment.booked",
			EntityID:   result.Appointment.ID,
			EntityType: "appointment",
		})
	}
	return result, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Appointment, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context, q ListQuery) ([]Appointment, int64, error) {
	if q.Limit == 0 {
		q.Limit = 20
	}
	return s.repo.List(ctx, q)
}

// Cancel validates the transition (BR-04), persists it and emits events.
func (s *Service) Cancel(ctx context.Context, id uuid.UUID, reason string, actorID *uuid.UUID) (*Appointment, error) {
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !CanTransition(current.Status, StatusCancelled) {
		return nil, apperr.Conflict("cannot cancel an appointment in status " + string(current.Status))
	}

	reasonCopy := reason
	updated, err := s.transition(ctx, transitionParams{
		id:                 id,
		expectedStatus:     current.Status,
		newStatus:          StatusCancelled,
		cancellationReason: &reasonCopy,
	})
	if err != nil {
		return nil, err
	}

	if s.publisher != nil {
		s.publisher.PublishAppointmentEvent(ctx, Event{Type: "appointment.cancelled", Appointment: *updated})
	}
	if s.audit != nil {
		s.audit.Write(ctx, AuditEntry{
			ActorID:    actorID,
			Action:     "appointment.cancelled",
			EntityID:   id,
			EntityType: "appointment",
			Details:    []byte(`{"reason":` + quote(reason) + `}`),
		})
	}
	return updated, nil
}

// Reschedule moves an active appointment to a new time window. The DB
// exclusion constraint rejects collisions introduced by the new range.
func (s *Service) Reschedule(ctx context.Context, id uuid.UUID, in RescheduleInput, actorID *uuid.UUID) (*Appointment, error) {
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !IsActive(current.Status) {
		return nil, apperr.Conflict("cannot reschedule an appointment in status " + string(current.Status))
	}
	if !in.StartTime.After(time.Now()) {
		return nil, apperr.Invalid("new start_time must be in the future")
	}
	end := in.StartTime.Add(time.Duration(in.DurationMinutes) * time.Minute)

	startUTC := in.StartTime.UTC()
	endUTC := end.UTC()
	updated, err := s.transition(ctx, transitionParams{
		id:             id,
		expectedStatus: current.Status,
		newStatus:      current.Status,
		startTime:      &startUTC,
		endTime:        &endUTC,
		reschedule:     true,
	})
	if err != nil {
		return nil, err
	}

	if s.publisher != nil {
		s.publisher.PublishAppointmentEvent(ctx, Event{Type: "appointment.rescheduled", Appointment: *updated})
	}
	if s.audit != nil {
		s.audit.Write(ctx, AuditEntry{
			ActorID:    actorID,
			Action:     "appointment.rescheduled",
			EntityID:   id,
			EntityType: "appointment",
		})
	}
	return updated, nil
}

func (s *Service) Confirm(ctx context.Context, id uuid.UUID, actorID *uuid.UUID) (*Appointment, error) {
	return s.simpleTransition(ctx, id, StatusConfirmed, "appointment.confirmed", actorID)
}

func (s *Service) Complete(ctx context.Context, id uuid.UUID, actorID *uuid.UUID) (*Appointment, error) {
	return s.simpleTransition(ctx, id, StatusCompleted, "appointment.completed", actorID)
}

func (s *Service) MarkNoShow(ctx context.Context, id uuid.UUID, actorID *uuid.UUID) (*Appointment, error) {
	return s.simpleTransition(ctx, id, StatusNoShow, "appointment.no_show", actorID)
}

type simpleSpec struct {
	target Status
	action string
}

func (s *Service) transition(ctx context.Context, p transitionParams) (*Appointment, error) {
	params := TransitionParams{
		ID:             p.id,
		ExpectedStatus: p.expectedStatus,
		NewStatus:      p.newStatus,
	}
	if p.cancellationReason != nil {
		reason := *p.cancellationReason
		params.CancellationReason = &reason
	}
	if p.reschedule {
		params.NewStartTime = p.startTime
		params.NewEndTime = p.endTime
	}
	return s.repo.Transition(ctx, params)
}

type transitionParams struct {
	id                 uuid.UUID
	expectedStatus     Status
	newStatus          Status
	cancellationReason *string
	startTime          *time.Time
	endTime            *time.Time
	reschedule         bool
}

func (s *Service) simpleTransition(ctx context.Context, id uuid.UUID, target Status, action string, actorID *uuid.UUID) (*Appointment, error) {
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !CanTransition(current.Status, target) {
		return nil, apperr.Conflict("cannot move appointment from " + string(current.Status) + " to " + string(target))
	}
	updated, err := s.transition(ctx, transitionParams{
		id:             id,
		expectedStatus: current.Status,
		newStatus:      target,
	})
	if err != nil {
		return nil, err
	}
	if s.audit != nil {
		s.audit.Write(ctx, AuditEntry{
			ActorID:    actorID,
			Action:     action,
			EntityID:   id,
			EntityType: "appointment",
		})
	}
	return updated, nil
}

func hashRequest(in BookInput) string {
	h := sha256.New()
	h.Write([]byte(in.PatientID))
	h.Write([]byte(in.DoctorID))
	h.Write([]byte(in.StartTime.Format(time.RFC3339Nano)))
	return hex.EncodeToString(h.Sum(nil))
}

func quote(s string) string { return `"` + s + `"` }
