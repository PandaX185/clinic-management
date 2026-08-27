package appointment

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusScheduled Status = "scheduled"
	StatusConfirmed Status = "confirmed"
	StatusCompleted Status = "completed"
	StatusCancelled Status = "cancelled"
	StatusNoShow    Status = "no_show"
)

var allowedTransitions = map[Status][]Status{
	StatusScheduled: {StatusConfirmed, StatusCancelled, StatusNoShow},
	StatusConfirmed: {StatusCompleted, StatusCancelled, StatusNoShow},
	StatusCompleted: {},
	StatusCancelled: {},
	StatusNoShow:    {},
}

func CanTransition(from, to Status) bool {
	for _, t := range allowedTransitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

func IsActive(s Status) bool {
	return s == StatusScheduled || s == StatusConfirmed
}

type Appointment struct {
	ID                 uuid.UUID  `json:"id"`
	PatientID          uuid.UUID  `json:"patient_id"`
	DoctorID           uuid.UUID  `json:"doctor_id"`
	AppointmentTypeID  uuid.UUID  `json:"appointment_type_id"`
	StartTime          time.Time  `json:"start_time"`
	EndTime            time.Time  `json:"end_time"`
	Status             Status     `json:"status"`
	Notes              *string    `json:"notes,omitempty"`
	CancellationReason *string    `json:"cancellation_reason,omitempty"`
	Version            int32      `json:"version"`
	CreatedBy          uuid.UUID  `json:"-"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"-"`
}

type BookInput struct {
	IdempotencyKey  string    `json:"-" header:"Idempotency-Key"`
	PatientID       string    `json:"patient_id" binding:"required,uuid"`
	DoctorID        string    `json:"doctor_id" binding:"required,uuid"`
	StartTime       time.Time `json:"start_time" binding:"required"`
	DurationMinutes int       `json:"duration_minutes" binding:"required,min=5,max=480"`
	Notes           *string   `json:"notes"`
}

type RescheduleInput struct {
	StartTime       time.Time `json:"start_time" binding:"required"`
	DurationMinutes int       `json:"duration_minutes" binding:"required,min=5,max=480"`
}

type CancelInput struct {
	Reason string `json:"reason" binding:"required,max=1000"`
}

type ListQuery struct {
	PatientID string     `form:"patient_id"`
	DoctorID  string     `form:"doctor_id"`
	Status    string     `form:"status" binding:"omitempty,oneof=scheduled confirmed completed cancelled no_show"`
	From      *time.Time `form:"from" time_format:"2006-01-02T15:04:05Z07:00"`
	To        *time.Time `form:"to" time_format:"2006-01-02T15:04:05Z07:00"`
	Limit     int        `form:"limit,default=20" binding:"min=1,max=100"`
	Offset    int        `form:"offset" binding:"min=0"`
}