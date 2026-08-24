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

type Membership struct {
	UserID   uuid.UUID
	TenantID uuid.UUID
	RoleName string
	IsActive bool
}

// Store is the global-schema surface for tenants and memberships.
// Implemented over public.users/user_tenants/tenants.
type Store interface {
	CreateTenant(ctx context.Context, name, slug string) (*Tenant, error)
	GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error)
	ListTenants(ctx context.Context) ([]Tenant, error)
	SetTenantActive(ctx context.Context, id uuid.UUID, active bool) error
	AddMember(ctx context.Context, userID, tenantID uuid.UUID, role string) error
	ListMembershipsForUser(ctx context.Context, userID uuid.UUID) ([]Membership, error)
	GetMembership(ctx context.Context, userID, tenantID uuid.UUID) (*Membership, error)
}

type Service struct {
	store Store
	pool  PoolProvider
}

// PoolProvider supplies the raw pool for schema provisioning (separate from
// the Store to keep global-data queries and DDL concerns apart).
type PoolProvider interface {
	Pool() *pgxpool.Pool
}

func NewService(store Store, pool PoolProvider) *Service { return &Service{store: store, pool: pool} }

func normalizeSlug(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, " ", "_")))
}

// Create provisions a new clinic: tenants row + physical schema with all
// clinical tables. Slug is validated before it ever reaches SQL text.
func (s *Service) Create(ctx context.Context, name, rawSlug string) (*Tenant, error) {
	slug := normalizeSlug(rawSlug)
	if !slugRe.MatchString(slug) {
		return nil, apperr.Invalid("slug must be lowercase letters, digits and underscores, starting with a letter")
	}
	if strings.HasPrefix(name, " ") || strings.TrimSpace(name) == "" {
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

func (s *Service) Deactivate(ctx context.Context, id uuid.UUID) error {
	return s.store.SetTenantActive(ctx, id, false)
}

// AddMember grants an existing user a role inside a clinic.
func (s *Service) AddMember(ctx context.Context, userID, tenantID uuid.UUID, role string) error {
	switch role {
	case "admin", "staff", "doctor", "patient":
	default:
		return apperr.Invalid("unknown role")
	}
	return s.store.AddMember(ctx, userID, tenantID, role)
}

// MembershipsOf lists a user's clinic memberships (used at login).
func (s *Service) MembershipsOf(ctx context.Context, userID uuid.UUID) ([]Membership, error) {
	return s.store.ListMembershipsForUser(ctx, userID)
}
