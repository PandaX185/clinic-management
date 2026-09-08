// Package api is the HTTP transport layer for the catalog feature.
// It maps gin requests to the directory service and owns response DTOs. It
// depends on directory/service (and auth/api for the admin gate); nothing
// below it depends back on this package.
package api

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/PandaX185/lahza/internal/catalog/service"
)

type profileResponse struct {
	ID          string   `json:"id"`
	UserID      string   `json:"user_id"`
	DisplayName string   `json:"display_name"`
	Status      string   `json:"status"`
	Roles       []string `json:"roles"`
	CreatedAt   string   `json:"created_at"`
}

type createProfileInput struct {
	UserID      string `json:"user_id" binding:"required,uuid"`
	DisplayName string `json:"display_name" binding:"required,max=255"`
	Role        string `json:"role"`
}

type profilesListResponse struct {
	Items []profileResponse `json:"items"`
}

type typeResponse struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	DurationMinutes int32  `json:"duration_minutes"`
	Price           string `json:"price"`
	Color           string `json:"color,omitempty"`
	Icon            string `json:"icon,omitempty"`
	CreatedAt       string `json:"created_at"`
}

type typesListResponse struct {
	Items []typeResponse `json:"items"`
}

type typeInput struct {
	Name            string `json:"name" binding:"required,max=100"`
	DurationMinutes int32  `json:"duration_minutes" binding:"required"`
	Price           string `json:"price"`
	Color           string `json:"color"`
	Icon            string `json:"icon"`
}

// weeklyHourInput is one recurring availability window. Times are "15:04".
type weeklyHourInput struct {
	DayOfWeek    int32  `json:"day_of_week" binding:"min=0,max=6"`
	StartTime    string `json:"start_time" binding:"required,datetime=15:04"`
	EndTime      string `json:"end_time" binding:"required,datetime=15:04"`
	SlotDuration int    `json:"slot_duration" binding:"min=0"`
}

type scheduleInput struct {
	Weekly []weeklyHourInput `json:"weekly"`
}

type exceptionInput struct {
	Date      string `json:"date" binding:"required,datetime=2006-01-02"`
	Type      string `json:"type" binding:"required,oneof=leave holiday unavailable extra_hours"`
	StartTime string `json:"start_time" binding:"required,datetime=15:04"`
	EndTime   string `json:"end_time" binding:"required,datetime=15:04"`
	Reason    string `json:"reason"`
}

type weeklyHourResponse struct {
	DayOfWeek    int32  `json:"day_of_week"`
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
	SlotDuration int    `json:"slot_duration"`
	IsActive     bool   `json:"is_active"`
}

type scheduleExceptionResponse struct {
	ID        string `json:"id"`
	Date      string `json:"date"`
	Type      string `json:"type"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Reason    string `json:"reason,omitempty"`
}

type scheduleResponse struct {
	DoctorID   string                      `json:"doctor_id"`
	Weekly     []weeklyHourResponse        `json:"weekly"`
	Exceptions []scheduleExceptionResponse `json:"exceptions"`
}

// Clock layout used for TIME-valued fields in schedule DTOs.
func hhmm(minutes int) string {
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

func minutesOf(hhmmValue string) int {
	parts := strings.Split(hhmmValue, ":")
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	return h*60 + m
}

// weeklyHours converts validated inputs into service weekly windows.
func weeklyHours(input []weeklyHourInput) ([]service.WeeklyHour, error) {
	out := make([]service.WeeklyHour, 0, len(input))
	for _, in := range input {
		out = append(out, service.WeeklyHour{
			DayOfWeek:    in.DayOfWeek,
			StartMin:     minutesOf(in.StartTime),
			EndMin:       minutesOf(in.EndTime),
			SlotDuration: in.SlotDuration,
			IsActive:     true,
		})
	}
	return out, nil
}

func toProfileResponse(p *service.Profile) profileResponse {
	roles := p.Roles
	if roles == nil {
		roles = []string{}
	}
	return profileResponse{
		ID:          p.ID.String(),
		UserID:      p.UserID.String(),
		DisplayName: p.DisplayName,
		Status:      p.Status,
		Roles:       roles,
		CreatedAt:   p.CreatedAt,
	}
}

func toProfileResponses(items []service.Profile) []profileResponse {
	out := make([]profileResponse, 0, len(items))
	for i := range items {
		out = append(out, toProfileResponse(&items[i]))
	}
	return out
}

func toTypeResponse(t *service.AppointmentType) typeResponse {
	return typeResponse{
		ID:              t.ID.String(),
		Name:            t.Name,
		DurationMinutes: t.DurationMinutes,
		Price:           t.Price,
		Color:           t.Color,
		Icon:            t.Icon,
		CreatedAt:       t.CreatedAt,
	}
}

func toTypeResponses(items []service.AppointmentType) []typeResponse {
	out := make([]typeResponse, 0, len(items))
	for i := range items {
		out = append(out, toTypeResponse(&items[i]))
	}
	return out
}

func toScheduleResponse(doctorID uuid.UUID, s *service.Schedule) scheduleResponse {
	weekly := make([]weeklyHourResponse, 0, len(s.Weekly))
	for _, w := range s.Weekly {
		weekly = append(weekly, weeklyHourResponse{
			DayOfWeek:    w.DayOfWeek,
			StartTime:    hhmm(w.StartMin),
			EndTime:      hhmm(w.EndMin),
			SlotDuration: w.SlotDuration,
			IsActive:     w.IsActive,
		})
	}
	exceptions := make([]scheduleExceptionResponse, 0, len(s.Exceptions))
	for _, e := range s.Exceptions {
		exceptions = append(exceptions, scheduleExceptionResponse{
			ID:        e.ID.String(),
			Date:      e.Date.Format("2006-01-02"),
			Type:      e.Type,
			StartTime: hhmm(e.StartMin),
			EndTime:   hhmm(e.EndMin),
			Reason:    e.Reason,
		})
	}
	return scheduleResponse{
		DoctorID:   doctorID.String(),
		Weekly:     weekly,
		Exceptions: exceptions,
	}
}
