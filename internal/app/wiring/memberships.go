package wiring

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/lahza/internal/clinic/repo"
	idsvc "github.com/PandaX185/lahza/internal/identity/service"
	"github.com/PandaX185/lahza/internal/platform/database"
)

// clinicMembershipProvider implements identity.Service's membership port using
// the global user_tenants index plus per-tenant role resolution. It lives in
// the wiring package so the identity feature never imports clinic (which
// already imports identity for its handler). Role resolution is delegated to
// the same ProfileStore the middleware uses, so there is a single source of
// truth for "a user's role in a clinic schema".
type clinicMembershipProvider struct {
	pool     *pgxpool.Pool
	store    *repo.PostgresStore
	profiles *repo.PostgresProfileStore
}

// MembershipsForUser returns the clinics the user is a member of with their
// primary role in each. Users with no membership get an empty list, matching
// the /clinics/mine behaviour.
func (p *clinicMembershipProvider) MembershipsForUser(ctx context.Context, userID uuid.UUID) ([]idsvc.UserClinic, error) {
	clinics, err := p.store.ClinicsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(clinics) == 0 {
		return []idsvc.UserClinic{}, nil
	}

	out := make([]idsvc.UserClinic, 0, len(clinics))
	for _, c := range clinics {
		role, err := p.primaryRole(ctx, userID, c.Slug)
		if err != nil {
			return nil, err
		}
		out = append(out, idsvc.UserClinic{
			ClinicID:   c.ID,
			ClinicName: c.Name,
			ClinicSlug: c.Slug,
			RoleName:   role,
		})
	}
	return out, nil
}

// primaryRole resolves the user's first role inside the clinic's schema by
// reusing the shared profile store. A user with a profile but no roles, or no
// profile at all, resolves to an empty role (patient-level visitor).
func (p *clinicMembershipProvider) primaryRole(ctx context.Context, userID uuid.UUID, slug string) (string, error) {
	scoped := database.WithTenantSlug(ctx, slug)
	roles, err := p.profiles.RoleForUser(scoped, userID)
	if err != nil {
		return "", err
	}
	if len(roles) == 0 {
		return "", nil
	}
	return roles[0], nil
}
