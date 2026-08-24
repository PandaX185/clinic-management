package appointment

import (
	"context"
	"errors"

	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

// PostgresIdentityResolver resolves authenticated users to their patient or
// doctor entity IDs via the users→patients/doctors links.
type PostgresIdentityResolver struct {
	pool *pgxpool.Pool
}

func NewPostgresIdentityResolver(pool *pgxpool.Pool) *PostgresIdentityResolver {
	return &PostgresIdentityResolver{pool: pool}
}

// PatientIDForUser returns uuid.Nil when no patient row is linked.
func (r *PostgresIdentityResolver) PatientIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	id, err := db.New(r.pool).GetPatientIDByUserID(ctx, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, apperr.Internal(err)
	}
	return id, nil
}

// DoctorIDForUser returns uuid.Nil when no doctor row is linked.
func (r *PostgresIdentityResolver) DoctorIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	id, err := db.New(r.pool).GetDoctorIDByUserID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, apperr.Internal(err)
	}
	return id, nil
}
