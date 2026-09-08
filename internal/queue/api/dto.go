package api

import (
	"strconv"
	"time"

	queuesvc "github.com/PandaX185/lahza/internal/queue/service"
)

type checkInInput struct {
	ProfileID     string `json:"profile_id" binding:"required"`
	AppointmentID string `json:"appointment_id"`
	Priority      int32  `json:"priority"`
}

type statusInput struct {
	Status string `json:"status" binding:"required,oneof=waiting called in_progress completed skipped cancelled"`
}

type entryResponse struct {
	ID            string     `json:"id"`
	ProfileID     string     `json:"profile_id"`
	AppointmentID *string    `json:"appointment_id,omitempty"`
	PatientName   string     `json:"patient_name"`
	Status        string     `json:"status"`
	Priority      int32      `json:"priority"`
	CheckedInAt   time.Time  `json:"checked_in_at"`
	CalledAt      *time.Time `json:"called_at,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	Position      int64      `json:"position"`
	ActiveTotal   int64      `json:"active_total"`
}

type queueListResponse struct {
	Items  []entryResponse `json:"items"`
	Total  int64           `json:"total"`
	Limit  int32           `json:"limit"`
	Offset int32           `json:"offset"`
}

func toEntryResponse(e *queuesvc.Entry) entryResponse {
	var apptID *string
	if e.AppointmentID != nil {
		s := e.AppointmentID.String()
		apptID = &s
	}
	return entryResponse{
		ID:            e.ID.String(),
		ProfileID:     e.ProfileID.String(),
		AppointmentID: apptID,
		PatientName:   e.PatientName,
		Status:        e.Status,
		Priority:      e.Priority,
		CheckedInAt:   e.CheckedInAt,
		CalledAt:      e.CalledAt,
		StartedAt:     e.StartedAt,
		CompletedAt:   e.CompletedAt,
		Position:      e.Position,
		ActiveTotal:   e.ActiveTotal,
	}
}

func parseInt32(s string) (int32, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, strconv.ErrSyntax
	}
	return int32(n), nil
}
