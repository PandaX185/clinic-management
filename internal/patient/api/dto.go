// Package api is the HTTP transport layer for the patient portal. It maps gin
// requests to the patient service and owns the response DTOs. Portal routes
// require an authenticated user but never carry an X-Tenant-ID: the clinic is
// always chosen explicitly (clinic_id) or derived per appointment.
package api

import (
	"time"

	"github.com/PandaX185/clinic-management/internal/patient/service"
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
