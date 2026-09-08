// Package api is the HTTP transport layer for public clinic discovery. It is
// unauthenticated and maps gin requests to the public service, owning the
// response DTOs. It depends on public/service; nothing below it depends back
// on this package.
package api

import (
	"time"

	"github.com/PandaX185/lahza/internal/booking/service"
)

type clinicListItem struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Slug      string            `json:"slug"`
	Address   *string           `json:"address,omitempty"`
	City      *string           `json:"city,omitempty"`
	Phone     *string           `json:"phone,omitempty"`
	Hours     map[string]string `json:"hours"`
	CreatedAt string            `json:"created_at"`
}

type clinicsListResponse struct {
	Items      []clinicListItem `json:"items"`
	Pagination pagination       `json:"pagination"`
}

type clinicDetailResponse struct {
	clinicListItem
	Email       *string               `json:"email,omitempty"`
	Description *string               `json:"description,omitempty"`
	Services    []serviceItemResponse `json:"services"`
	Doctors     []doctorResponse      `json:"doctors"`
}

type serviceItemResponse struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	DurationMin int     `json:"duration_min"`
	Price       float64 `json:"price,omitempty"`
}

type doctorResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Specialty  string `json:"specialty,omitempty"`
	ClinicID   string `json:"clinic_id,omitempty"`
	ClinicName string `json:"clinic_name,omitempty"`
}

type doctorsListResponse struct {
	Items      []doctorResponse `json:"items"`
	Pagination pagination       `json:"pagination"`
}

type slotResponse struct {
	Start             string `json:"start"`
	End               string `json:"end"`
	DoctorID          string `json:"doctor_id"`
	AppointmentTypeID string `json:"appointment_type_id,omitempty"`
}

type slotsResponse struct {
	Items []slotResponse `json:"items"`
}

type pagination struct {
	Page  int   `json:"page"`
	Size  int   `json:"size"`
	Total int64 `json:"total"`
}

func toClinicListItem(c service.Clinic) clinicListItem {
	return clinicListItem{
		ID:        c.ID.String(),
		Name:      c.Name,
		Slug:      c.Slug,
		Address:   c.Address,
		City:      c.City,
		Phone:     c.Phone,
		Hours:     c.Hours,
		CreatedAt: formatTime(c.CreatedAt),
	}
}

func toClinicDetail(c service.Clinic) clinicDetailResponse {
	item := toClinicListItem(c)
	services := make([]serviceItemResponse, 0, len(c.Services))
	for _, s := range c.Services {
		services = append(services, serviceItemResponse{
			ID:          s.ID.String(),
			Name:        s.Name,
			DurationMin: s.DurationMin,
			Price:       s.Price,
		})
	}
	doctors := make([]doctorResponse, 0, len(c.Doctors))
	for _, d := range c.Doctors {
		doctors = append(doctors, toDoctorResponse(d))
	}
	return clinicDetailResponse{
		clinicListItem: item,
		Email:          c.Email,
		Description:    c.Description,
		Services:       services,
		Doctors:        doctors,
	}
}

func toDoctorResponse(d service.Doctor) doctorResponse {
	return doctorResponse{
		ID:         d.ID.String(),
		Name:       d.Name,
		Specialty:  d.Specialty,
		ClinicID:   d.ClinicID.String(),
		ClinicName: d.ClinicName,
	}
}

func toSlotResponse(s service.Slot) slotResponse {
	return slotResponse{
		Start:             s.Start.Format(time.RFC3339),
		End:               s.End.Format(time.RFC3339),
		DoctorID:          s.DoctorID.String(),
		AppointmentTypeID: s.AppointmentTypeID.String(),
	}
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC3339)
}
