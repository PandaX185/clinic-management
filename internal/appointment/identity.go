package appointment

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
)

type PostgresIdentityResolver struct {
	pool *pgxpool.Pool
}

func NewPostgresIdentityResolver(pool *pgxpool.Pool) *PostgresIdentityResolver {
	return &PostgresIdentityResolver{pool: pool}
}

// PatientIDForUser returns uuid.Nil when no patient row is linked.
func (r *PostgresIdentityResolver) PatientIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	profile, err := db.New(r.pool).GetProfileByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, nil
		}
		return uuid.Nil, apperr.Internal(err)
	}
	return profile.ID, nil
}

// DoctorIDForUser returns uuid.Nil in v2 — doctors are profiles too and
// there's no separate doctor-patient link. Access is role-based instead.
func (r *PostgresIdentityResolver) DoctorIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	// In v2, doctors are profiles; access is checked via role_permissions
	// and profile_roles rather than separate doctor IDs.
	// Return uuid.Nil and enforce authorization at the service level.
	return uuid.Nil, nil
}