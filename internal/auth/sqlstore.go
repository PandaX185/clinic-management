package auth

import (
	"context"
	"errors"

	sqlc "github.com/axiom/clinic-appointment/internal/platform/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// SQLUserStore adapts the sqlc-generated Queries to the auth.UserStore
// interface.
type SQLUserStore struct {
	q *sqlc.Queries
}

func NewSQLUserStore(q *sqlc.Queries) *SQLUserStore {
	return &SQLUserStore{q: q}
}

var uniqueViolationCode = "23505"

func (s *SQLUserStore) CreateUser(ctx context.Context, email, passwordHash, fullName, phone string) (*User, error) {
	u, err := s.q.CreateUser(ctx, sqlc.CreateUserParams{
		Email:        email,
		PasswordHash: passwordHash,
		FullName:     fullName,
		Phone:        pgtype.Text{String: phone, Valid: phone != ""},
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
			return nil, ErrDuplicateEmail
		}
		return nil, err
	}
	return fromSQL(u), nil
}

func (s *SQLUserStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	u, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return fromSQL(u), nil
}

func (s *SQLUserStore) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := s.q.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	roles := make([]string, 0, len(rows))
	for _, r := range rows {
		roles = append(roles, r.Name)
	}
	return roles, nil
}

func fromSQL(u sqlc.User) *User {
	return &User{
		ID:            u.ID,
		Email:         u.Email,
		PasswordHash:  u.PasswordHash,
		FullName:      u.FullName,
		Phone:         u.Phone.String,
		IsActive:      u.IsActive,
		EmailVerified: u.EmailVerified,
	}
}
