// Package service holds the use cases for patient-facing public clinic
// discovery: browsing clinics, doctors and the available slots a doctor has
// on a given day. It has no database access; persistence goes through the
// Repository port implemented by internal/public/repo.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Clinic is a clinic visible for public discovery.
type Clinic struct {
	ID          uuid.UUID
	Name        string
	Slug        string
	Address     *string
	City        *string
	Phone       *string
	Email       *string
	Description *string
	Hours       map[string]string
	CreatedAt   time.Time

	Services []ServiceItem
	Doctors  []Doctor
}

// ServiceItem is a bookable service type offered by a clinic.
type ServiceItem struct {
	ID          uuid.UUID
	Name        string
	DurationMin int
	Price       float64
}

// Doctor is a doctor profile exposed through public discovery. ClinicID and
// ClinicName identify the clinic the doctor practises at.
type Doctor struct {
	ID         uuid.UUID
	Name       string
	Specialty  string
	ClinicID   uuid.UUID
	ClinicName string
}

// Schedule is one recurring weekly window for a doctor, expressed in minutes
// past midnight so the slot builder is timezone-agnostic.
type Schedule struct {
	DayOfWeek int
	StartMin  int
	EndMin    int
}

// Appointment is a booked slot used to compute busy intervals.
type Appointment struct {
	Start time.Time
	End   time.Time
}

// Slot is an available booking window for a doctor on a day.
type Slot struct {
	Start             time.Time
	End               time.Time
	DoctorID          uuid.UUID
	AppointmentTypeID uuid.UUID
}

// Repository is the persistence port for public clinic discovery. Tenant
// schema selection happens inside the implementation.
type Repository interface {
	ListClinics(ctx context.Context, offset, limit int) ([]Clinic, int64, error)
	GetClinic(ctx context.Context, id uuid.UUID) (*Clinic, error)
	ListClinicServices(ctx context.Context, clinicID uuid.UUID) ([]ServiceItem, error)
	ListClinicDoctors(ctx context.Context, clinicID uuid.UUID, offset, limit int) ([]Doctor, int64, error)
	FindDoctor(ctx context.Context, doctorID uuid.UUID) (*Doctor, error)
	ListDoctorSchedules(ctx context.Context, clinicID uuid.UUID, doctorID uuid.UUID, date time.Time) ([]Schedule, error)
	ListDoctorAppointments(ctx context.Context, clinicID uuid.UUID, doctorID uuid.UUID, from, to time.Time) ([]Appointment, error)
	GetAppointmentType(ctx context.Context, clinicID uuid.UUID, typeID uuid.UUID) (*ServiceItem, error)
}
