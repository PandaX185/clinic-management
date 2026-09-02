// Package api is the HTTP transport layer for the clinic directory feature.
// It maps gin requests to the directory service and owns response DTOs. It
// depends on directory/service (and auth/api for the admin gate); nothing
// below it depends back on this package.
package api

import (
	"github.com/PandaX185/clinic-management/internal/directory/service"
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
