package patient

import (
	"context"
	"errors"

	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

type PostgresRepository struct {
	q *db.Queries
}

func NewPostgresRepository(pool db.DBTX) *PostgresRepository {
	return &PostgresRepository{q: db.New(pool)}
}

func (r *PostgresRepository) Create(ctx context.Context, in CreateInput) (*Patient, error) {
	p, err := r.q.CreatePatient(ctx, db.CreatePatientParams{
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
		return nil, apperr.Internal(err)
	}
	return fromRow(p), nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Patient, error) {
	p, err := r.q.GetPatientByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("patient not found")
		}
		return nil, apperr.Internal(err)
	}
	return fromRow(p), nil
}

func (r *PostgresRepository) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (*Patient, error) {
	if _, err := r.GetByID(ctx, id); err != nil {
		return nil, err
	}
	p, err := r.q.UpdatePatient(ctx, db.UpdatePatientParams{
		ID:                    id,
		FullName:              in.FullName,
		Gender:                in.Gender,
		Phone:                 in.Phone,
		Address:               in.Address,
		EmergencyContactName:  in.EmergencyContactName,
		EmergencyContactPhone: in.EmergencyContactPhone,
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return fromRow(p), nil
}

func (r *PostgresRepository) List(ctx context.Context, q ListQuery) ([]Patient, int64, error) {
	total, err := r.q.CountPatients(ctx, q.Search)
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	rows, err := r.q.SearchPatients(ctx, db.SearchPatientsParams{
		Search: q.Search,
		Limit:  int32(q.Limit),
		Offset: int32(q.Offset),
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	patients := make([]Patient, 0, len(rows))
	for i := range rows {
		patients = append(patients, *fromRow(rows[i]))
	}
	return patients, total, nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	rows, err := r.q.DeletePatient(ctx, id)
	if err != nil {
		return apperr.Internal(err)
	}
	if rows == 0 {
		return apperr.NotFound("patient not found")
	}
	return nil
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
