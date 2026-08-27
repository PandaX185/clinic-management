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

// Pool returns the underlying pool for cross-schema operations.
func (s *PostgresStore) Pool() *pgxpool.Pool { return s.pool }

// Tenant CRUD methods
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

// TenantsForUser returns clinics where the user has an explicit
// staff/doctor/admin binding (user_tenants). Patients can only access active clinics.
func (s *PostgresStore) TenantsForUser(ctx context.Context, userID uuid.UUID) ([]Tenant, error) {
	rows, err := db.New(s.pool).ListTenantsForUser(ctx, userID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]Tenant, 0, len(rows))
	for _, r := range rows {
		out = append(out, Tenant{ID: r.ID, Name: r.Name, Slug: r.Slug, IsActive: r.Status == "active"})
	}
	return out, nil
}

// AddStaffBinding registers a user_tenants row so the user's clinic list at
// login includes this tenant (staff/doctor/admin only).
func (s *PostgresStore) AddStaffBinding(ctx context.Context, userID, tenantID uuid.UUID) error {
	_, err := db.New(s.pool).AddUserTenant(ctx, db.AddUserTenantParams{UserID: userID, TenantID: tenantID})
	return apperr.Internal(err)
}