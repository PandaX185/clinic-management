package appointment

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/PandaX185/clinic-management/internal/platform/database"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

const idempotencyEndpoint = "POST /api/v1/appointments"

type PostgresRepository struct {
	scoped *database.ScopedPool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{scoped: database.NewScopedPool(pool)}
}

// GetByID reads one appointment inside the tenant schema resolved from the
// request context (set by server.TenantMiddleware). A missing tenant scope
// is a programming error and fails closed.
func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Appointment, error) {
	var out *Appointment
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		row, err := db.New(tx).GetAppointmentByID(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.NotFound("appointment not found")
			}
			return apperr.Internal(err)
		}
		out = fromRow(row)
		return nil
	})
	return out, err
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

	var items []Appointment
	var total int64
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		total, err = q.CountAppointments(ctx, params)
		if err != nil {
			return apperr.Internal(err)
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
			return apperr.Internal(err)
		}

		items = make([]Appointment, 0, len(rows))
		for i := range rows {
			items = append(items, *fromRow(rows[i]))
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
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
	var result BookingResult
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		res, err := bookInTx(ctx, tx, p)
		if err != nil {
			return err
		}
		result = res
		return nil
	})
	if err != nil {
		if be, ok := err.(*bookingError); ok {
			return BookingResult{}, be.err
		}
		return BookingResult{}, apperr.Internal(err)
	}
	return result, nil
}

// bookingError wraps an apperr so BookTx can pass domain errors (409/400)
// through the WithSchema closure untouched.
type bookingError struct{ err error }

func (e *bookingError) Error() string { return e.err.Error() }

func bookInTx(ctx context.Context, tx pgx.Tx, p BookTxParams) (BookingResult, error) {
	defer tx.Rollback(ctx)

	q := db.New(tx)

	// Auto-provision the patient's profile in THIS clinic on first action
	// (patients are global users; clinical records stay per-tenant).
	if p.PatientUser != nil {
		if _, err := q.UpsertPatientProfile(ctx, *p.PatientUser); err != nil {
			return BookingResult{}, wrapBooking(apperr.Internal(err))
		}
	}

	if p.IdempotencyKey != "" {
		stored, err := q.GetIdempotentResponse(ctx, db.GetIdempotentResponseParams{
			Key:      p.IdempotencyKey,
			Endpoint: idempotencyEndpoint,
		})
		switch {
		case err == nil:
			// SEC-03: a key belongs to the user that created it and must
			// match the request payload; anything else is a conflict, never
			// a replay of someone else's booking.
			if stored.UserID != nil && p.CreatedBy != nil && *stored.UserID != *p.CreatedBy {
				return BookingResult{}, apperr.Conflict("idempotency key was already used by another request")
			}
			if stored.RequestHash != "" && stored.RequestHash != p.RequestHash {
				return BookingResult{}, apperr.Conflict("idempotency key was already used with a different request body")
			}
			return BookingResult{Replayed: true, StoredStatus: int(stored.ResponseStatus), StoredBody: rawJSON(stored.ResponseBody)}, nil
		case !errors.Is(err, pgx.ErrNoRows):
			return BookingResult{}, wrapBooking(apperr.Internal(err))
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
			return BookingResult{}, wrapBooking(apperr.Conflict("the requested slot is no longer available"))
		}
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return BookingResult{}, wrapBooking(apperr.Invalid("unknown patient or doctor"))
		}
		return BookingResult{}, wrapBooking(apperr.Internal(err))
	}

	appt := fromRow(created)

	if p.IdempotencyKey != "" {
		body, _ := json.Marshal(appt)
		raw := json.RawMessage(body)
		// DO-UPDATE-WHERE-false RETURNING: on a conflicting commit the
		// statement returns no rows (pgx.ErrNoRows) — another transaction
		// owns this key already, so abort instead of duplicating (BR-07).
		if _, err := q.InsertIdempotentResponse(ctx, db.InsertIdempotentResponseParams{
			Key:            p.IdempotencyKey,
			Endpoint:       idempotencyEndpoint,
			UserID:         p.CreatedBy,
			RequestHash:    p.RequestHash,
			ResponseStatus: 201,
			ResponseBody:   &raw,
			TtlSeconds:     int32(p.TTLSeconds),
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return BookingResult{}, apperr.Conflict("request with this idempotency key is already being processed")
			}
			return BookingResult{}, apperr.Internal(err)
		}
	}

	return BookingResult{Appointment: appt}, nil
}

func wrapBooking(err error) error { return &bookingError{err: err} }

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
	var out *Appointment
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		q := db.New(tx)
		if p.NewStartTime != nil && p.NewEndTime != nil {
			row, err := q.RescheduleAppointment(ctx, db.RescheduleAppointmentParams{
				ID:        p.ID,
				StartTime: *p.NewStartTime,
				EndTime:   *p.NewEndTime,
			})
			if err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
					return apperr.Conflict("the new time slot is no longer available")
				}
				if errors.Is(err, pgx.ErrNoRows) {
					return apperr.Conflict("appointment state changed concurrently; retry")
				}
				return apperr.Internal(err)
			}
			out = fromRow(row)
			return nil
		}

		var reasonPtr *string
		if p.CancellationReason != nil {
			reason := *p.CancellationReason
			reasonPtr = &reason
		}
		row, err := q.TransitionAppointmentStatus(ctx, db.TransitionAppointmentStatusParams{
			ID:                 p.ID,
			Status:             string(p.NewStatus),
			CancellationReason: reasonPtr,
			ExpectedStatus:     string(p.ExpectedStatus),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.Conflict("appointment state changed concurrently; retry")
			}
			return apperr.Internal(err)
		}
		out = fromRow(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
