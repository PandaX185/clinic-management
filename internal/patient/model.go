package patient

import (
	"time"

	"github.com/google/uuid"
)

type Patient struct {
	ID                    uuid.UUID
	UserID                *uuid.UUID
	FullName              string
	DateOfBirth           *time.Time
	Gender                *string
	Phone                 *string
	Address               *string
	EmergencyContactName  *string
	EmergencyContactPhone *string
	MedicalNotes          *string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type CreateInput struct {
	FullName              string     `json:"full_name" binding:"required,max=255"`
	DateOfBirth           *time.Time `json:"date_of_birth" binding:"omitempty"`
	Gender                *string    `json:"gender" binding:"omitempty,oneof=male female other"`
	Phone                 *string    `json:"phone" binding:"omitempty,max=50"`
	Address               *string    `json:"address"`
	EmergencyContactName  *string    `json:"emergency_contact_name" binding:"omitempty,max=255"`
	EmergencyContactPhone *string    `json:"emergency_contact_phone" binding:"omitempty,max=50"`
	MedicalNotes          *string    `json:"medical_notes"`
}

type UpdateInput struct {
	FullName              *string `json:"full_name" binding:"omitempty,min=1,max=255"`
	Gender                *string `json:"gender" binding:"omitempty,oneof=male female other"`
	Phone                 *string `json:"phone" binding:"omitempty,max=50"`
	Address               *string `json:"address" binding:"omitempty"`
	EmergencyContactName  *string `json:"emergency_contact_name" binding:"omitempty,max=255"`
	EmergencyContactPhone *string `json:"emergency_contact_phone" binding:"omitempty,max=50"`
}

type ListQuery struct {
	Search string `form:"search"`
	Limit  int    `form:"limit,default=20" binding:"min=1,max=100"`
	Offset int    `form:"offset" binding:"min=0"`
}
