package wiring

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/clinic/repo"
	idsvc "github.com/PandaX185/clinic-management/internal/identity/service"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/database"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
)

// clinicMembershipProvider implements identity.Service's membership port using
// the global user_tenants index plus per-tenant role resolution. It lives in
// the wiring package so the identity feature never imports clinic (which
// already imports identity for its handler).
type clinicMembershipProvider struct {
	pool  *pgxpool.Pool
	store *repo.PostgresStore
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

// primaryRole resolves the user's first role inside the clinic's schema.
func (p *clinicMembershipProvider) primaryRole(ctx context.Context, userID uuid.UUID, slug string) (string, error) {
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
