package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// User is the auth domain model (decoupled from the sqlc-generated row so
// services can be unit tested without a database).
type User struct {
	ID            uuid.UUID
	Email         string
	PasswordHash  string
	FullName      string
	Phone         string
	IsActive      bool
	EmailVerified bool
}

// UserStore is the persistence port the auth service depends on.
// Implementations: SQLUserStore (sqlstore.go).
type UserStore interface {
	CreateUser(ctx context.Context, email, passwordHash, fullName, phone string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserRoles(ctx context.Context, userID uuid.UUID) ([]string, error)
}

// TokenStore manages refresh-token revocation state. Implementations:
// RedisTokenStore (redistore.go).
type TokenStore interface {
	// SaveRefresh stores jti -> userID with the given TTL.
	SaveRefresh(ctx context.Context, jti, userID string, ttl time.Duration) error
	// ConsumeRefresh atomically checks-and-deletes the refresh token jti,
	// returning the stored userID. Returns ErrRefreshNotFound if the token
	// was already consumed/revoked. This prevents the TOCTOU race where two
	// concurrent requests both observe the token before either deletes it.
	ConsumeRefresh(ctx context.Context, jti string) (userID string, err error)
	// DeleteRefresh revokes the refresh token jti without consuming it
	// (used by logout).
	DeleteRefresh(ctx context.Context, jti string) error
}

// Sentinel domain errors. The transport layer maps these to HTTP status
// codes; stores translate driver-specific failures into them.
var (
	ErrDuplicateEmail     = errors.New("auth: email already registered")
	ErrUserNotFound       = errors.New("auth: user not found")
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrAccountDisabled    = errors.New("auth: account disabled")
	ErrRefreshNotFound    = errors.New("auth: refresh token not found")
	ErrRefreshInvalid     = errors.New("auth: refresh token invalid")
	ErrRefreshRevoked     = errors.New("auth: refresh token revoked")
)
