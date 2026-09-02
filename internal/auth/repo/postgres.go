// Package repo holds the PostgreSQL adapter that satisfies the auth service's
// Repository contract. The contract itself (Repository interface) and the
// domain types it traffics in are defined in the service package so
// persistence depends on the application boundary, never the other way around.
package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/auth/service"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
)

// PostgresRepository is the PostgreSQL implementation of auth.service.Repository.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateUser(ctx context.Context, phone, passwordHash, fullName string) (*service.User, error) {
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

func (r *PostgresRepository) GetUserByPhone(ctx context.Context, phone string) (*service.User, error) {
	row, err := db.New(r.pool).GetUserByPhone(ctx, phone)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return userFromRow(row), nil
}

func (r *PostgresRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*service.User, error) {
	row, err := db.New(r.pool).GetUserByID(ctx, id)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return userFromRow(row), nil
}

// IsGlobalAdmin reports whether the user holds the global super-admin flag
// (distinct from the per-clinic "admin" role resolved from tenant profiles).
func (r *PostgresRepository) IsGlobalAdmin(ctx context.Context, userID uuid.UUID) (bool, error) {
	admin, err := db.New(r.pool).GetUserAdminFlag(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, apperr.Internal(err)
	}
	return admin, nil
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

func userFromRow(row db.User) *service.User {
	return &service.User{
		ID:           row.ID,
		Phone:        row.Phone,
		PasswordHash: row.PasswordHash,
		FullName:     row.FullName,
		IsActive:     row.Status == "active",
		IsAdmin:      row.IsAdmin,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}
