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
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    int64
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
