package auth

import (
	"context"
	"errors"

	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

type PostgresRepository struct {
	q *db.Queries
}

func NewPostgresRepository(pool db.DBTX) *PostgresRepository {
	return &PostgresRepository{q: db.New(pool)}
}

func (r *PostgresRepository) CreateUser(ctx context.Context, email, passwordHash, fullName string, phone *string, role Role) (*User, error) {
	u, err := r.q.CreateUser(ctx, db.CreateUserParams{
		Email:        email,
		PasswordHash: passwordHash,
		FullName:     fullName,
		Phone:        phone,
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	initialRole := RolePatient
	if role == RoleDoctor || role == RoleStaff {
		initialRole = role
	}
	if err := r.q.AssignUserRole(ctx, db.AssignUserRoleParams{
		UserID: u.ID,
		Name:   string(initialRole),
	}); err != nil {
		return nil, apperr.Internal(err)
	}
	return &User{
		ID:           u.ID,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		FullName:     u.FullName,
		Phone:        u.Phone,
		Roles:        []Role{initialRole},
		IsActive:     u.IsActive,
	}, nil
}

func (r *PostgresRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.Unauthorized("invalid credentials")
		}
		return nil, apperr.Internal(err)
	}
	return r.toUser(ctx, row.ID, row.Email, row.PasswordHash, row.FullName, row.Phone, row.IsActive)
}

func (r *PostgresRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	row, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("user not found")
		}
		return nil, apperr.Internal(err)
	}
	return r.toUser(ctx, row.ID, row.Email, row.PasswordHash, row.FullName, row.Phone, row.IsActive)
}

func (r *PostgresRepository) toUser(ctx context.Context, id uuid.UUID, email, hash, name string, phone *string, active bool) (*User, error) {
	roleRows, err := r.q.ListUserRoles(ctx, id)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	roles := make([]Role, 0, len(roleRows))
	for _, rr := range roleRows {
		roles = append(roles, Role(rr))
	}
	return &User{
		ID:           id,
		Email:        email,
		PasswordHash: hash,
		FullName:     name,
		Phone:        phone,
		Roles:        roles,
		IsActive:     active,
	}, nil
}
