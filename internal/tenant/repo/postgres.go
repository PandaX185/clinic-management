// Package repo holds the PostgreSQL adapters that satisfy the tenant service
// ports (service.Store and service.ProfileStore). Persistence depends on the
// service boundary, never the other way around.
package repo

import (
	"context"
	"errors"

	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/tenant/service"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }

func (s *PostgresStore) Pool() *pgxpool.Pool { return s.pool }

func (s *PostgresStore) CreateTenant(ctx context.Context, name, slug string) (*service.Tenant, error) {
	row, err := db.New(s.pool).CreateTenant(ctx, db.CreateTenantParams{Name: name, Slug: slug})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &service.Tenant{ID: row.ID, Name: row.Name, Slug: row.Slug, IsActive: row.Status == "active"}, nil
}

func (s *PostgresStore) GetTenantBySlug(ctx context.Context, slug string) (*service.Tenant, error) {
	row, err := db.New(s.pool).GetTenantBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("tenant not found")
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &service.Tenant{ID: row.ID, Name: row.Name, Slug: row.Slug, IsActive: row.Status == "active"}, nil
}

func (s *PostgresStore) GetTenantByID(ctx context.Context, id uuid.UUID) (*service.Tenant, error) {
	row, err := db.New(s.pool).GetTenantByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("tenant not found")
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &service.Tenant{ID: row.ID, Name: row.Name, Slug: row.Slug, IsActive: row.Status == "active"}, nil
}

func (s *PostgresStore) ListTenants(ctx context.Context) ([]service.Tenant, error) {
	rows, err := db.New(s.pool).ListTenants(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]service.Tenant, 0, len(rows))
	for _, r := range rows {
		out = append(out, service.Tenant{ID: r.ID, Name: r.Name, Slug: r.Slug, IsActive: r.Status == "active"})
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

// TenantsForUser returns the tenants the user has an explicit membership in
// (user_tenants index, maintained as staff are bound to clinics). Users with
// no memberships are patients and get the browse-all behavior from the
// service layer.
func (s *PostgresStore) TenantsForUser(ctx context.Context, userID uuid.UUID) ([]service.Tenant, error) {
	rows, err := db.New(s.pool).ListUserTenantIDs(ctx, userID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if len(rows) == 0 {
		return []service.Tenant{}, nil
	}
	out := make([]service.Tenant, 0, len(rows))
	for _, tid := range rows {
		t, err := s.GetTenantByID(ctx, tid)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, nil
}

// RecordMembership records that userID belongs to the tenant, so the user's
// "my clinics" list reflects real memberships. Idempotent.
func (s *PostgresStore) RecordMembership(ctx context.Context, userID, tenantID uuid.UUID) error {
	if err := db.New(s.pool).EnsureUserTenantMembership(ctx, db.EnsureUserTenantMembershipParams{
		UserID:   userID,
		TenantID: tenantID,
	}); err != nil {
		return apperr.Internal(err)
	}
	return nil
}
