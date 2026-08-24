package doctor

import (
	"context"
	"errors"
	"time"

	"github.com/PandaX185/clinic-management/internal/platform/database"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

// PostgresRepository reads/writes doctors inside the tenant schema named by
// the request context (see server.TenantMiddleware).
type PostgresRepository struct {
	scoped *database.ScopedPool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{scoped: database.NewScopedPool(pool)}
}

func (r *PostgresRepository) Create(ctx context.Context, in CreateDoctorInput) (*Doctor, error) {
	var out *Doctor
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		d, err := db.New(tx).CreateDoctor(ctx, db.CreateDoctorParams{
			UserID:         in.UserID,
			Specialization: in.Specialization,
			LicenseNumber:  in.LicenseNumber,
			Bio:            in.Bio,
		})
		if err != nil {
			return apperr.Internal(err)
		}
		out = &Doctor{
			ID:             d.ID,
			UserID:         d.UserID,
			Specialization: d.Specialization,
			LicenseNumber:  d.LicenseNumber,
			Bio:            d.Bio,
			IsActive:       d.IsActive,
		}
		return nil
	})
	return out, err
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Doctor, error) {
	var out *Doctor
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		row, err := db.New(tx).GetDoctorByID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("doctor not found")
		}
		if err != nil {
			return apperr.Internal(err)
		}
		out = &Doctor{
			ID:             row.ID,
			UserID:         row.UserID,
			Specialization: row.Specialization,
			LicenseNumber:  row.LicenseNumber,
			Bio:            row.Bio,
			FullName:       row.FullName,
			Email:          row.UserEmail,
			IsActive:       row.IsActive,
		}
		return nil
	})
	return out, err
}

func (r *PostgresRepository) List(ctx context.Context, activeOnly bool, specialization string, limit, offset int) ([]Doctor, error) {
	var doctors []Doctor
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		rows, err := db.New(tx).ListDoctors(ctx, db.ListDoctorsParams{
			IsActive:       activeOnly,
			Specialization: specialization,
			Limit:          int32(limit),
			Offset:         int32(offset),
		})
		if err != nil {
			return apperr.Internal(err)
		}
		doctors = make([]Doctor, 0, len(rows))
		for i := range rows {
			doctors = append(doctors, Doctor{
				ID:             rows[i].ID,
				UserID:         rows[i].UserID,
				Specialization: rows[i].Specialization,
				LicenseNumber:  rows[i].LicenseNumber,
				Bio:            rows[i].Bio,
				FullName:       rows[i].FullName,
				Email:          rows[i].UserEmail,
				IsActive:       rows[i].IsActive,
			})
		}
		return nil
	})
	return doctors, err
}

func (r *PostgresRepository) AddSchedule(ctx context.Context, doctorID uuid.UUID, dayOfWeek int, start, end time.Time, slotMinutes int) (*Schedule, error) {
	var out *Schedule
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		s, err := db.New(tx).CreateDoctorSchedule(ctx, db.CreateDoctorScheduleParams{
			DoctorID:    doctorID,
			DayOfWeek:   int16(dayOfWeek),
			StartTime:   toPgTime(start),
			EndTime:     toPgTime(end),
			SlotMinutes: int32(slotMinutes),
		})
		if err != nil {
			return apperr.Conflict("schedule conflicts with an existing one for this doctor")
		}
		out = fromScheduleRow(s)
		return nil
	})
	return out, err
}

func (r *PostgresRepository) GetSchedules(ctx context.Context, doctorID uuid.UUID) ([]Schedule, error) {
	var schedules []Schedule
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		rows, err := db.New(tx).ListDoctorSchedules(ctx, doctorID)
		if err != nil {
			return apperr.Internal(err)
		}
		schedules = make([]Schedule, 0, len(rows))
		for i := range rows {
			schedules = append(schedules, *fromScheduleRow(rows[i]))
		}
		return nil
	})
	return schedules, err
}

func (r *PostgresRepository) RemoveSchedule(ctx context.Context, scheduleID uuid.UUID) error {
	return r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		if err := db.New(tx).DeactivateDoctorSchedule(ctx, scheduleID); err != nil {
			return apperr.Internal(err)
		}
		return nil
	})
}

func (r *PostgresRepository) AddException(ctx context.Context, doctorID uuid.UUID, date time.Time, isUnavailable bool, start, end *time.Time, reason string) error {
	return r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		var st, et pgtype.Time
		if !isUnavailable && start != nil && end != nil {
			st = toPgTime(*start)
			et = toPgTime(*end)
		}
		var reasonPtr *string
		if reason != "" {
			reasonPtr = &reason
		}
		if _, err := db.New(tx).CreateScheduleException(ctx, db.CreateScheduleExceptionParams{
			DoctorID:      doctorID,
			ExceptionDate: dateOnly(date),
			IsUnavailable: isUnavailable,
			StartTime:     st,
			EndTime:       et,
			Reason:        reasonPtr,
		}); err != nil {
			return apperr.Invalid("invalid exception configuration")
		}
		return nil
	})
}

func (r *PostgresRepository) GetExceptions(ctx context.Context, doctorID uuid.UUID, from, to time.Time) ([]ScheduleExceptionRow, error) {
	var out []ScheduleExceptionRow
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		rows, err := db.New(tx).ListScheduleExceptions(ctx, db.ListScheduleExceptionsParams{
			DoctorID:        doctorID,
			ExceptionDate:   dateOnly(from),
			ExceptionDate_2: dateOnly(to),
		})
		if err != nil {
			return apperr.Internal(err)
		}
		out = make([]ScheduleExceptionRow, 0, len(rows))
		for i := range rows {
			row := rows[i]
			e := ScheduleExceptionRow{Date: row.ExceptionDate, IsUnavailable: row.IsUnavailable}
			if row.StartTime.Valid {
				t := pgToTime(row.StartTime)
				e.StartTime = &t
			}
			if row.EndTime.Valid {
				t := pgToTime(row.EndTime)
				e.EndTime = &t
			}
			out = append(out, e)
		}
		return nil
	})
	return out, err
}

func (r *PostgresRepository) GetActiveAppointmentsInRange(ctx context.Context, doctorID uuid.UUID, from, to time.Time) ([]AppointmentRange, error) {
	var out []AppointmentRange
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		rows, err := db.New(tx).ListActiveAppointmentsForDoctorInRange(ctx, db.ListActiveAppointmentsForDoctorInRangeParams{
			DoctorID:   doctorID,
			RangeStart: from,
			RangeEnd:   to,
		})
		if err != nil {
			return apperr.Internal(err)
		}
		out = make([]AppointmentRange, 0, len(rows))
		for i := range rows {
			out = append(out, AppointmentRange{
				ID:        rows[i].ID,
				StartTime: rows[i].StartTime,
				EndTime:   rows[i].EndTime,
			})
		}
		return nil
	})
	return out, err
}

func fromScheduleRow(s db.DoctorSchedule) *Schedule {
	return &Schedule{
		ID:          s.ID,
		DayOfWeek:   int(s.DayOfWeek),
		StartTime:   pgToTime(s.StartTime),
		EndTime:     pgToTime(s.EndTime),
		SlotMinutes: int(s.SlotMinutes),
	}
}

func toPgTime(t time.Time) pgtype.Time {
	micros := int64(t.Hour()*3600+t.Minute()*60+t.Second()) * 1_000_000
	micros += int64(t.Nanosecond() / 1000)
	return pgtype.Time{Microseconds: micros, Valid: true}
}

func pgToTime(p pgtype.Time) time.Time {
	base := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	return base.Add(time.Duration(p.Microseconds) * time.Microsecond)
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
