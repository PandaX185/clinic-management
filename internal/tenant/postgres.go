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

// Pool satisfies the Service's PoolProvider.
func (s *PostgresStore) Pool() *pgxpool.Pool { return s.pool }

type poolProviderAdapter struct{ s *PostgresStore }

func (a poolProviderAdapter) Pool() *pgxpool.Pool { return a.s.pool }

var _ PoolProvider = poolProviderAdapter{}

func (s *PostgresStore) PoolProvider() PoolProvider { return poolProviderAdapter{s} }

func (s *PostgresStore) CreateTenant(ctx context.Context, name, slug string) (*Tenant, error) {
	row, err := db.New(s.pool).CreateTenant(ctx, db.CreateTenantParams{Name: name, Slug: slug})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return fromRow(row.ID, row.Name, row.Slug, row.IsActive), nil
}

func (s *PostgresStore) GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error) {
	row, err := db.New(s.pool).GetTenantBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("tenant not found")
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return fromRow(row.ID, row.Name, row.Slug, row.IsActive), nil
}

func (s *PostgresStore) ListTenants(ctx context.Context) ([]Tenant, error) {
	rows, err := db.New(s.pool).ListTenants(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]Tenant, 0, len(rows))
	for _, r := range rows {
		t := fromRow(r.ID, r.Name, r.Slug, r.IsActive)
		out = append(out, *t)
	}
	return out, nil
}

func (s *PostgresStore) SetTenantActive(ctx context.Context, id uuid.UUID, active bool) error {
	if err := db.New(s.pool).SetTenantActive(ctx, db.SetTenantActiveParams{ID: id, IsActive: active}); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

func (s *PostgresStore) AddMember(ctx context.Context, userID, tenantID uuid.UUID, role string) error {
	_, err := db.New(s.pool).AddTenantMember(ctx, db.AddTenantMemberParams{
		UserID:   userID,
		TenantID: tenantID,
		RoleName: role,
	})
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

func (s *PostgresStore) ListMembershipsForUser(ctx context.Context, userID uuid.UUID) ([]Membership, error) {
	rows, err := db.New(s.pool).ListMembershipsForUser(ctx, userID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]Membership, 0, len(rows))
	for _, r := range rows {
		out = append(out, Membership{
			UserID:   r.UserID,
			TenantID: r.TenantID,
			RoleName: r.RoleName,
			IsActive: r.IsActive,
		})
	}
	return out, nil
}

func (s *PostgresStore) GetMembership(ctx context.Context, userID, tenantID uuid.UUID) (*Membership, error) {
	row, err := db.New(s.pool).GetMembership(ctx, db.GetMembershipParams{UserID: userID, TenantID: tenantID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.Forbidden("no active membership in this clinic")
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &Membership{
		UserID:   row.UserID,
		TenantID: row.TenantID,
		RoleName: row.RoleName,
		IsActive: row.IsActive,
	}, nil
}

func fromRow(id uuid.UUID, name, slug string, isActive bool) *Tenant {
	return &Tenant{ID: id, Name: name, Slug: slug, IsActive: isActive}
}
