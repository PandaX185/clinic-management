package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

// IdentityResolver maps an authenticated user to their domain entity IDs so
// the service can scope access by role (SEC-02). Both lookups return
// uuid.Nil when no linked record exists.
type IdentityResolver interface {
	PatientIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	DoctorIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
}

// AccessContext carries the authenticated actor's identity into the service
// layer so ownership rules can be enforced in one place.
type AccessContext struct {
	UserID  uuid.UUID
	Roles   []string
	ActorID *uuid.UUID // pointer form for audit rows
}

func (a AccessContext) hasRole(role string) bool {
	for _, r := range a.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// IsPrivileged reports whether the actor may see and manage any appointment
// (admin/staff). Doctors are privileged for their own appointments only;
// patients only for their own.
func (a AccessContext) IsPrivileged() bool {
	return a.hasRole("admin") || a.hasRole("staff")
}

// Scope describes what filter set applies to a read or mutation.
type scope struct {
	patientID *uuid.UUID
	doctorID  *uuid.UUID
	deny      bool // patient/doctor with no linked entity: nothing is theirs
}

func (s *Service) resolveScope(ctx context.Context, ac AccessContext) (scope, error) {
	if ac.IsPrivileged() {
		return scope{}, nil
	}
	if ac.hasRole("patient") && !ac.hasRole("doctor") {
		pid, err := s.identity.PatientIDForUser(ctx, ac.UserID)
		if err != nil {
			return scope{}, err
		}
		if pid == uuid.Nil {
			return scope{deny: true}, nil
		}
		return scope{patientID: &pid}, nil
	}
	if ac.hasRole("doctor") && !ac.hasRole("patient") {
		did, err := s.identity.DoctorIDForUser(ctx, ac.UserID)
		if err != nil {
			return scope{}, err
		}
		if did == uuid.Nil {
			return scope{deny: true}, nil
		}
		return scope{doctorID: &did}, nil
	}
	// Mixed or unrecognized role sets: treat as unprivileged with no grants.
	return scope{deny: true}, nil
}

// canAccessAppointment checks read/mutate access for a specific appointment.
func (s *Service) canAccessAppointment(sc scope, appt *Appointment) error {
	if sc.deny {
		return apperr.Forbidden("you do not have access to this appointment")
	}
	if sc.patientID != nil && appt.PatientID != *sc.patientID {
		return apperr.Forbidden("you do not have access to this appointment")
	}
	if sc.doctorID != nil && appt.DoctorID != *sc.doctorID {
		return apperr.Forbidden("you do not have access to this appointment")
	}
	return nil
}
