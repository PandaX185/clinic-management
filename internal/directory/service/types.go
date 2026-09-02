// Package service owns the clinic directory application boundary: domain
// types (Profile, AppointmentType), the persistence port they travel in, and
// the directory use cases. The HTTP layer (api) and persistence adapters
// (repo) both depend on this package, never the other way around.
package service

import (
	"context"

	"github.com/google/uuid"
)

// Profile is a person registered in a clinic (patient, doctor, staff, ...).
type Profile struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	DisplayName string
	Status      string
	Roles       []string
	CreatedAt   string
	UpdatedAt   string
}

// AppointmentType is a bookable service a clinic offers (consultation, ...).
type AppointmentType struct {
	ID              uuid.UUID
	Name            string
	DurationMinutes int32
	Price           string
	Color           string
	Icon            string
	CreatedAt       string
	UpdatedAt       string
}

// Repo is the persistence port the directory service depends on. PostgresRepo
// (package repo) implements it; the service only sees this interface.
type Repo interface {
	ListProfiles(ctx context.Context) ([]Profile, error)
	CreateProfile(ctx context.Context, userID uuid.UUID, displayName, role string) (*Profile, error)
	ListDoctors(ctx context.Context) ([]Profile, error)
	ListAppointmentTypes(ctx context.Context) ([]AppointmentType, error)
	GetAppointmentType(ctx context.Context, id uuid.UUID) (*AppointmentType, error)
	CreateAppointmentType(ctx context.Context, in AppointmentType) (*AppointmentType, error)
	UpdateAppointmentType(ctx context.Context, in AppointmentType) (*AppointmentType, error)
}
