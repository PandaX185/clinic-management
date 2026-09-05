package service

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository defines the persistence operations the auth service depends on.
// PostgresRepository (package repo) implements it; the service only sees this
// port so persistence can be swapped independently of the application logic.
type Repository interface {
	CreateUser(ctx context.Context, phone, passwordHash, fullName string) (*User, error)
	GetUserByPhone(ctx context.Context, phone string) (*User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	IsGlobalAdmin(ctx context.Context, userID uuid.UUID) (bool, error)
	StoreRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	DeleteRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string) error
	ValidateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string) error
}
