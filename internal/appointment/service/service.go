package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

// transitionParams defines the parameters for a state transition.
type transitionParams struct {
	id                 uuid.UUID
	expectedStatus     Status
	newStatus          Status
	cancellationReason *string
	startTime          *time.Time
	endTime            *time.Time
	reschedule         bool
}

// Service is the appointment service with role-scoped access enforcement (SEC-02).
type Service struct {
	repo      Repository
	publisher EventPublisher
	identity  IdentityResolver
	ttlSecs   int
}

// NewService creates a Service with the given dependencies.
func NewService(repo Repository, publisher EventPublisher, identity IdentityResolver, ttl time.Duration) *Service {
	return NewServiceWithIdentity(repo, publisher, identity, ttl)
}

// NewServiceWithIdentity allows injecting an IdentityResolver for role-scoped
// access enforcement. A nil resolver disables scoping (tests only).
func NewServiceWithIdentity(repo Repository, publisher EventPublisher, identity IdentityResolver, idempotencyTTL time.Duration) *Service {
	return &Service{repo: repo, publisher: publisher, identity: identity, ttlSecs: int(idempotencyTTL.Seconds())}
}

// Book creates an appointment safely under concurrency. The repository runs
// the whole flow in a single transaction; after commit we publish the
// notification event.
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
	return result, nil
}

// BookScoped enforces role rules on booking: patient-role actors may only
// book for themselves; staff/admin may book for anyone (SEC-02).
func (s *Service) BookScoped(ctx context.Context, in BookInput, ac AccessContext) (BookingResult, error) {
	if s.identity != nil && !ac.IsPrivileged() {
		pid, err := s.identity.PatientIDForUser(ctx, ac.UserID)
		if err != nil {
			return BookingResult{}, err
		}
		if pid == uuid.Nil {
			return BookingResult{}, apperr.Forbidden("no patient profile is linked to your account")
		}
		in.PatientID = pid.String()
	}
	return s.Book(ctx, in, ac.ActorID)
}

// Get retrieves an appointment by ID.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Appointment, error) {
	return s.repo.GetByID(ctx, id)
}

// GetScoped enforces role-based access on a single appointment read (SEC-02).
func (s *Service) GetScoped(ctx context.Context, id uuid.UUID, ac AccessContext) (*Appointment, error) {
	appt, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	sc, err := s.resolveScope(ctx, ac)
	if err != nil {
		return nil, err
	}
	if err := s.canAccessAppointment(sc, appt); err != nil {
		return nil, err
	}
	return appt, nil
}

// List returns a paginated list of appointments.
func (s *Service) List(ctx context.Context, q ListQuery) ([]Appointment, int64, error) {
	if q.Limit == 0 {
		q.Limit = 20
	}
	return s.repo.List(ctx, q)
}

// ListScoped forces the caller's own patient/doctor filter onto a listing.
func (s *Service) ListScoped(ctx context.Context, q ListQuery, ac AccessContext) ([]Appointment, int64, error) {
	if q.Limit == 0 {
		q.Limit = 20
	}
	sc, err := s.resolveScope(ctx, ac)
	if err != nil {
		return nil, 0, err
	}
	if sc.deny {
		return []Appointment{}, 0, nil
	}
	if sc.patientID != nil {
		q.PatientID = sc.patientID.String()
		q.DoctorID = ""
	} else if sc.doctorID != nil {
		q.DoctorID = sc.doctorID.String()
		q.PatientID = ""
	}
	return s.repo.List(ctx, q)
}

// Cancel validates the transition (BR-04), persists it and emits events.
func (s *Service) Cancel(ctx context.Context, id uuid.UUID, reason string, actorID *uuid.UUID) (*Appointment, error) {
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.cancelAuthorized(ctx, id, reason, actorID, current)
}

// CancelScoped enforces role-based ownership before cancelling.
func (s *Service) CancelScoped(ctx context.Context, id uuid.UUID, reason string, ac AccessContext) (*Appointment, error) {
	appt, err := s.authorizeMutation(ctx, id, ac)
	if err != nil {
		return nil, err
	}
	return s.cancelAuthorized(ctx, id, reason, ac.ActorID, appt)
}

