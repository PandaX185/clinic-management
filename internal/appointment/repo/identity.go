package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/database"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
)

type PostgresIdentityResolver struct {
	pool *pgxpool.Pool
}

func NewPostgresIdentityResolver(pool *pgxpool.Pool) *PostgresIdentityResolver {
	return &PostgresIdentityResolver{pool: pool}
}

// PatientIDForUser returns the caller's profile id — appointments reference
// patients and doctors by profile id, so the same lookup serves both roles.
// Returns uuid.Nil when no profile is linked.
func (r *PostgresIdentityResolver) PatientIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return r.profileIDForUser(ctx, userID)
}

// DoctorIDForUser returns the caller's profile id. In this model doctors are
// profiles too (with the doctor role), and appointments store the doctor's
// profile id in doctor_profile_id, so the identity is the same one returned
// for patients. uuid.Nil when no profile is linked.
func (r *PostgresIdentityResolver) DoctorIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return r.profileIDForUser(ctx, userID)
}

func (r *PostgresIdentityResolver) profileIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	slug := database.TenantSlugFrom(ctx)
	if slug == "" {
		return uuid.Nil, apperr.Internal(errors.New("tenant scope missing from context"))
	}
	var profileID uuid.UUID
	err := database.NewScopedPool(r.pool).WithSchema(ctx, slug, func(tx pgx.Tx) error {
		profile, err := db.New(tx).GetProfileByUserID(ctx, userID)
		if err != nil {
			return err
		}
		profileID = profile.ID
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, nil
		}
		return uuid.Nil, apperr.Internal(err)
	}
	return profileID, nil
}
