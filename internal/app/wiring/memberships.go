package wiring

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/auth"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/database"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/PandaX185/clinic-management/internal/tenant"
)

// tenantMembershipProvider implements auth.TenantMembershipProvider using the
// global user_tenants index plus per-tenant role resolution. It lives in the
// wiring package so the auth package never imports tenant (which already
// imports auth for its handler).
type tenantMembershipProvider struct {
	pool  *pgxpool.Pool
	store *tenant.PostgresStore
}

// MembershipsForUser returns the clinics the user is a member of with their
// primary role in each. Users with no membership get an empty list, matching
// the /tenants/mine behaviour.
func (p *tenantMembershipProvider) MembershipsForUser(ctx context.Context, userID uuid.UUID) ([]auth.UserTenant, error) {
	tenants, err := p.store.TenantsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(tenants) == 0 {
		return []auth.UserTenant{}, nil
	}

	out := make([]auth.UserTenant, 0, len(tenants))
	for _, t := range tenants {
		role, err := p.primaryRole(ctx, userID, t.Slug)
		if err != nil {
			return nil, err
		}
		out = append(out, auth.UserTenant{
			TenantID:   t.ID,
			TenantName: t.Name,
			TenantSlug: t.Slug,
			RoleName:   role,
		})
	}
	return out, nil
}

// primaryRole resolves the user's first role inside the tenant's schema.
func (p *tenantMembershipProvider) primaryRole(ctx context.Context, userID uuid.UUID, slug string) (string, error) {
	var role string
	err := database.NewScopedPool(p.pool).WithSchema(ctx, slug, func(tx pgx.Tx) error {
		profile, err := db.New(tx).GetProfileByUserID(ctx, userID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil // no profile in this clinic → patient-level visitor
			}
			return err
		}
		rows, err := db.New(tx).ListUserRoles(ctx, profile.ID)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.Name != "" {
				role = r.Name
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return "", apperr.Internal(err)
	}
	return role, nil
}
