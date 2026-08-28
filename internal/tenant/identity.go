package tenant

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

// PostgresIdentityResolver resolves user identities to domain profiles.
type PostgresIdentityResolver struct {
	pool *pgxpool.Pool
}

// NewPostgresIdentityResolver creates a ProfileResolver from the raw pool.
func NewPostgresIdentityResolver(pool *pgxpool.Pool) *PostgresIdentityResolver {
	return &PostgresIdentityResolver{pool: pool}
}

// RoleForUser returns the user's role string in the current tenant.
// Empty string means no profile yet (patient-level access).
func (r *PostgresIdentityResolver) RoleForUser(ctx context.Context, userID uuid.UUID) (string, error) {
	slug := database.TenantSlugFrom(ctx)
	var role string
	err := database.NewScopedPool(r.pool).WithSchema(ctx, slug, func(tx pgx.Tx) error {
		profile, err := db.New(tx).GetProfileByUserID(ctx, userID)
		if err != nil {
			return err
		}
		roles, err := db.New(tx).ListUserRoles(ctx, profile.ID)
		if err != nil {
			return err
		}
		// Pick the first non-empty role name (highest precedence is arbitrary;
		// the per-endpoint authorization layer handles conflicts).
		for _, rl := range roles {
			if rl.Name != "" {
				role = rl.Name
				break
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", apperr.Internal(err)
	}
	return role, nil
}

// PatientIDForUser returns uuid.Nil when no patient row is linked.
func (r *PostgresIdentityResolver) PatientIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	slug := database.TenantSlugFrom(ctx)
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

// DoctorIDForUser returns uuid.Nil in v2 — doctors are profiles too and
// there's no separate doctor-patient link. Access is role-based instead.
func (r *PostgresIdentityResolver) DoctorIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	// In v2, doctors are profiles; access is checked via role_permissions
	// and profile_roles rather than separate doctor IDs.
	// Return uuid.Nil and enforce authorization at the service level.
	return uuid.Nil, nil
}
