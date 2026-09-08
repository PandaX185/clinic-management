package service

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	idsvc "github.com/PandaX185/lahza/internal/identity/service"
	"github.com/PandaX185/lahza/internal/platform/apperr"
	"github.com/PandaX185/lahza/internal/platform/database"
	db "github.com/PandaX185/lahza/internal/platform/db/sqlc"
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
func (s *Service) Create(ctx context.Context, creatorID uuid.UUID, name, rawSlug string) (*Clinic, error) {
	slug := normalizeSlug(rawSlug)
	if !slugRe.MatchString(slug) {
		return nil, apperr.Invalid("slug must be lowercase letters, digits and underscores, starting with a letter")
	}
	if strings.TrimSpace(name) == "" {
		return nil, apperr.Invalid("name is required")
	}
	c, err := s.store.CreateClinic(ctx, name, slug)
	if err != nil {
		return nil, err
	}
	if err := database.ProvisionTenant(ctx, s.pool.Pool(), slug); err != nil {
		return nil, apperr.Internal(err)
	}
	if err := s.bindRole(ctx, creatorID, c, "admin"); err != nil {
		return nil, err
	}
	if err := s.store.RecordMembership(ctx, creatorID, c.ID); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) List(ctx context.Context) ([]Clinic, error) { return s.store.ListClinics(ctx) }

// ListForUser returns the clinics the user has an explicit membership in.
// A user with no bindings has no clinics; this is used by "my clinics" so it
// must not fall back to the global active-clinic list.
func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID) ([]Clinic, error) {
	bindings, err := s.store.ClinicsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return bindings, nil
}

// SlugForTenant validates the clinic exists and is active. The returned slug
// is the only permitted source of SQL schema identifiers.
func (s *Service) SlugForTenant(ctx context.Context, clinicID uuid.UUID) (string, error) {
	c, err := s.store.GetClinicByID(ctx, clinicID)
	if err != nil {
		return "", err
	}
	if !c.IsActive {
		return "", apperr.Forbidden("this clinic is not active")
	}
	return c.Slug, nil
}

func (s *Service) Deactivate(ctx context.Context, id uuid.UUID) error {
	return s.store.SetClinicActive(ctx, id, false)
}

// BindStaff registers a user within a clinic's profile table with the given
// role, so the clinic shows up in that user's list at login and access checks
// resolve the role from the tenant's profile_roles. The role row must exist
// (seeded by ProvisionTenant); assignment is idempotent.
func (s *Service) BindStaff(ctx context.Context, userID, clinicID uuid.UUID, role string) error {
	c, err := s.store.GetClinicByID(ctx, clinicID)
	if err != nil {
		return err
	}
	if !c.IsActive {
		return apperr.Invalid("clinic is not active")
	}
	if err := s.bindRole(ctx, userID, c, role); err != nil {
		return err
	}
	return s.store.RecordMembership(ctx, userID, clinicID)
}

func (s *Service) bindRole(ctx context.Context, userID uuid.UUID, c *Clinic, role string) error {
	role = strings.ToLower(strings.TrimSpace(role))
	if !idsvc.StandardRoles()[role] {
		return apperr.Invalid("unknown role: " + role)
	}

	err := database.WithTenantSchema(ctx, s.pool.Pool(), c.Slug, func(q db.DBTX) error {
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