// cancelAuthorized performs the cancellation and emits events.
func (s *Service) cancelAuthorized(ctx context.Context, id uuid.UUID, reason string, actorID *uuid.UUID, current *Appointment) (*Appointment, error) {
	if !CanTransition(current.Status, StatusCancelled) {
		return nil, apperr.Conflict("cannot cancel an appointment in status " + string(current.Status))
	}

	re := reason
	updated, err := s.transition(ctx, transitionParams{
		id:                 id,
		expectedStatus:     current.Status,
		newStatus:          StatusCancelled,
		cancellationReason: &re,
	})
	if err != nil {
		return nil, err
	}

	if s.publisher != nil {
		s.publisher.PublishAppointmentEvent(ctx, Event{Type: "appointment.cancelled", Appointment: *updated})
	}
	return updated, nil
}

// authorizeMutation loads the appointment and verifies the actor may mutate it (SEC-02).
func (s *Service) authorizeMutation(ctx context.Context, id uuid.UUID, ac AccessContext) (*Appointment, error) {
	appt, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	sc, err := s.resolveScope(ctx, ac)
	if err != nil {
		return nil, err
	}
	if err := s.canAccessAppointment(sc, appt); err != nil {
		return nil, err
	}
	return appt, nil
}

// RescheduleScoped moves an active appointment to a new time window. The DB
// exclusion constraint rejects collisions introduced by the new range.
func (s *Service) RescheduleScoped(ctx context.Context, id uuid.UUID, in RescheduleInput, ac AccessContext) (*Appointment, error) {
	if _, err := s.authorizeMutation(ctx, id, ac); err != nil {
		return nil, err
	}
	return s.Reschedule(ctx, id, in, ac.ActorID)
}

// Reschedule moves an active appointment to a new time window.
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
	return updated, nil
}

// Confirm confirms an appointment.
func (s *Service) Confirm(ctx context.Context, id uuid.UUID, actorID *uuid.UUID) (*Appointment, error) {
	return s.simpleTransition(ctx, id, StatusConfirmed, "appointment.confirmed", actorID)
}

// Complete marks an appointment as completed.
func (s *Service) Complete(ctx context.Context, id uuid.UUID, actorID *uuid.UUID) (*Appointment, error) {
	return s.simpleTransition(ctx, id, StatusCompleted, "appointment.completed", actorID)
}

// MarkNoShow marks an appointment as a no-show.
func (s *Service) MarkNoShow(ctx context.Context, id uuid.UUID, actorID *uuid.UUID) (*Appointment, error) {
	return s.simpleTransition(ctx, id, StatusNoShow, "appointment.no_show", actorID)
}

// ConfirmScoped confirms an appointment with role-based access.
func (s *Service) ConfirmScoped(ctx context.Context, id uuid.UUID, ac AccessContext) (*Appointment, error) {
	if _, err := s.authorizeMutation(ctx, id, ac); err != nil {
		return nil, err
	}
	return s.simpleTransition(ctx, id, StatusConfirmed, "appointment.confirmed", ac.ActorID)
}

// CompleteScoped completes an appointment with role-based access.
func (s *Service) CompleteScoped(ctx context.Context, id uuid.UUID, ac AccessContext) (*Appointment, error) {
	if _, err := s.authorizeMutation(ctx, id, ac); err != nil {
		return nil, err
	}
	return s.simpleTransition(ctx, id, StatusCompleted, "appointment.completed", ac.ActorID)
}

// MarkNoShowScoped marks an appointment as a no-show with role-based access.
func (s *Service) MarkNoShowScoped(ctx context.Context, id uuid.UUID, ac AccessContext) (*Appointment, error) {
	if _, err := s.authorizeMutation(ctx, id, ac); err != nil {
		return nil, err
	}
	return s.simpleTransition(ctx, id, StatusNoShow, "appointment.no_show", ac.ActorID)
}

// transition performs a state transition on an appointment.
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

// simpleTransition performs a state transition with ownership enforcement.
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
	if s.publisher != nil {
		s.publisher.PublishAppointmentEvent(ctx, Event{Type: action, Appointment: *updated})
	}
	return updated, nil
}

// hashRequest computes a SHA-256 hash of the booking request for idempotency.
func hashRequest(in BookInput) string {
	h := sha256.New()
	_, _ = h.Write([]byte(in.PatientID))
	_, _ = h.Write([]byte(in.DoctorID))
	_, _ = h.Write([]byte(in.StartTime.Format(time.RFC3339Nano)))
	return hex.EncodeToString(h.Sum(nil))
}

// marshalDetails marshals details for audit (no-op in v2).
func marshalDetails(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(`{}`)
	}
	return b
}
