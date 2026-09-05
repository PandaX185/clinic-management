// Package service owns the auth application boundary: the use cases, the
// domain types they traffic in, and the Repository port that persistence
// adapters must satisfy. The HTTP layer (api) and persistence adapters (repo)
// both depend on this package, never the other way around.
package service

import (
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleAdmin   Role = "admin"
	RoleStaff   Role = "staff"
	RoleDoctor  Role = "doctor"
	RolePatient Role = "patient"
	RoleNurse   Role = "nurse"
	RoleManager Role = "manager"
)

type User struct {
	ID           uuid.UUID
	Phone        string
	PasswordHash string
	FullName     string
	IsActive     bool
	IsAdmin      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Claims struct {
	UserID uuid.UUID
	Type   string
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

// UserClinic represents a clinic that a user has access to, with their role.
// It is produced by the membership provider / repository and serialized by
// the HTTP layer.
type UserClinic struct {
	ClinicID   uuid.UUID `json:"clinic_id"`
	ClinicName string    `json:"clinic_name"`
	ClinicSlug string    `json:"clinic_slug"`
	RoleName   string    `json:"role_name"`
}

type RegisterInput struct {
	Phone    string `json:"phone" binding:"required,e164"`
	Password string `json:"password" binding:"required,min=8,max=72"`
	FullName string `json:"full_name" binding:"required,max=255"`
}

type LoginInput struct {
	Phone    string `json:"phone" binding:"required,e164"`
	Password string `json:"password" binding:"required"`
}
