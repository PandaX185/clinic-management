// Package service owns the clinic directory application boundary: domain
// types (Profile, AppointmentType), the persistence port they travel in, and
// the directory use cases. The HTTP layer (api) and persistence adapters
// (repo) both depend on this package, never the other way around.
package service

import (
	"context"
	"time"

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

// WeeklyHour is one recurring availability window for a doctor, in minutes
// since local midnight on DayOfWeek (0 = Sunday). SlotDuration is the default
// slot length for windows that do not pick one.
type WeeklyHour struct {
	DayOfWeek    int32
	StartMin     int
	EndMin       int
	SlotDuration int
	IsActive     bool
}

// ScheduleException is a one-off override for a doctor on a date. Type is one
// of leave, holiday, unavailable (blocked hours) or extra_hours (adds a
// window). StartMin/EndMin are minutes since midnight.
type ScheduleException struct {
	ID       uuid.UUID
	Date     time.Time
	Type     string
	StartMin int
	EndMin   int
	Reason   string
}

// Schedule is a doctor's full availability: recurring weekly windows plus
// one-off date exceptions.
type Schedule struct {
	Weekly     []WeeklyHour
	Exceptions []ScheduleException
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
	ListDoctorSchedule(ctx context.Context, doctorID uuid.UUID) ([]WeeklyHour, error)
	ReplaceWeeklySchedule(ctx context.Context, doctorID uuid.UUID, hours []WeeklyHour) error
	ListScheduleExceptions(ctx context.Context, doctorID uuid.UUID) ([]ScheduleException, error)
	AddScheduleException(ctx context.Context, doctorID uuid.UUID, ex ScheduleException) (*ScheduleException, error)
	RemoveScheduleException(ctx context.Context, exceptionID uuid.UUID) error
}
