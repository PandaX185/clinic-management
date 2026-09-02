package api

import (
	"github.com/PandaX185/clinic-management/internal/appointment/service"
)

type appointmentResponse struct {
	ID                 string  `json:"id"`
	PatientID          string  `json:"patient_id"`
	DoctorID           string  `json:"doctor_id"`
	StartTime          string  `json:"start_time"`
	EndTime            string  `json:"end_time"`
	Status             string  `json:"status"`
	Notes              *string `json:"notes,omitempty"`
	CancellationReason *string `json:"cancellation_reason,omitempty"`
	Version            int32   `json:"version"`
	CreatedAt          string  `json:"created_at"`
}

type appointmentsListResponse struct {
	Items  []appointmentResponse `json:"items"`
	Total  int                   `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

func toResponse(a *service.Appointment) *appointmentResponse {
	return &appointmentResponse{
		ID:                 a.ID.String(),
		PatientID:          a.PatientID.String(),
		DoctorID:           a.DoctorID.String(),
		StartTime:          a.StartTime.Format("2006-01-02T15:04:05Z07:00"),
		EndTime:            a.EndTime.Format("2006-01-02T15:04:05Z07:00"),
		Status:             string(a.Status),
		Notes:              a.Notes,
		CancellationReason: a.CancellationReason,
		Version:            a.Version,
		CreatedAt:          a.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
