package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
)

// UserTenant represents a tenant that a user has access to, with their role.
type UserTenant struct {
	TenantID   uuid.UUID
	TenantName string
	TenantSlug string
	RoleName   string
}

// Repository defines persistence operations for the auth package.
type Repository interface {
	CreateUser(ctx context.Context, phone, passwordHash, fullName string) (*User, error)
	GetUserByPhone(ctx context.Context, phone string) (*User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	StoreRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	DeleteRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string) error
	ValidateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string) error
	ListTenantsForUser(ctx context.Context, userID uuid.UUID) ([]UserTenant, error)
}

// PostgresRepository is the PostgreSQL implementation of Repository.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateUser(ctx context.Context, phone, passwordHash, fullName string) (*User, error) {
	row, err := db.New(r.pool).CreateUser(ctx, db.CreateUserParams{
		Phone:        phone,
		PasswordHash: passwordHash,
		FullName:     fullName,
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return userFromRow(row), nil
}

func (r *PostgresRepository) GetUserByPhone(ctx context.Context, phone string) (*User, error) {
	row, err := db.New(r.pool).GetUserByPhone(ctx, phone)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return userFromRow(row), nil
}

func (r *PostgresRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	row, err := db.New(r.pool).GetUserByID(ctx, id)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return userFromRow(row), nil
}

func (r *PostgresRepository) StoreRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	return db.New(r.pool).InsertRefreshToken(ctx, db.InsertRefreshTokenParams{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	})
}

func (r *PostgresRepository) DeleteRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string) error {
	return db.New(r.pool).DeleteRefreshToken(ctx, db.DeleteRefreshTokenParams{
		UserID:    userID,
		TokenHash: tokenHash,
	})
}

func (r *PostgresRepository) ValidateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string) error {
	_, err := db.New(r.pool).ValidateRefreshToken(ctx, db.ValidateRefreshTokenParams{
		UserID:    userID,
		TokenHash: tokenHash,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Unauthorized("refresh token was revoked or doesn't exist")
		}
		return apperr.Internal(err)
	}
	return nil
}

// ListTenantsForUser returns all tenants the user has a profile in, with their role in each.
//
// Note: This is a placeholder. The full implementation requires running the query
// inside each tenant's schema (since profiles and roles are tenant-scoped), which
// the auth package doesn't have direct access to. The handler should call
// tenant.Service.TenantsForUser instead.
func (r *PostgresRepository) ListTenantsForUser(ctx context.Context, userID uuid.UUID) ([]UserTenant, error) {
	return nil, nil
}

func userFromRow(row db.User) *User {
	return &User{
		ID:           row.ID,
		Phone:        row.Phone,
		PasswordHash: row.PasswordHash,
		FullName:     row.FullName,
		IsActive:     row.Status == "active",
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}
