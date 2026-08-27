package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
)

type Repository interface {
	CreateUser(ctx context.Context, phone, passwordHash, fullName string) (*User, error)
	GetUserByPhone(ctx context.Context, phone string) (*User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
}

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
	return &User{
		ID:           row.ID,
		Phone:        row.Phone,
		PasswordHash: row.PasswordHash,
		FullName:     row.FullName,
		IsActive:     row.Status == "active",
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}, nil
}

func (r *PostgresRepository) GetUserByPhone(ctx context.Context, phone string) (*User, error) {
	row, err := db.New(r.pool).GetUserByPhone(ctx, phone)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("user not found")
		}
		return nil, apperr.Internal(err)
	}
	return &User{
		ID:           row.ID,
		Phone:        row.Phone,
		PasswordHash: row.PasswordHash,
		FullName:     row.FullName,
		IsActive:     row.Status == "active",
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}, nil
}

func (r *PostgresRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	row, err := db.New(r.pool).GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("user not found")
		}
		return nil, apperr.Internal(err)
	}
	return &User{
		ID:           row.ID,
		Phone:        row.Phone,
		PasswordHash: row.PasswordHash,
		FullName:     row.FullName,
		IsActive:     row.Status == "active",
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}, nil
}