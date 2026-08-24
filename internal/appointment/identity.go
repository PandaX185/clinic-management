package appointment

import (
	"context"
	"errors"

	"github.com/PandaX185/clinic-management/internal/platform/database"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

// PostgresIdentityResolver resolves authenticated users to their patient or
// doctor entity IDs within the ACTIVE TENANT's schema. The tenant slug comes
// from the request context (server.TenantMiddleware).
type PostgresIdentityResolver struct {
	pool *pgxpool.Pool
}

func NewPostgresIdentityResolver(pool *pgxpool.Pool) *PostgresIdentityResolver {
	return &PostgresIdentityResolver{pool: pool}
}

// PatientIDForUser returns the patient row for this user in the active
// clinic, auto-provisioning it on first access (patients are global; charts
// are per-clinic). Returns uuid.Nil only if provisioning fails.
func (r *PostgresIdentityResolver) PatientIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := database.NewScopedPool(r.pool).WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		q := db.New(tx)
		pid, err := q.GetPatientIDByUserID(ctx, &userID)
		if errors.Is(err, pgx.ErrNoRows) {
			// First action in this clinic: create their chart entry.
			row, err := q.UpsertPatientProfile(ctx, userID)
			if err != nil {
				return apperr.Internal(err)
			}
			patientID, err := q.GetPatientIDByUserID(ctx, &userID)
			if err != nil {
				return apperr.Internal(err)
			}
			id = patientID
			_ = row
			return nil
		}
		if err != nil {
			return apperr.Internal(err)
		}
		id = pid
		return nil
	})
	return id, err
}

// DoctorIDForUser returns uuid.Nil when no doctor row is linked in this
// clinic. Doctor rows are provisioned by admins, never auto-created.
func (r *PostgresIdentityResolver) DoctorIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := database.NewScopedPool(r.pool).WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		did, err := db.New(tx).GetDoctorIDByUserID(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // uuid.Nil
		}
		if err != nil {
			return apperr.Internal(err)
		}
		id = did
		return nil
	})
	return id, err
}
