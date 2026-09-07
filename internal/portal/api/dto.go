// Package api is the HTTP transport layer for the patient portal. It maps gin
// requests to the patient service and owns the response DTOs. Portal routes
// require an authenticated user but never carry an X-Tenant-ID: the clinic is
// always chosen explicitly (clinic_id) or derived per appointment.
package api

import (
	"time"

	paymentsvc "github.com/PandaX185/clinic-management/internal/payments/service"
	"github.com/PandaX185/clinic-management/internal/portal/service"
)

type userResponse struct {
	ID       string `json:"id"`
	Phone    string `json:"phone"`
	FullName string `json:"full_name"`
}

type updateUserInput struct {
	FullName string `json:"full_name" binding:"required,max=255"`
	Phone    string `json:"phone" binding:"required,e164"`
}

type appointmentResponse struct {
	ID                string  `json:"id"`
	PatientID         string  `json:"patient_id"`
	DoctorID          string  `json:"doctor_id"`
	AppointmentTypeID string  `json:"appointment_type_id"`
	StartTime         string  `json:"start_time"`
	EndTime           string  `json:"end_time"`
	Status            string  `json:"status"`
	Notes             *string `json:"notes,omitempty"`
	ClinicID          string  `json:"clinic_id"`
	ClinicName        string  `json:"clinic_name"`
}

type appointmentsListResponse struct {
	Items []appointmentResponse `json:"items"`
}

type bookInput struct {
	ClinicID        string    `json:"clinic_id" binding:"required,uuid"`
	DoctorID        string    `json:"doctor_id" binding:"required,uuid"`
	StartTime       time.Time `json:"start_time" binding:"required"`
	DurationMinutes int       `json:"duration_minutes"`
	Notes           *string   `json:"notes"`
}

type cancelInput struct {
	ClinicID string `json:"clinic_id" binding:"required,uuid"`
	Reason   string `json:"reason" binding:"required"`
}

type payInput struct {
	ClinicID string `json:"clinic_id" binding:"required,uuid"`
	Method   string `json:"method" binding:"omitempty,oneof=cash card e_wallet"`
}

type paymentResponse struct {
	ID            string     `json:"id"`
	AppointmentID string     `json:"appointment_id"`
	Amount        string     `json:"amount"`
	Currency      string     `json:"currency"`
	Method        string     `json:"method"`
	Status        string     `json:"status"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
	Reference     *string    `json:"reference,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	ClinicID      string     `json:"clinic_id"`
	ClinicName    string     `json:"clinic_name"`
}

func toPaymentResponse(p *paymentsvc.Payment, clinic *service.ClinicRef) paymentResponse {
	return paymentResponse{
		ID:            p.ID.String(),
		AppointmentID: p.AppointmentID.String(),
		Amount:        p.Amount,
		Currency:      p.Currency,
		Method:        string(p.Method),
		Status:        string(p.Status),
		PaidAt:        p.PaidAt,
		Reference:     p.Reference,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
		ClinicID:      clinic.ID.String(),
		ClinicName:    clinic.Name,
	}
}

type rescheduleInput struct {
	ClinicID        string    `json:"clinic_id" binding:"required,uuid"`
	StartTime       time.Time `json:"start_time" binding:"required"`
	DurationMinutes int       `json:"duration_minutes"`
}

func toUserResponse(u *service.User) userResponse {
	return userResponse{ID: u.ID.String(), Phone: u.Phone, FullName: u.FullName}
}

func toAppointmentResponse(a *service.Appointment) appointmentResponse {
	return appointmentResponse{
		ID:                a.ID.String(),
		PatientID:         a.PatientID.String(),
		DoctorID:          a.DoctorID.String(),
		AppointmentTypeID: a.AppointmentTypeID.String(),
		StartTime:         a.StartTime.Format(time.RFC3339),
		EndTime:           a.EndTime.Format(time.RFC3339),
		Status:            a.Status,
		Notes:             a.Notes,
		ClinicID:          a.ClinicID.String(),
		ClinicName:        a.ClinicName,
	}
}

func toAppointmentResponses(items []service.Appointment) []appointmentResponse {
	out := make([]appointmentResponse, 0, len(items))
	for _, item := range items {
		out = append(out, toAppointmentResponse(&item))
	}
	return out
}

type joinQueueInput struct {
	ClinicID string `json:"clinic_id" binding:"required,uuid"`
}

type queueEntryResponse struct {
	ID            string     `json:"id"`
	ProfileID     string     `json:"profile_id"`
	AppointmentID *string    `json:"appointment_id,omitempty"`
	Status        string     `json:"status"`
	Priority      int32      `json:"priority"`
	CheckedInAt   time.Time  `json:"checked_in_at"`
	CalledAt      *time.Time `json:"called_at,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	Position      int64      `json:"position"`
	ActiveTotal   int64      `json:"active_total"`
	ClinicID      string     `json:"clinic_id"`
	ClinicName    string     `json:"clinic_name"`
}

type myQueueResponse struct {
	Items []queueEntryResponse `json:"items"`
}

func toQueueEntryResponse(e *service.QueueEntry) queueEntryResponse {
	var apptID *string
	if e.AppointmentID != nil {
		s := e.AppointmentID.String()
		apptID = &s
	}
	return queueEntryResponse{
		ID:            e.ID.String(),
		ProfileID:     e.ProfileID.String(),
		AppointmentID: apptID,
		Status:        e.Status,
		Priority:      e.Priority,
		CheckedInAt:   e.CheckedInAt,
		CalledAt:      e.CalledAt,
		StartedAt:     e.StartedAt,
		CompletedAt:   e.CompletedAt,
		Position:      e.Position,
		ActiveTotal:   e.ActiveTotal,
		ClinicID:      e.ClinicID.String(),
		ClinicName:    e.ClinicName,
	}
}

func toQueueEntryResponses(items []service.QueueEntry) []queueEntryResponse {
	out := make([]queueEntryResponse, 0, len(items))
	for _, item := range items {
		out = append(out, toQueueEntryResponse(&item))
	}
	return out
}
