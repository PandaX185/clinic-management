// Package service owns the tenant application boundary: domain types, the
// persistence ports the service depends on, and the clinic lifecycle use
// cases (create/list/browse/bind-staff). HTTP layer (api) and persistence
// adapters (repo) both depend on this package, never the other way around.
package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolProvider provides access to a pgx connection pool.
type PoolProvider interface {
	Pool() *pgxpool.Pool
}

type Tenant struct {
	ID       uuid.UUID
	Name     string
	Slug     string
	IsActive bool
}

// Store is the global-registry surface (tenants, user_tenants bindings).
type Store interface {
	CreateTenant(ctx context.Context, name, slug string) (*Tenant, error)
	GetTenantByID(ctx context.Context, id uuid.UUID) (*Tenant, error)
	ListTenants(ctx context.Context) ([]Tenant, error)
	SetTenantActive(ctx context.Context, id uuid.UUID, active bool) error
	TenantsForUser(ctx context.Context, userID uuid.UUID) ([]Tenant, error)
	RecordMembership(ctx context.Context, userID, tenantID uuid.UUID) error
	Pool() *pgxpool.Pool
}

// ProfileStore resolves per-tenant roles inside the active clinic's schema.
type ProfileStore interface {
	// RoleForUser returns the caller's role in the active tenant; empty
	// means no profile yet (patient-level access).
	RoleForUser(ctx context.Context, userID uuid.UUID) ([]string, error)
	EnsurePatientProfile(ctx context.Context, userID uuid.UUID) error
}
