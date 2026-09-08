// Package repo holds the PostgreSQL adapters that satisfy the clinic service
// ports (service.Store and service.ProfileStore). Persistence depends on the
// service boundary, never the other way around.
package repo

import (
	"context"
	"errors"

	db "github.com/PandaX185/lahza/internal/platform/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/lahza/internal/clinic/service"
	"github.com/PandaX185/lahza/internal/platform/apperr"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }

func (s *PostgresStore) Pool() *pgxpool.Pool { return s.pool }

func (s *PostgresStore) CreateClinic(ctx context.Context, name, slug string) (*service.Clinic, error) {
	row, err := db.New(s.pool).CreateClinic(ctx, db.CreateClinicParams{Name: name, Slug: slug})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &service.Clinic{ID: row.ID, Name: row.Name, Slug: row.Slug, IsActive: row.Status == "active"}, nil
}

func (s *PostgresStore) GetClinicByID(ctx context.Context, id uuid.UUID) (*service.Clinic, error) {
	row, err := db.New(s.pool).GetClinicByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("clinic not found")
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &service.Clinic{ID: row.ID, Name: row.Name, Slug: row.Slug, IsActive: row.Status == "active"}, nil
}

func (s *PostgresStore) ListClinics(ctx context.Context) ([]service.Clinic, error) {
	rows, err := db.New(s.pool).ListClinics(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]service.Clinic, 0, len(rows))
	for _, r := range rows {
		out = append(out, service.Clinic{ID: r.ID, Name: r.Name, Slug: r.Slug, IsActive: r.Status == "active"})
	}
	return out, nil
}

func (s *PostgresStore) SetClinicActive(ctx context.Context, id uuid.UUID, active bool) error {
	status := "active"
	if !active {
		status = "inactive"
	}
	if err := db.New(s.pool).SetClinicActive(ctx, db.SetClinicActiveParams{ID: id, Status: status}); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// ClinicsForUser returns the clinics the user has an explicit membership in
// (user_tenants index, maintained as staff are bound to clinics). Users with
// no memberships are patients and get the browse-all behavior from the
// service layer.
func (s *PostgresStore) ClinicsForUser(ctx context.Context, userID uuid.UUID) ([]service.Clinic, error) {
	rows, err := db.New(s.pool).ListUserClinicIDs(ctx, userID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if len(rows) == 0 {
		return []service.Clinic{}, nil
	}
	out := make([]service.Clinic, 0, len(rows))
	for _, tid := range rows {
		c, err := s.GetClinicByID(ctx, tid)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}

// RecordMembership records that userID belongs to the clinic, so the user's
// "my clinics" list reflects real memberships. Idempotent.
func (s *PostgresStore) RecordMembership(ctx context.Context, userID, clinicID uuid.UUID) error {
	if err := db.New(s.pool).EnsureUserClinicMembership(ctx, db.EnsureUserClinicMembershipParams{
		UserID:   userID,
		TenantID: clinicID,
	}); err != nil {
		return apperr.Internal(err)
	}
	return nil
}
