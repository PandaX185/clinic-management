package patient

import (
	"context"
	"errors"

	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/PandaX185/clinic-management/internal/platform/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

// PostgresRepository reads/writes patients inside the tenant schema named
// by the request context (see server.TenantMiddleware).
type PostgresRepository struct {
	scoped *database.ScopedPool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{scoped: database.NewScopedPool(pool)}
}

func (r *PostgresRepository) Create(ctx context.Context, in CreateInput) (*Patient, error) {
	var out *Patient
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		p, err := db.New(tx).CreatePatient(ctx, db.CreatePatientParams{
			FullName:              in.FullName,
			DateOfBirth:           in.DateOfBirth,
			Gender:                in.Gender,
			Phone:                 in.Phone,
			Address:               in.Address,
			EmergencyContactName:  in.EmergencyContactName,
			EmergencyContactPhone: in.EmergencyContactPhone,
			MedicalNotes:          in.MedicalNotes,
		})
		if err != nil {
			return apperr.Internal(err)
		}
		out = fromRow(p)
		return nil
	})
	return out, err
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Patient, error) {
	var out *Patient
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		p, err := db.New(tx).GetPatientByID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("patient not found")
		}
		if err != nil {
			return apperr.Internal(err)
		}
		out = fromRow(p)
		return nil
	})
	return out, err
}

func (r *PostgresRepository) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (*Patient, error) {
	if _, err := r.GetByID(ctx, id); err != nil {
		return nil, err
	}
	var out *Patient
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		p, err := db.New(tx).UpdatePatient(ctx, db.UpdatePatientParams{
			ID:                    id,
			FullName:              in.FullName,
			Gender:                in.Gender,
			Phone:                 in.Phone,
			Address:               in.Address,
			EmergencyContactName:  in.EmergencyContactName,
			EmergencyContactPhone: in.EmergencyContactPhone,
		})
		if err != nil {
			return apperr.Internal(err)
		}
		out = fromRow(p)
		return nil
	})
	return out, err
}

func (r *PostgresRepository) List(ctx context.Context, q ListQuery) ([]Patient, int64, error) {
	var (
		items    []Patient
		total    int64
	)
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		qq := db.New(tx)
		t, err := qq.CountPatients(ctx, q.Search)
		if err != nil {
			return apperr.Internal(err)
		}
		rows, err := qq.SearchPatients(ctx, db.SearchPatientsParams{
			Search: q.Search,
			Limit:  int32(q.Limit),
			Offset: int32(q.Offset),
		})
		if err != nil {
			return apperr.Internal(err)
		}
		items = make([]Patient, 0, len(rows))
		for i := range rows {
			items = append(items, *fromRow(rows[i]))
		}
		total = t
		return nil
	})
	return items, total, err
}

func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) (err error) {
	err = r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		rows, err := db.New(tx).DeletePatient(ctx, id)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return apperr.Conflict("patient has appointment history and cannot be deleted")
			}
			return apperr.Internal(err)
		}
		if rows == 0 {
			return apperr.NotFound("patient not found")
		}
		return nil
	})
	return err
}

func fromRow(p db.Patient) *Patient {
	return &Patient{
		ID:                    p.ID,
		UserID:                p.UserID,
		FullName:              p.FullName,
		DateOfBirth:           p.DateOfBirth,
		Gender:                p.Gender,
		Phone:                 p.Phone,
		Address:               p.Address,
		EmergencyContactName:  p.EmergencyContactName,
		EmergencyContactPhone: p.EmergencyContactPhone,
		MedicalNotes:          p.MedicalNotes,
		CreatedAt:             p.CreatedAt,
		UpdatedAt:             p.UpdatedAt,
	}
}
