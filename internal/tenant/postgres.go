package tenant

import (
	"context"
	"errors"

	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/database"
)

// PostgresStore handles global-registry data (tenants). Profile lookups are
// per-tenant and go through ScopedProfileStore.
type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }

// Pool satisfies the Service's PoolProvider.
func (s *PostgresStore) Pool() *pgxpool.Pool { return s.pool }

func (s *PostgresStore) CreateTenant(ctx context.Context, name, slug string) (*Tenant, error) {
	row, err := db.New(s.pool).CreateTenant(ctx, db.CreateTenantParams{Name: name, Slug: slug})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &Tenant{ID: row.ID, Name: row.Name, Slug: row.Slug, IsActive: row.IsActive}, nil
}

func (s *PostgresStore) GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error) {
	row, err := db.New(s.pool).GetTenantBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("tenant not found")
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &Tenant{ID: row.ID, Name: row.Name, Slug: row.Slug, IsActive: row.IsActive}, nil
}

func (s *PostgresStore) GetTenantByID(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	row, err := db.New(s.pool).GetTenantByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("tenant not found")
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &Tenant{ID: row.ID, Name: row.Name, Slug: row.Slug, IsActive: row.IsActive}, nil
}

func (s *PostgresStore) ListTenants(ctx context.Context) ([]Tenant, error) {
	rows, err := db.New(s.pool).ListTenants(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]Tenant, 0, len(rows))
	for _, r := range rows {
		out = append(out, Tenant{ID: r.ID, Name: r.Name, Slug: r.Slug, IsActive: r.IsActive})
	}
	return out, nil
}

func (s *PostgresStore) SetTenantActive(ctx context.Context, id uuid.UUID, active bool) error {
	if err := db.New(s.pool).SetTenantActive(ctx, db.SetTenantActiveParams{ID: id, IsActive: active}); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// ListTenantsForUser returns clinics where the user has an explicit
// staff/doctor binding (user_tenants). Patients browse all active tenants.
func (s *PostgresStore) TenantsForUser(ctx context.Context, userID uuid.UUID) ([]Tenant, error) {
	rows, err := db.New(s.pool).ListTenantsForUser(ctx, userID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]Tenant, 0, len(rows))
	for _, r := range rows {
		out = append(out, Tenant{ID: r.ID, Name: r.Name, Slug: r.Slug, IsActive: r.IsActive})
	}
	return out, nil
}

// ScopedProfileStore reads/writes the profiles table inside one tenant
// schema. Every method resolves the schema from the request context.
type ScopedProfileStore struct {
	scoped *database.ScopedPool
}

func NewScopedProfileStore(pool *pgxpool.Pool) *ScopedProfileStore {
	return &ScopedProfileStore{scoped: database.NewScopedPool(pool)}
}

// RoleForUser returns the caller's role in the active tenant. Empty string
// means no profile exists yet (treated as patient-level access).
func (s *ScopedProfileStore) RoleForUser(ctx context.Context, userID uuid.UUID) (string, error) {
	var role string
	err := s.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		row, err := db.New(tx).GetProfileForTenant(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // no profile -> anonymous patient level
		}
		if err != nil {
			return apperr.Internal(err)
		}
		if !row.IsActive {
			return apperr.Forbidden("your profile at this clinic is deactivated")
		}
		role = row.Role
		return nil
	})
	return role, err
}

// EnsurePatientProfile auto-provisions the patient profile on first action.
func (s *ScopedProfileStore) EnsurePatientProfile(ctx context.Context, userID uuid.UUID) error {
	return s.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		_, err := db.New(tx).UpsertPatientProfile(ctx, userID)
		return apperr.Internal(err)
	})
}

// AddStaffBinding registers a user_tenants row so the user's clinic list at
// login includes this tenant (staff/doctor/admin only).
func (s *PostgresStore) AddStaffBinding(ctx context.Context, userID, tenantID uuid.UUID) error {
	_, err := db.New(s.pool).AddUserTenant(ctx, db.AddUserTenantParams{UserID: userID, TenantID: tenantID})
	return apperr.Internal(err)
}
