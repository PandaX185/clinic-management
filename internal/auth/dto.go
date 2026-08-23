package auth

import (
	"github.com/google/uuid"
)

// Transport-layer DTOs. These live at the edge only: handlers decode them
// from JSON, call the service with plain values / domain types, and map the
// result back. Services never see these structs.

// errBody is the uniform JSON error envelope.
type errBody struct {
	Error string `json:"error"`
}

type tokenResponse struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	TokenType    string  `json:"token_type"`
	ExpiresIn    int64   `json:"expires_in"` // access TTL in seconds
	User         userDTO `json:"user"`
}

type userDTO struct {
	ID       uuid.UUID `json:"id"`
	Email    string    `json:"email"`
	FullName string    `json:"full_name"`
	Phone    string    `json:"phone,omitempty"`
	Roles    []string  `json:"roles"`
}

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	FullName string `json:"full_name" binding:"required"`
	Phone    string `json:"phone"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func newUserDTO(u *User, roles []string) userDTO {
	if roles == nil {
		roles = []string{}
	}
	return userDTO{
		ID: u.ID, Email: u.Email,
		FullName: u.FullName, Phone: u.Phone, Roles: roles,
	}
}
