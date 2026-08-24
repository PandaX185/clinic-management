package auth

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
)

type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	FullName     string
	Phone        *string
	Roles        []Role
	IsActive     bool
	CreatedAt    time.Time
}

type Claims struct {
	UserID   uuid.UUID `json:"uid"`
	Roles    []string  `json:"roles"`
	Type     string    `json:"typ"`
	TenantID uuid.UUID `json:"tid,omitempty"` // active clinic (multi-tenant)
	TenantSlug string  `json:"tslug,omitempty"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

type RegisterInput struct {
	Email       string  `json:"email" binding:"required,email"`
	Password    string  `json:"password" binding:"required,min=8,max=72"`
	FullName    string  `json:"full_name" binding:"required,max=255"`
	Phone       *string `json:"phone" binding:"omitempty,max=50"`
	InitialRole Role    `json:"role" binding:"omitempty,oneof=patient doctor staff"`
}

type LoginInput struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}
