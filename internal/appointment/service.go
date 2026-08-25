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
	identity  IdentityResolver
	ttlSecs   int
}

func NewService(repo Repository, publisher EventPublisher, audit AuditWriter, idempotencyTTL time.Duration) *Service {
	return NewServiceWithIdentity(repo, publisher, audit, nil, idempotencyTTL)
}

// NewServiceWithIdentity allows injecting an IdentityResolver for role-scoped
// access enforcement. A nil resolver disables scoping (tests only).
func NewServiceWithIdentity(repo Repository, publisher EventPublisher, audit AuditWriter, identity IdentityResolver, idempotencyTTL time.Duration) *Service {
	return &Service{repo: repo, publisher: publisher, audit: audit, identity: identity, ttlSecs: int(idempotencyTTL.Seconds())}
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

func (s *Service) List(ctx context.Context, q ListQuery) ([]Appointment, int64, error) {
	if q.Limit == 0 {
		q.Limit = 20
	}
	return s.repo.List(ctx, q)
}

// ListScoped forces the caller's own patient/doctor filter onto a listing.
// Privileged actors may pass arbitrary filters; patients and doctors cannot
// enumerate appointments that are not theirs (SEC-02).
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

// CancelScoped additionally enforces role-based ownership before cancelling.
func (s *Service) CancelScoped(ctx context.Context, id uuid.UUID, reason string, ac AccessContext) (*Appointment, error) {
	appt, err := s.authorizeMutation(ctx, id, ac)
	if err != nil {
		return nil, err
	}
	return s.cancelAuthorized(ctx, id, reason, ac.ActorID, appt)
}

func (s *Service) cancelAuthorized(ctx context.Context, id uuid.UUID, reason string, actorID *uuid.UUID, current *Appointment) (*Appointment, error) {
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

// authorizeMutation loads the appointment and verifies the actor may mutate
// it (SEC-02). Doctors may manage their own appointments; patients their own.
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

// Reschedule moves an active appointment to a new time window. The DB
// exclusion constraint rejects collisions introduced by the new range.
// RescheduleScoped enforces ownership before rescheduling.
func (s *Service) RescheduleScoped(ctx context.Context, id uuid.UUID, in RescheduleInput, ac AccessContext) (*Appointment, error) {
	if _, err := s.authorizeMutation(ctx, id, ac); err != nil {
		return nil, err
	}
	return s.Reschedule(ctx, id, in, ac.ActorID)
}

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

// Simple transitions with ownership enforcement (SEC-02).
func (s *Service) ConfirmScoped(ctx context.Context, id uuid.UUID, ac AccessContext) (*Appointment, error) {
	if _, err := s.authorizeMutation(ctx, id, ac); err != nil {
		return nil, err
	}
	return s.simpleTransition(ctx, id, StatusConfirmed, "appointment.confirmed", ac.ActorID)
}

func (s *Service) CompleteScoped(ctx context.Context, id uuid.UUID, ac AccessContext) (*Appointment, error) {
	if _, err := s.authorizeMutation(ctx, id, ac); err != nil {
		return nil, err
	}
	return s.simpleTransition(ctx, id, StatusCompleted, "appointment.completed", ac.ActorID)
}

func (s *Service) MarkNoShowScoped(ctx context.Context, id uuid.UUID, ac AccessContext) (*Appointment, error) {
	if _, err := s.authorizeMutation(ctx, id, ac); err != nil {
		return nil, err
	}
	return s.simpleTransition(ctx, id, StatusNoShow, "appointment.no_show", ac.ActorID)
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
