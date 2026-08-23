package doctor

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	Create(ctx context.Context, in CreateDoctorInput) (*Doctor, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Doctor, error)
	List(ctx context.Context, activeOnly bool, specialization string, limit, offset int) ([]Doctor, error)

	AddSchedule(ctx context.Context, doctorID uuid.UUID, dayOfWeek int, start, end time.Time, slotMinutes int) (*Schedule, error)
	GetSchedules(ctx context.Context, doctorID uuid.UUID) ([]Schedule, error)
	RemoveSchedule(ctx context.Context, scheduleID uuid.UUID) error
	AddException(ctx context.Context, doctorID uuid.UUID, date time.Time, isUnavailable bool, start, end *time.Time, reason string) error
	GetExceptions(ctx context.Context, doctorID uuid.UUID, from, to time.Time) ([]ScheduleExceptionRow, error)

	GetActiveAppointmentsInRange(ctx context.Context, doctorID uuid.UUID, from, to time.Time) ([]AppointmentRange, error)
}

type ScheduleExceptionRow struct {
	Date          time.Time
	IsUnavailable bool
	StartTime     *time.Time
	EndTime       *time.Time
}

type AppointmentRange struct {
	ID        uuid.UUID
	StartTime time.Time
	EndTime   time.Time
}
