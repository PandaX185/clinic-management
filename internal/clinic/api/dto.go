// Package api is the HTTP transport layer for the clinic feature. It maps gin
// requests to the clinic service and owns the response DTOs. It depends on
// clinic/service and httpctx; nothing below it depends back on this package.
package api

import (
	"github.com/PandaX185/lahza/internal/clinic/service"
)

type clinicResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	IsActive bool   `json:"is_active"`
}

type clinicsListResponse struct {
	Items []clinicResponse `json:"items"`
}

type createClinicInput struct {
	Name string `json:"name" binding:"required,max=255"`
	Slug string `json:"slug" binding:"required,max=63"`
}

type bindStaffInput struct {
	UserID string `json:"user_id" binding:"required,uuid"`
	Role   string `json:"role"`
}

type bindStaffResponse struct {
	Bound bool `json:"bound"`
}

func toResponse(t *service.Clinic) clinicResponse {
	return clinicResponse{
		ID:       t.ID.String(),
		Name:     t.Name,
		Slug:     t.Slug,
		IsActive: t.IsActive,
	}
}

func toResponses(items []service.Clinic) []clinicResponse {
	out := make([]clinicResponse, 0, len(items))
	for i := range items {
		out = append(out, toResponse(&items[i]))
	}
	return out
}
