package service

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/database"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
)

var slugRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

type Service struct {
	store   Store
	profile ProfileStore
	pool    PoolProvider
}

func NewService(store Store, profile ProfileStore, pool PoolProvider) *Service {
	return &Service{store: store, profile: profile, pool: pool}
}

// Create provisions a new clinic: tenants row + physical schema. The
// creating global super-admin is bound as the clinic's first admin so the
// per-clinic admin role exists to onboard further staff.
func (s *Service) Create(ctx context.Context, creatorID uuid.UUID, name, rawSlug string) (*Tenant, error) {
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
	if err := s.bindRole(ctx, creatorID, t, "admin"); err != nil {
		return nil, err
	}
	if err := s.store.RecordMembership(ctx, creatorID, t.ID); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Service) List(ctx context.Context) ([]Tenant, error) { return s.store.ListTenants(ctx) }

// ListForUser returns the clinics the user has an explicit membership in.
// A user with no bindings has no clinics; this is used by "my clinics" so it
// must not fall back to the global active-clinic list.
func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID) ([]Tenant, error) {
	bindings, err := s.store.TenantsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return bindings, nil
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

// standardRoles are the roles seeded into every tenant schema on provision.
var standardRoles = map[string]bool{
	"admin": true, "staff": true, "doctor": true,
	"nurse": true, "manager": true, "patient": true,
}

// BindStaff registers a user within a tenant's profile table with the given
// role, so the clinic shows up in that user's list at login and access checks
// resolve the role from the tenant's profile_roles. The role row must exist
// (seeded by ProvisionTenant); assignment is idempotent.
func (s *Service) BindStaff(ctx context.Context, userID, tenantID uuid.UUID, role string) error {
	t, err := s.store.GetTenantByID(ctx, tenantID)
	if err != nil {
		return err
	}
	if !t.IsActive {
		return apperr.Invalid("tenant is not active")
	}
	if err := s.bindRole(ctx, userID, t, role); err != nil {
		return err
	}
	return s.store.RecordMembership(ctx, userID, tenantID)
}

func (s *Service) bindRole(ctx context.Context, userID uuid.UUID, t *Tenant, role string) error {
	role = strings.ToLower(strings.TrimSpace(role))
	if !standardRoles[role] {
		return apperr.Invalid("unknown role: " + role)
	}

	err := database.WithTenantSchema(ctx, s.pool.Pool(), t.Slug, func(q db.DBTX) error {
		prof, err := db.New(q).UpsertPatientProfile(ctx, db.UpsertPatientProfileParams{
			UserID:      userID,
			DisplayName: role,
		})
		if err != nil {
			return err
		}
		r, err := db.New(q).GetRoleByName(ctx, role)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.Internal(errors.New("role not seeded in tenant schema"))
			}
			return err
		}
		return db.New(q).AssignRoleToProfile(ctx, db.AssignRoleToProfileParams{
			ProfileID: prof.ID,
			RoleID:    r.ID,
		})
	})
	if err != nil {
		return err
	}
	return nil
}

func normalizeSlug(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, " ", "_")))
}
