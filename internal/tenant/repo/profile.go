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

// PostgresProfileStore is a service.ProfileStore implementation that reads
// from the active tenant schema via ScopedPool. The tenant slug is carried
// on the context by TenantMiddleware (database.WithTenantSlug) and is the
// only permitted source of the schema name.
type PostgresProfileStore struct {
	pool *pgxpool.Pool
}

// NewScopedProfileStore creates a ProfileStore from the raw pool.
func NewScopedProfileStore(pool *pgxpool.Pool) *PostgresProfileStore {
	return &PostgresProfileStore{pool: pool}
}

// RoleForUser returns every role the caller holds in the active tenant. The
// caller must have pinned the tenant slug on ctx (via TenantMiddleware); an
// absent slug fails closed with an empty slice so the middleware falls back
// to patient-level access rather than guessing a schema.
func (s *PostgresProfileStore) RoleForUser(ctx context.Context, userID uuid.UUID) ([]string, error) {
	slug := database.TenantSlugFrom(ctx)
	if slug == "" {
		return nil, apperr.Internal(errors.New("tenant scope missing from context"))
	}

	var roles []string
	err := database.NewScopedPool(s.pool).WithSchema(ctx, slug, func(tx pgx.Tx) error {
		profile, err := db.New(tx).GetProfileByUserID(ctx, userID)
		if err != nil {
			return err
		}
		rows, err := db.New(tx).ListUserRoles(ctx, profile.ID)
		if err != nil {
			return err
		}
		roles = make([]string, 0, len(rows))
		for _, r := range rows {
			if r.Name != "" {
				roles = append(roles, r.Name)
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No profile in this tenant → caller is a patient-level visitor.
			return nil, nil
		}
		return nil, apperr.Internal(err)
	}
	return roles, nil
}

// EnsurePatientProfile creates the caller's patient profile in the active
// tenant if it does not already exist. Booking relies on this so a patient
// with no explicit profile can still be recorded as the appointment owner.
func (s *PostgresProfileStore) EnsurePatientProfile(ctx context.Context, userID uuid.UUID) error {
	slug := database.TenantSlugFrom(ctx)
	if slug == "" {
		return apperr.Internal(errors.New("tenant scope missing from context"))
	}
	return database.NewScopedPool(s.pool).WithSchema(ctx, slug, func(tx pgx.Tx) error {
		if _, err := db.New(tx).UpsertPatientProfile(ctx, db.UpsertPatientProfileParams{
			UserID:      userID,
			DisplayName: "Patient",
		}); err != nil {
			return err
		}
		return nil
	})
}
