// Package service holds the patient portal use cases: the patient's own
// identity, their appointments across every clinic they attend, and booking
// management. It has no database access; persistence goes through the
// Repository port. Appointment mutations delegate to the appointment service
// with a tenant-scoped context derived from the chosen clinic.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	apptsvc "github.com/PandaX185/clinic-management/internal/appointment/service"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/database"
)

// User is the patient's global identity.
type User struct {
	ID       uuid.UUID
	Phone    string
	FullName string
	IsActive bool
}

// UpdateUserInput is the mutable identity fields of a user.
type UpdateUserInput struct {
	FullName string
	Phone    string
}

// ClinicRef identifies a clinic a patient operates against.
type ClinicRef struct {
	ID   uuid.UUID
	Name string
	Slug string
}

// Appointment is a patient's appointment annotated with the clinic it belongs
// to. Because patients span clinics, the appointment alone cannot imply a
// tenant schema; ClinicID always accompanies it.
type Appointment struct {
	ID                uuid.UUID
	PatientID         uuid.UUID
	DoctorID          uuid.UUID
	AppointmentTypeID uuid.UUID
	StartTime         time.Time
	EndTime           time.Time
	Status            string
	Notes             *string
	ClinicID          uuid.UUID
	ClinicName        string
}

// BookInput describes a new patient booking.
type BookInput struct {
	ClinicID        uuid.UUID
	DoctorID        uuid.UUID
	StartTime       time.Time
	DurationMinutes int
	Notes           *string
	IdempotencyKey  string
}

// RescheduleInput describes the new timing for an appointment.
type RescheduleInput struct {
	StartTime       time.Time
	DurationMinutes int
}

// Repository is the persistence port for the patient portal. Everything
// tenant-specific (profiles, appointments) is resolved per clinic schema.
type Repository interface {
	Me(ctx context.Context, userID uuid.UUID) (*User, error)
	UpdateMe(ctx context.Context, userID uuid.UUID, in UpdateUserInput) error
	GetClinic(ctx context.Context, clinicID uuid.UUID) (*ClinicRef, error)
	ListAppointments(ctx context.Context, userID uuid.UUID) ([]Appointment, error)
	GetAppointment(ctx context.Context, userID uuid.UUID, apptID, clinicID uuid.UUID) (*Appointment, error)
	EnsurePatientProfile(ctx context.Context, slug string, userID uuid.UUID) (uuid.UUID, error)
}

// Service is the patient portal use case surface.
type Service struct {
	repo    Repository
	apptSvc *apptsvc.Service
}

func NewService(repo Repository, apptSvc *apptsvc.Service) *Service {
	return &Service{repo: repo, apptSvc: apptSvc}
}

func (s *Service) Me(ctx context.Context, userID uuid.UUID) (*User, error) {
	user, err := s.repo.Me(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, apperr.Unauthorized("account is deactivated")
	}
	return user, nil
}

func (s *Service) UpdateMe(ctx context.Context, userID uuid.UUID, in UpdateUserInput) error {
	if len(in.FullName) < 1 || len(in.FullName) > 255 {
		return apperr.Invalid("full_name is required")
	}
	return s.repo.UpdateMe(ctx, userID, in)
}

// ListAppointments returns the patient's appointments across every clinic
// they attend, newest first.
func (s *Service) ListAppointments(ctx context.Context, userID uuid.UUID) ([]Appointment, error) {
	return s.repo.ListAppointments(ctx, userID)
}

// GetAppointment returns a single appointment, pinning the clinic explicitly
// because the appointment id alone does not identify a tenant schema.
func (s *Service) GetAppointment(ctx context.Context, userID uuid.UUID, apptID, clinicID uuid.UUID) (*Appointment, error) {
	return s.repo.GetAppointment(ctx, userID, apptID, clinicID)
}

// Book creates an appointment in the given clinic on the patient's behalf.
// A patient profile is provisioned in the clinic on first booking.
func (s *Service) Book(ctx context.Context, userID uuid.UUID, in BookInput) (*Appointment, error) {
	clinic, err := s.repo.GetClinic(ctx, in.ClinicID)
	if err != nil {
		return nil, err
	}
	if in.DoctorID == uuid.Nil {
		return nil, apperr.Invalid("doctor_id is required")
	}
	if in.StartTime.IsZero() {
		return nil, apperr.Invalid("start_time is required")
	}

	if _, err := s.repo.EnsurePatientProfile(ctx, clinic.Slug, userID); err != nil {
		return nil, err
	}

	result, err := s.apptSvc.BookScoped(database.WithTenantSlug(ctx, clinic.Slug), apptsvc.BookInput{
		DoctorID:        in.DoctorID.String(),
		StartTime:       in.StartTime,
		DurationMinutes: in.DurationMinutes,
		Notes:           in.Notes,
		IdempotencyKey:  in.IdempotencyKey,
	}, patientAccess(userID))
	if err != nil {
		return nil, err
	}
	if result.Appointment == nil {
		return nil, apperr.Conflict("booking could not be completed")
	}
	return toPatientAppointment(result.Appointment, clinic), nil
}

// Cancel cancels a patient's appointment in the given clinic.
func (s *Service) Cancel(ctx context.Context, userID uuid.UUID, clinicID, apptID uuid.UUID, reason string) (*Appointment, error) {
	clinic, err := s.repo.GetClinic(ctx, clinicID)
	if err != nil {
		return nil, err
	}
	updated, err := s.apptSvc.CancelScoped(database.WithTenantSlug(ctx, clinic.Slug), apptID, reason, patientAccess(userID))
	if err != nil {
		return nil, err
	}
	return toPatientAppointment(updated, clinic), nil
}

// Reschedule moves a patient's appointment to a new time in the given clinic.
func (s *Service) Reschedule(ctx context.Context, userID uuid.UUID, clinicID, apptID uuid.UUID, in RescheduleInput) (*Appointment, error) {
	clinic, err := s.repo.GetClinic(ctx, clinicID)
	if err != nil {
		return nil, err
	}
	updated, err := s.apptSvc.RescheduleScoped(database.WithTenantSlug(ctx, clinic.Slug), apptID, apptsvc.RescheduleInput{
		StartTime:       in.StartTime,
		DurationMinutes: in.DurationMinutes,
	}, patientAccess(userID))
	if err != nil {
		return nil, err
	}
	return toPatientAppointment(updated, clinic), nil
}

// patientAccess builds the always-patient AccessContext for the portal. The
// appointment service then forces bookings/read/mutations onto the caller's
// own patient profile regardless of any client-supplied ids.
func patientAccess(userID uuid.UUID) apptsvc.AccessContext {
	uid := userID
	return apptsvc.AccessContext{UserID: userID, Roles: []string{"patient"}, ActorID: &uid}
}

func toPatientAppointment(a *apptsvc.Appointment, clinic *ClinicRef) *Appointment {
	return &Appointment{
		ID:                a.ID,
		PatientID:         a.PatientID,
		DoctorID:          a.DoctorID,
		AppointmentTypeID: a.AppointmentTypeID,
		StartTime:         a.StartTime,
		EndTime:           a.EndTime,
		Status:            string(a.Status),
		Notes:             a.Notes,
		ClinicID:          clinic.ID,
		ClinicName:        clinic.Name,
	}
}
