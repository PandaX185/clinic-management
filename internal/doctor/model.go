package doctor

import (
	"time"

	"github.com/google/uuid"
)

type Doctor struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	Specialization string
	LicenseNumber  string
	Bio            *string
	FullName       string
	Email          string
	IsActive       bool
	CreatedAt      time.Time
}

type CreateDoctorInput struct {
	UserID         uuid.UUID
	Specialization string
	LicenseNumber  string
	Bio            *string
}

type Schedule struct {
	ID          uuid.UUID `json:"id"`
	DayOfWeek   int       `json:"day_of_week"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	SlotMinutes int       `json:"slot_minutes"`
}

type AddScheduleInput struct {
	DayOfWeek   int `json:"day_of_week" binding:"min=0,max=6"`
	StartHour   int `json:"start_hour" binding:"min=0,max=23"`
	StartMinute int `json:"start_minute" binding:"min=0,max=59"`
	EndHour     int `json:"end_hour" binding:"min=0,max=23"`
	EndMinute   int `json:"end_minute" binding:"min=0,max=59"`
	SlotMinutes int `json:"slot_minutes" binding:"omitempty,min=5,max=240"`
}

type AvailabilitySlot struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type AvailabilityResponse struct {
	DoctorID string             `json:"doctor_id"`
	Date     string             `json:"date"`
	Slots    []AvailabilitySlot `json:"slots"`
}
