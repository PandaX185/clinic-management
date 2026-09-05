// Package repo implements the public clinic discovery persistence port.
// Global registry lookups (tenants) run against the public schema; everything
// clinic-specific (services, doctors, schedules, bookings) runs inside the
// clinic's tenant schema.
package repo

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	bookingsvc "github.com/PandaX185/clinic-management/internal/booking/service"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/database"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
)

// PostgresRepository resolves clinic discovery against Postgres.
type PostgresRepository struct {
	pool   *pgxpool.Pool
	scoped *database.ScopedPool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, scoped: database.NewScopedPool(pool)}
}

func (r *PostgresRepository) ListClinics(ctx context.Context, offset, limit int) ([]bookingsvc.Clinic, int64, error) {
	q := db.New(r.pool)

	total, err := q.CountActiveClinics(ctx)
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}

	rows, err := q.ListClinicsPaginated(ctx, db.ListClinicsPaginatedParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}

	out := make([]bookingsvc.Clinic, 0, len(rows))
	for _, row := range rows {
		out = append(out, clinicFromRow(row))
	}
	return out, total, nil
}

func (r *PostgresRepository) GetClinic(ctx context.Context, id uuid.UUID) (*bookingsvc.Clinic, error) {
	row, err := db.New(r.pool).GetClinicByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("clinic not found")
		}
		return nil, apperr.Internal(err)
	}
	if row.Status != "active" {
		return nil, apperr.NotFound("clinic not found")
	}
	c := clinicFromRow(row)
	return &c, nil
}

func (r *PostgresRepository) ListClinicServices(ctx context.Context, clinicID uuid.UUID) ([]bookingsvc.ServiceItem, error) {
	slug, err := r.slugFor(ctx, clinicID)
	if err != nil {
		return nil, err
	}

	var out []bookingsvc.ServiceItem
	err = r.scoped.WithSchema(ctx, slug, func(tx pgx.Tx) error {
		rows, err := db.New(tx).ListAppointmentTypes(ctx)
		if err != nil {
			return apperr.Internal(err)
		}
		out = make([]bookingsvc.ServiceItem, 0, len(rows))
		for _, row := range rows {
			out = append(out, bookingsvc.ServiceItem{
				ID:          row.ID,
				Name:        row.Name,
				DurationMin: int(row.DurationMinutes),
				Price:       numericToFloat(row.Price),
			})
		}
		return nil
	})
	return out, err
}

func (r *PostgresRepository) ListClinicDoctors(ctx context.Context, clinicID uuid.UUID, offset, limit int) ([]bookingsvc.Doctor, int64, error) {
	slug, err := r.slugFor(ctx, clinicID)
	if err != nil {
		return nil, 0, err
	}
	clinicName, err := r.nameFor(ctx, clinicID)
	if err != nil {
		return nil, 0, err
	}

	var out []bookingsvc.Doctor
	var total int64
	err = r.scoped.WithSchema(ctx, slug, func(tx pgx.Tx) error {
		q := db.New(tx)
		count, err := q.CountProfilesByRole(ctx, "doctor")
		if err != nil {
			return apperr.Internal(err)
		}
		total = count

		rows, err := q.ListProfilesByRolePaginated(ctx, db.ListProfilesByRolePaginatedParams{
			Name:   "doctor",
			Limit:  int32(limit),
			Offset: int32(offset),
		})
		if err != nil {
			return apperr.Internal(err)
		}
		out = make([]bookingsvc.Doctor, 0, len(rows))
		for _, row := range rows {
			out = append(out, bookingsvc.Doctor{
				ID:         row.ID,
				Name:       row.DisplayName,
				ClinicID:   clinicID,
				ClinicName: clinicName,
			})
		}
		return nil
	})
	return out, total, err
}

// FindDoctor scans active clinics for a profile holding the doctor role and
// returns it with the clinic it practises at. Tenants without a provisioned
// schema are skipped so a single missing schema does not fail the lookup.
func (r *PostgresRepository) FindDoctor(ctx context.Context, doctorID uuid.UUID) (*bookingsvc.Doctor, error) {
	clinics, err := db.New(r.pool).ListClinics(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	existing, err := database.ExistingTenantSchemas(ctx, r.pool)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	for _, clinic := range clinics {
		if _, ok := existing[database.SchemaName(clinic.Slug)]; !ok {
			continue
		}
		var found *bookingsvc.Doctor
		err := r.scoped.WithSchema(ctx, clinic.Slug, func(tx pgx.Tx) error {
			q := db.New(tx)
			prof, err := q.GetProfileByID(ctx, doctorID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil
				}
				return err
			}
			isDoctor, err := q.ProfileHasRole(ctx, db.ProfileHasRoleParams{ProfileID: prof.ID, Name: "doctor"})
			if err != nil {
				return err
			}
			if !isDoctor || prof.Status != "active" {
				return nil
			}
			found = &bookingsvc.Doctor{
				ID:         prof.ID,
				Name:       prof.DisplayName,
				ClinicID:   clinic.ID,
				ClinicName: clinic.Name,
			}
			return nil
		})
		if err != nil {
			return nil, apperr.Internal(err)
		}
		if found != nil {
			return found, nil
		}
	}
	return nil, apperr.NotFound("doctor not found")
}

