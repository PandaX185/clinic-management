package tenant

import (
	"context"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/database"
)

var slugRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

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
	AddStaffBinding(ctx context.Context, userID, tenantID uuid.UUID) error
	PoolProvider
}

// ProfileStore resolves per-tenant roles inside the active clinic's schema.
type ProfileStore interface {
	// RoleForUser returns the caller's role in the active tenant; empty
	// means no profile yet (patient-level access).
	RoleForUser(ctx context.Context, userID uuid.UUID) (string, error)
	EnsurePatientProfile(ctx context.Context, userID uuid.UUID) error
}

// PoolProvider supplies the raw pool for schema provisioning.
type PoolProvider interface {
	Pool() *pgxpool.Pool
}

type Service struct {
	store   Store
	profile ProfileStore
	pool    PoolProvider
}

func NewService(store Store, profile ProfileStore, pool PoolProvider) *Service {
	return &Service{store: store, profile: profile, pool: pool}
}

func normalizeSlug(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, " ", "_")))
}

// Create provisions a new clinic: tenants row + physical schema.
func (s *Service) Create(ctx context.Context, name, rawSlug string) (*Tenant, error) {
	slug := normalizeSlug(rawSlug)
	if !slugRe.MatchString(slug) {
		return nil, apperr.Invalid("slug must be lowercase letters, digits and underscores, starting with a letter")
	}
	if strings.TrimSpace(name) == "" {
		return nil, apperr.Invalid("name is required")
	}
	t, err := s.store.CreateTenant(ctx, name, slug)
	if err != nil {
		return nil, err
	}
	if err := database.ProvisionTenant(ctx, s.pool.Pool(), slug); err != nil {
		return nil, apperr.Internal(err)
	}
	return t, nil
}

func (s *Service) List(ctx context.Context) ([]Tenant, error) { return s.store.ListTenants(ctx) }

// ListForUser returns clinics relevant to this user: staff bindings for
// employees; every active clinic for patients. A user with no bindings is
// treated as a patient (global browsing).
func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID) ([]Tenant, error) {
	bindings, err := s.store.TenantsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(bindings) > 0 {
		return bindings, nil
	}
	return s.store.ListTenants(ctx)
}

// SlugForTenant validates the tenant exists and is active. The returned slug
// is the only permitted source of SQL schema identifiers.
func (s *Service) SlugForTenant(ctx context.Context, tenantID uuid.UUID) (string, error) {
	t, err := s.store.GetTenantByID(ctx, tenantID)
	if err != nil {
		return "", err
	}
	if !t.IsActive {
		return "", apperr.Forbidden("this clinic is not active")
	}
	return t.Slug, nil
}

func (s *Service) Deactivate(ctx context.Context, id uuid.UUID) error {
	return s.store.SetTenantActive(ctx, id, false)
}

// BindStaff registers a doctor/staff/admin binding so the clinic shows up in
// that user's list at login. The actual role lives in the tenant's profiles
// table and must be provisioned separately by an admin of that clinic.
func (s *Service) BindStaff(ctx context.Context, userID, tenantID uuid.UUID) error {
	t, err := s.store.GetTenantByID(ctx, tenantID)
	if err != nil {
		return err
	}
	if !t.IsActive {
		return apperr.Invalid("tenant is not active")
	}
	return s.store.AddStaffBinding(ctx, userID, tenantID)
}
