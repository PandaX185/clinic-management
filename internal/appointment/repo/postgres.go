package repo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/appointment/service"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/database"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
)

const idempotencyEndpoint = "POST /api/v1/appointments"

type PostgresRepository struct {
	scoped *database.ScopedPool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{scoped: database.NewScopedPool(pool)}
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*service.Appointment, error) {
	var out *service.Appointment
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

func (r *PostgresRepository) List(ctx context.Context, query service.ListQuery) ([]service.Appointment, int64, error) {
	var items []service.Appointment
	var total int64

	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		q := db.New(tx)

		filter := query.PatientID
		if filter == "" {
			filter = query.DoctorID
		}
		profileID := nilUUIDPtr(filter)
		status := query.Status

		var e error
		total, e = q.CountAppointments(ctx, db.CountAppointmentsParams{
			Column1: derefUUIDPtr(profileID),
			Column2: status,
		})
		if e != nil {
			return apperr.Internal(e)
		}

		rows, e := q.ListAppointments(ctx, db.ListAppointmentsParams{
			Column1: derefUUIDPtr(profileID),
			Column2: status,
			Limit:   int32(query.Limit),
			Offset:  int32(query.Offset),
		})
		if e != nil {
			return apperr.Internal(e)
		}

		items = make([]service.Appointment, 0, len(rows))
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

func (r *PostgresRepository) BookTx(ctx context.Context, p service.BookTxParams) (service.BookingResult, error) {
	var result service.BookingResult
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
			return service.BookingResult{}, be.err
		}
		return service.BookingResult{}, apperr.Internal(err)
	}
	return result, nil
}

type bookingError struct{ err error }

func (e *bookingError) Error() string { return e.err.Error() }

func bookInTx(ctx context.Context, tx pgx.Tx, p service.BookTxParams) (service.BookingResult, error) {
	// No Rollback here: the caller (ScopedPool.WithSchema) owns the
	// transaction lifecycle and commits/rolls back on return.
	q := db.New(tx)

	if p.PatientUser != nil {
		_, err := q.UpsertPatientProfile(ctx, db.UpsertPatientProfileParams{
			UserID:      *p.PatientUser,
			DisplayName: "Patient",
		})
		if err != nil {
			return service.BookingResult{}, wrapBooking(apperr.Internal(err))
		}
	}

	if p.IdempotencyKey != "" {
		stored, err := q.GetIdempotentResponse(ctx, db.GetIdempotentResponseParams{
			Key:      p.IdempotencyKey,
			Endpoint: idempotencyEndpoint,
		})
		switch {
		case err == nil:
			if stored.UserID != nil && p.CreatedBy != nil && *stored.UserID != *p.CreatedBy {
				return service.BookingResult{}, apperr.Conflict("idempotency key was already used by another request")
			}
			if stored.RequestHash != "" && stored.RequestHash != p.RequestHash {
				return service.BookingResult{}, apperr.Conflict("idempotency key was already used with a different request body")
			}
			return service.BookingResult{Replayed: true, StoredStatus: int(stored.ResponseStatus), StoredBody: rawJSON(stored.ResponseBody)}, nil
		case !errors.Is(err, pgx.ErrNoRows):
			return service.BookingResult{}, wrapBooking(apperr.Internal(err))
		}
	}

	var appointmentTypeID uuid.UUID
	if p.AppointmentTypeID != nil {
		appointmentTypeID = *p.AppointmentTypeID
	} else {
		apptTypes, err := q.ListAppointmentTypes(ctx)
		if err != nil || len(apptTypes) == 0 {
			return service.BookingResult{}, wrapBooking(apperr.Internal(err))
		}
		appointmentTypeID = apptTypes[0].ID
	}

	created, err := q.CreateAppointment(ctx, db.CreateAppointmentParams{
		ProfileID:         p.PatientID,
		DoctorProfileID:   p.DoctorID,
		AppointmentTypeID: appointmentTypeID,
		ScheduledStart:    p.StartTime,
		ScheduledEnd:      p.EndTime,
		Status:            "scheduled",
		VisitNotes:        pgTextFromString(p.Notes),
		Version:           1,
		CreatedBy:         p.CreatedBy,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
			return service.BookingResult{}, wrapBooking(apperr.Conflict("the requested slot is no longer available"))
		}
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return service.BookingResult{}, wrapBooking(apperr.Invalid("unknown patient or doctor"))
		}
		return service.BookingResult{}, wrapBooking(apperr.Internal(err))
	}

	appt := fromRow(created)

	if p.IdempotencyKey != "" {
		body, _ := json.Marshal(appt)
		raw := json.RawMessage(body)
		err := q.InsertIdempotentResponse(ctx, db.InsertIdempotentResponseParams{
			Key:            p.IdempotencyKey,
			Endpoint:       idempotencyEndpoint,
			UserID:         p.CreatedBy,
			RequestHash:    p.RequestHash,
			ResponseStatus: 201,
			ResponseBody:   &raw,
			ExpiresAt:      time.Now().Add(time.Duration(p.TTLSeconds) * time.Second),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return service.BookingResult{}, apperr.Conflict("request with this idempotency key is already being processed")
			}
			return service.BookingResult{}, apperr.Internal(err)
		}
	}

	return service.BookingResult{Appointment: appt}, nil
}

func wrapBooking(err error) error {
	return &bookingError{err: err}
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

func fromRow(a db.Appointment) *service.Appointment {
	out := &service.Appointment{
		ID:                 a.ID,
		PatientID:          a.ProfileID,
		DoctorID:           a.DoctorProfileID,
		AppointmentTypeID:  a.AppointmentTypeID,
		StartTime:          a.ScheduledStart,
		EndTime:            a.ScheduledEnd,
		Status:             service.Status(a.Status),
		Notes:              pgTextToString(a.VisitNotes),
		CancellationReason: pgTextToString(a.CancellationReason),
		Version:            a.Version,
		CreatedAt:          a.CreatedAt,
		UpdatedAt:          a.UpdatedAt,
	}
	if a.CreatedBy != nil {
		out.CreatedBy = *a.CreatedBy
	}
	return out
}

func derefUUIDPtr(p *uuid.UUID) uuid.UUID {
	if p == nil {
		return uuid.Nil
	}
	return *p
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

func nilPgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: s, Valid: true}
}

func pgTextToString(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func nilTime(t *time.Time) *time.Time {
	return t
}

func (r *PostgresRepository) Transition(ctx context.Context, p service.TransitionParams) (*service.Appointment, error) {
	var out *service.Appointment
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		if p.NewStartTime != nil && p.NewEndTime != nil {
			row, err := db.New(tx).RescheduleAppointment(ctx, db.RescheduleAppointmentParams{
				ID:             p.ID,
				ScheduledStart: *p.NewStartTime,
				ScheduledEnd:   *p.NewEndTime,
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
		row, err := db.New(tx).TransitionAppointmentStatus(ctx, db.TransitionAppointmentStatusParams{
			ID:                 p.ID,
			Status:             string(p.NewStatus),
			CancellationReason: pgTextFromString(reasonPtr),
			Status_2:           string(p.ExpectedStatus),
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

func pgTextFromString(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}
