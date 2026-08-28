package tenant

import (
	"context"
	"errors"

	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }

func (s *PostgresStore) Pool() *pgxpool.Pool { return s.pool }

func (s *PostgresStore) CreateTenant(ctx context.Context, name, slug string) (*Tenant, error) {
	row, err := db.New(s.pool).CreateTenant(ctx, db.CreateTenantParams{Name: name, Slug: slug})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &Tenant{ID: row.ID, Name: row.Name, Slug: row.Slug, IsActive: row.Status == "active"}, nil
}

func (s *PostgresStore) GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error) {
	row, err := db.New(s.pool).GetTenantBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("tenant not found")
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &Tenant{ID: row.ID, Name: row.Name, Slug: row.Slug, IsActive: row.Status == "active"}, nil
}

func (s *PostgresStore) GetTenantByID(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	row, err := db.New(s.pool).GetTenantByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("tenant not found")
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &Tenant{ID: row.ID, Name: row.Name, Slug: row.Slug, IsActive: row.Status == "active"}, nil
}

func (s *PostgresStore) ListTenants(ctx context.Context) ([]Tenant, error) {
	rows, err := db.New(s.pool).ListTenants(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]Tenant, 0, len(rows))
	for _, r := range rows {
		out = append(out, Tenant{ID: r.ID, Name: r.Name, Slug: r.Slug, IsActive: r.Status == "active"})
	}
	return out, nil
}

func (s *PostgresStore) SetTenantActive(ctx context.Context, id uuid.UUID, active bool) error {
	status := "active"
	if !active {
		status = "inactive"
	}
	if err := db.New(s.pool).SetTenantActive(ctx, db.SetTenantActiveParams{ID: id, Status: status}); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// TenantsForUser returns all active tenants. In v2, which tenants a user
// belongs to is determined by ProfileStore queries (tenant-scoped profiles).
// This method returns all tenants so the caller can filter by profile.
func (s *PostgresStore) TenantsForUser(ctx context.Context, userID uuid.UUID) ([]Tenant, error) {
	rows, err := db.New(s.pool).ListTenants(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]Tenant, 0, len(rows))
	for _, r := range rows {
		out = append(out, Tenant{ID: r.ID, Name: r.Name, Slug: r.Slug, IsActive: r.Status == "active"})
	}
	return out, nil
}

// AddStaffBinding is a no-op for now. In v2, profile creation happens in
// the tenant schema (ProfileStore), not via a global user_tenants table.
// The tenant.Service.BindStaff method handles validation and profile creation.
func (s *PostgresStore) AddStaffBinding(ctx context.Context, userID, tenantID uuid.UUID) error {
	return nil
}