func (r *PostgresRepository) ListDoctorSchedules(ctx context.Context, clinicID uuid.UUID, doctorID uuid.UUID, date time.Time) ([]bookingsvc.Schedule, error) {
	slug, err := r.slugFor(ctx, clinicID)
	if err != nil {
		return nil, err
	}

	var out []bookingsvc.Schedule
	err = r.scoped.WithSchema(ctx, slug, func(tx pgx.Tx) error {
		rows, err := db.New(tx).ListDoctorSchedulesOnDay(ctx, db.ListDoctorSchedulesOnDayParams{
			DoctorProfileID: doctorID,
			Date:            date,
		})
		if err != nil {
			return apperr.Internal(err)
		}
		out = make([]bookingsvc.Schedule, 0, len(rows))
		for _, row := range rows {
			out = append(out, bookingsvc.Schedule{
				DayOfWeek: int(row.DayOfWeek),
				StartMin:  pgTimeMinutes(row.StartTime),
				EndMin:    pgTimeMinutes(row.EndTime),
			})
		}
		return nil
	})
	return out, err
}

func (r *PostgresRepository) ListDoctorAppointments(ctx context.Context, clinicID uuid.UUID, doctorID uuid.UUID, from, to time.Time) ([]bookingsvc.Appointment, error) {
	slug, err := r.slugFor(ctx, clinicID)
	if err != nil {
		return nil, err
	}

	var out []bookingsvc.Appointment
	err = r.scoped.WithSchema(ctx, slug, func(tx pgx.Tx) error {
		rows, err := db.New(tx).ListAppointmentsForDoctorDate(ctx, db.ListAppointmentsForDoctorDateParams{
			DoctorProfileID: doctorID,
			From:            from,
			To:              to,
		})
		if err != nil {
			return apperr.Internal(err)
		}
		out = make([]bookingsvc.Appointment, 0, len(rows))
		for _, row := range rows {
			out = append(out, bookingsvc.Appointment{Start: row.ScheduledStart, End: row.ScheduledEnd})
		}
		return nil
	})
	return out, err
}

func (r *PostgresRepository) GetAppointmentType(ctx context.Context, clinicID uuid.UUID, typeID uuid.UUID) (*bookingsvc.ServiceItem, error) {
	slug, err := r.slugFor(ctx, clinicID)
	if err != nil {
		return nil, err
	}

	var out *bookingsvc.ServiceItem
	err = r.scoped.WithSchema(ctx, slug, func(tx pgx.Tx) error {
		row, err := db.New(tx).GetAppointmentTypeByID(ctx, typeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.NotFound("appointment type not found")
			}
			return apperr.Internal(err)
		}
		out = &bookingsvc.ServiceItem{
			ID:          row.ID,
			Name:        row.Name,
			DurationMin: int(row.DurationMinutes),
			Price:       numericToFloat(row.Price),
		}
		return nil
	})
	return out, err
}

// slugFor resolves a clinic's tenant schema slug, failing closed when the
// clinic does not exist or is not active.
func (r *PostgresRepository) slugFor(ctx context.Context, clinicID uuid.UUID) (string, error) {
	var slug string
	err := r.pool.QueryRow(ctx, `
		SELECT slug FROM tenants WHERE id = $1 AND status = 'active'
	`, clinicID).Scan(&slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.NotFound("clinic not found")
	}
	if err != nil {
		return "", apperr.Internal(err)
	}
	return slug, nil
}

func (r *PostgresRepository) nameFor(ctx context.Context, clinicID uuid.UUID) (string, error) {
	var name string
	err := r.pool.QueryRow(ctx, `SELECT name FROM tenants WHERE id = $1`, clinicID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.NotFound("clinic not found")
	}
	if err != nil {
		return "", apperr.Internal(err)
	}
	return name, nil
}

func clinicFromRow(row db.Tenant) bookingsvc.Clinic {
	c := bookingsvc.Clinic{
		ID:        row.ID,
		Name:      row.Name,
		Slug:      row.Slug,
		Address:   row.Address,
		City:      row.City,
		Phone:     row.Phone,
		Email:     row.Email,
		Hours:     map[string]string{},
		CreatedAt: row.CreatedAt,
	}
	if row.Description.Valid {
		desc := row.Description.String
		c.Description = &desc
	}
	if len(row.Hours) > 0 {
		var hours map[string]string
		if err := json.Unmarshal(row.Hours, &hours); err == nil && hours != nil {
			c.Hours = hours
		}
	}
	return c
}

func pgTimeMinutes(t pgtype.Time) int {
	if !t.Valid {
		return 0
	}
	return int(t.Microseconds / 1_000_000 / 60)
}

func numericToFloat(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	v, err := n.Value()
	if err != nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	}
	return 0
}
