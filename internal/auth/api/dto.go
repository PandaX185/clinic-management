package api

import (
	"time"

	"github.com/PandaX185/clinic-management/internal/auth/service"
)

type userResponse struct {
	ID        string `json:"id"`
	Phone     string `json:"phone"`
	Name      string `json:"full_name"`
	IsActive  bool   `json:"is_active"`
	CreatedAt string `json:"created_at"`
}

func toUserResponse(u *service.User) userResponse {
	return userResponse{
		ID:        u.ID.String(),
		Phone:     u.Phone,
		Name:      u.FullName,
		IsActive:  u.IsActive,
		CreatedAt: u.CreatedAt.Format(time.RFC3339),
	}
}
