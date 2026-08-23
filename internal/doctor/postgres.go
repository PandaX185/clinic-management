package doctor

import (
	"context"
	"errors"
	"time"

	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

type PostgresRepository struct {
	q *db.Queries
}

func NewPostgresRepository(pool db.DBTX) *PostgresRepository {
	return &PostgresRepository{q: db.New(pool)}
}

func (r *PostgresRepository) Create(ctx context.Context, in CreateDoctorInput) (*Doctor, error) {
	d, err := r.q.CreateDoctor(ctx, db.CreateDoctorParams{
		UserID:         in.UserID,
		Specialization: in.Specialization,
		LicenseNumber:  in.LicenseNumber,
		Bio:            in.Bio,
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &Doctor{
		ID:             d.ID,
		UserID:         d.UserID,
		Specialization: d.Specialization,
		LicenseNumber:  d.LicenseNumber,
		Bio:            d.Bio,
		IsActive:       d.IsActive,
	}, nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Doctor, error) {
	row, err := r.q.GetDoctorByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("doctor not found")
		}
		return nil, apperr.Internal(err)
	}
	return &Doctor{
		ID:             row.ID,
		UserID:         row.UserID,
		Specialization: row.Specialization,
		LicenseNumber:  row.LicenseNumber,
		Bio:            row.Bio,
		FullName:       row.FullName,
		Email:          row.UserEmail,
		IsActive:       row.IsActive,
	}, nil
}

func (r *PostgresRepository) List(ctx context.Context, activeOnly bool, specialization string, limit, offset int) ([]Doctor, error) {
	rows, err := r.q.ListDoctors(ctx, db.ListDoctorsParams{
		IsActive:       activeOnly,
		Specialization: specialization,
		Limit:          int32(limit),
		Offset:         int32(offset),
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	doctors := make([]Doctor, 0, len(rows))
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
	return doctors, nil
}

func (r *PostgresRepository) AddSchedule(ctx context.Context, doctorID uuid.UUID, dayOfWeek int, start, end time.Time, slotMinutes int) (*Schedule, error) {
	s, err := r.q.CreateDoctorSchedule(ctx, db.CreateDoctorScheduleParams{
		DoctorID:    doctorID,
		DayOfWeek:   int16(dayOfWeek),
		StartTime:   toPgTime(start),
		EndTime:     toPgTime(end),
		SlotMinutes: int32(slotMinutes),
	})
	if err != nil {
		return nil, apperr.Conflict("schedule conflicts with an existing one for this doctor")
	}
	return fromScheduleRow(s), nil
}

func (r *PostgresRepository) GetSchedules(ctx context.Context, doctorID uuid.UUID) ([]Schedule, error) {
	rows, err := r.q.ListDoctorSchedules(ctx, doctorID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	schedules := make([]Schedule, 0, len(rows))
	for i := range rows {
		schedules = append(schedules, *fromScheduleRow(rows[i]))
	}
	return schedules, nil
}

func (r *PostgresRepository) RemoveSchedule(ctx context.Context, scheduleID uuid.UUID) error {
	if err := r.q.DeactivateDoctorSchedule(ctx, scheduleID); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

func (r *PostgresRepository) AddException(ctx context.Context, doctorID uuid.UUID, date time.Time, isUnavailable bool, start, end *time.Time, reason string) error {
	var st, et pgtype.Time
	if !isUnavailable && start != nil && end != nil {
		st = toPgTime(*start)
		et = toPgTime(*end)
	}
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}
	if _, err := r.q.CreateScheduleException(ctx, db.CreateScheduleExceptionParams{
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
}

func (r *PostgresRepository) GetExceptions(ctx context.Context, doctorID uuid.UUID, from, to time.Time) ([]ScheduleExceptionRow, error) {
	rows, err := r.q.ListScheduleExceptions(ctx, db.ListScheduleExceptionsParams{
		DoctorID:        doctorID,
		ExceptionDate:   dateOnly(from),
		ExceptionDate_2: dateOnly(to),
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]ScheduleExceptionRow, 0, len(rows))
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
	return out, nil
}

func (r *PostgresRepository) GetActiveAppointmentsInRange(ctx context.Context, doctorID uuid.UUID, from, to time.Time) ([]AppointmentRange, error) {
	rows, err := r.q.ListActiveAppointmentsForDoctorInRange(ctx, db.ListActiveAppointmentsForDoctorInRangeParams{
		DoctorID:   doctorID,
		RangeStart: from,
		RangeEnd:   to,
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]AppointmentRange, 0, len(rows))
	for i := range rows {
		out = append(out, AppointmentRange{
			ID:        rows[i].ID,
			StartTime: rows[i].StartTime,
			EndTime:   rows[i].EndTime,
		})
	}
	return out, nil
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
