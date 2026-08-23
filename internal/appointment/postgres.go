package appointment

import (
	"context"
	"encoding/json"
	"errors"

	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

const idempotencyEndpoint = "POST /api/v1/appointments"

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Appointment, error) {
	q := db.New(r.pool)
	row, err := q.GetAppointmentByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("appointment not found")
		}
		return nil, apperr.Internal(err)
	}
	return fromRow(row), nil
}

func (r *PostgresRepository) List(ctx context.Context, query ListQuery) ([]Appointment, int64, error) {
	params := db.CountAppointmentsParams{
		PatientID: nilUUIDPtr(query.PatientID),
		DoctorID:  nilUUIDPtr(query.DoctorID),
		Status:    nilText(query.Status),
	}
	if query.From != nil {
		from := *query.From
		params.FromTime = &from
	}
	if query.To != nil {
		to := *query.To
		params.ToTime = &to
	}

	q := db.New(r.pool)
	total, err := q.CountAppointments(ctx, params)
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}

	rows, err := q.ListAppointments(ctx, db.ListAppointmentsParams{
		PatientID: params.PatientID,
		DoctorID:  params.DoctorID,
		Status:    params.Status,
		FromTime:  params.FromTime,
		ToTime:    params.ToTime,
		Limit:     int32(query.Limit),
		Offset:    int32(query.Offset),
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}

	items := make([]Appointment, 0, len(rows))
	for i := range rows {
		items = append(items, *fromRow(rows[i]))
	}
	return items, total, nil
}

// BookTx performs the entire booking workflow inside one transaction:
//
//  1. replay check against idempotency_keys (BR-07 / FR-APT-07)
//  2. appointment insert — the no_overlapping_appointments exclusion
//     constraint rejects concurrent double-booking at the DB level (FR-APT-06)
//  3. persist the response under the idempotency key before commit
//
// A retried request with the same key either replays the stored response or,
// if it races the original, serializes behind it via row-level locking on the
// primary key and replays after commit.
func (r *PostgresRepository) BookTx(ctx context.Context, p BookTxParams) (BookingResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return BookingResult{}, apperr.Internal(err)
	}
	defer tx.Rollback(ctx)

	q := db.New(tx)

	if p.IdempotencyKey != "" {
		stored, err := q.GetIdempotentResponse(ctx, db.GetIdempotentResponseParams{
			Key:      p.IdempotencyKey,
			Endpoint: idempotencyEndpoint,
		})
		switch {
		case err == nil:
			return BookingResult{Replayed: true, StoredStatus: int(stored.ResponseStatus), StoredBody: rawJSON(stored.ResponseBody)}, nil
		case !errors.Is(err, pgx.ErrNoRows):
			return BookingResult{}, apperr.Internal(err)
		}
	}

	created, err := q.CreateAppointment(ctx, db.CreateAppointmentParams{
		PatientID: p.PatientID,
		DoctorID:  p.DoctorID,
		StartTime: p.StartTime,
		EndTime:   p.EndTime,
		Notes:     p.Notes,
		CreatedBy: p.CreatedBy,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
			return BookingResult{}, apperr.Conflict("the requested slot is no longer available")
		}
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return BookingResult{}, apperr.Invalid("unknown patient or doctor")
		}
		return BookingResult{}, apperr.Internal(err)
	}

	appt := fromRow(created)

	if p.IdempotencyKey != "" {
		body, _ := json.Marshal(appt)
		raw := json.RawMessage(body)
		if err := q.InsertIdempotentResponse(ctx, db.InsertIdempotentResponseParams{
			Key:            p.IdempotencyKey,
			Endpoint:       idempotencyEndpoint,
			UserID:         p.CreatedBy,
			RequestHash:    p.RequestHash,
			ResponseStatus: 201,
			ResponseBody:   &raw,
			TtlSeconds:     int32(p.TTLSeconds),
		}); err != nil {
			return BookingResult{}, apperr.Internal(err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
			return BookingResult{}, apperr.Conflict("the requested slot is no longer available")
		}
		return BookingResult{}, apperr.Internal(err)
	}
	return BookingResult{Appointment: appt}, nil
}

func rawJSON(v any) []byte {
	if v == nil {
		return nil
	}
	if raw, ok := v.(json.RawMessage); ok {
		return raw
	}
	b, _ := json.Marshal(v)
	return b
}

func fromRow(a db.Appointment) *Appointment {
	return &Appointment{
		ID:                 a.ID,
		PatientID:          a.PatientID,
		DoctorID:           a.DoctorID,
		StartTime:          a.StartTime,
		EndTime:            a.EndTime,
		Status:             Status(a.Status),
		Notes:              a.Notes,
		CancellationReason: a.CancellationReason,
		Version:            a.Version,
		CreatedBy:          a.CreatedBy,
		CreatedAt:          a.CreatedAt,
		UpdatedAt:          a.UpdatedAt,
	}
}

func nilUUIDPtr(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}

func nilText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (r *PostgresRepository) Transition(ctx context.Context, p TransitionParams) (*Appointment, error) {
	if p.NewStartTime != nil && p.NewEndTime != nil {
		q := db.New(r.pool)
		row, err := q.RescheduleAppointment(ctx, db.RescheduleAppointmentParams{
			ID:        p.ID,
			StartTime: *p.NewStartTime,
			EndTime:   *p.NewEndTime,
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
				return nil, apperr.Conflict("the new time slot is no longer available")
			}
			return nil, apperr.Internal(err)
		}
		return fromRow(row), nil
	}

	q := db.New(r.pool)
	var reasonPtr *string
	if p.CancellationReason != nil {
		r := *p.CancellationReason
		reasonPtr = &r
	}
	row, err := q.TransitionAppointmentStatus(ctx, db.TransitionAppointmentStatusParams{
		ID:                 p.ID,
		Status:             string(p.NewStatus),
		CancellationReason: reasonPtr,
		ExpectedStatus:     string(p.ExpectedStatus),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.Conflict("appointment state changed concurrently; retry")
		}
		return nil, apperr.Internal(err)
	}
	return fromRow(row), nil
}
