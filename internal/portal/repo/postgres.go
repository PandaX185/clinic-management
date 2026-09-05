// Package repo implements the patient portal persistence port. Global reads
// (identity, clinic registry) run against the public schema; the cross-clinic
// appointment view scans each active clinic's tenant schema for the user's
// profile. Tenant counts per deployment are small, so the fan-out is bounded.
package repo

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/database"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	portalsvc "github.com/PandaX185/clinic-management/internal/portal/service"
)

// PostgresRepository resolves patient portal data against Postgres.
type PostgresRepository struct {
	pool   *pgxpool.Pool
	scoped *database.ScopedPool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, scoped: database.NewScopedPool(pool)}
}

func (r *PostgresRepository) Me(ctx context.Context, userID uuid.UUID) (*portalsvc.User, error) {
	row, err := db.New(r.pool).GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("user not found")
		}
		return nil, apperr.Internal(err)
	}
	return &portalsvc.User{
		ID:       row.ID,
		Phone:    row.Phone,
		FullName: row.FullName,
		IsActive: row.Status == "active",
	}, nil
}

func (r *PostgresRepository) UpdateMe(ctx context.Context, userID uuid.UUID, in portalsvc.UpdateUserInput) error {
	_, err := db.New(r.pool).UpdateUserProfile(ctx, db.UpdateUserProfileParams{
		ID:       userID,
		FullName: in.FullName,
		Phone:    in.Phone,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return apperr.Conflict("phone is already in use")
		}
		return apperr.Internal(err)
	}
	return nil
}

func (r *PostgresRepository) GetClinic(ctx context.Context, clinicID uuid.UUID) (*portalsvc.ClinicRef, error) {
	var out portalsvc.ClinicRef
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, slug FROM tenants WHERE id = $1 AND status = 'active'
	`, clinicID).Scan(&out.ID, &out.Name, &out.Slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("clinic not found")
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &out, nil
}

// ListAppointments fans out to every active clinic, collecting the patient's
// appointments wherever they have a profile. Results are merged newest first.
// Clinics without a provisioned schema are skipped rather than poisoning the
// whole scan.
func (r *PostgresRepository) ListAppointments(ctx context.Context, userID uuid.UUID) ([]portalsvc.Appointment, error) {
	clinics, err := db.New(r.pool).ListClinics(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	existing, err := database.ExistingTenantSchemas(ctx, r.pool)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	out := make([]portalsvc.Appointment, 0, 8)
	for _, clinic := range clinics {
		if _, ok := existing[database.SchemaName(clinic.Slug)]; !ok {
			continue
		}
		err := r.scoped.WithSchema(ctx, clinic.Slug, func(tx pgx.Tx) error {
			rows, err := db.New(tx).ListAppointmentsByUser(ctx, userID)
			if err != nil {
				return err
			}
			for _, row := range rows {
				appt := fromAppointmentRow(row)
				appt.ClinicID = clinic.ID
				appt.ClinicName = clinic.Name
				out = append(out, appt)
			}
			return nil
		})
		if err != nil {
			return nil, apperr.Internal(err)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].StartTime.After(out[j].StartTime) })
	return out, nil
}

func (r *PostgresRepository) GetAppointment(ctx context.Context, userID uuid.UUID, apptID, clinicID uuid.UUID) (*portalsvc.Appointment, error) {
	clinic, err := r.GetClinic(ctx, clinicID)
	if err != nil {
		return nil, err
	}

	var out *portalsvc.Appointment
	err = r.scoped.WithSchema(ctx, clinic.Slug, func(tx pgx.Tx) error {
		row, err := db.New(tx).GetAppointmentByIDAndUser(ctx, db.GetAppointmentByIDAndUserParams{
			ID:     apptID,
			UserID: userID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.NotFound("appointment not found")
			}
			return apperr.Internal(err)
		}
		a := fromAppointmentRow(row)
		a.ClinicID = clinic.ID
		a.ClinicName = clinic.Name
		out = &a
		return nil
	})
	return out, err
}

// EnsurePatientProfile provisions the patient's profile row in a clinic so
// the appointment service can link bookings to them, returning its id.
// Existing profiles are reused untouched (their display name is preserved).
func (r *PostgresRepository) EnsurePatientProfile(ctx context.Context, slug string, userID uuid.UUID) (uuid.UUID, error) {
	var profileID uuid.UUID
	err := r.scoped.WithSchema(ctx, slug, func(tx pgx.Tx) error {
		q := db.New(tx)
		profile, err := q.GetProfileByUserID(ctx, userID)
		if err == nil {
			profileID = profile.ID
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		created, err := q.UpsertPatientProfile(ctx, db.UpsertPatientProfileParams{
			UserID:      userID,
			DisplayName: "Patient",
		})
		if err != nil {
			return err
		}
		profileID = created.ID
		return nil
	})
	if err != nil {
		return uuid.Nil, apperr.Internal(err)
	}
	return profileID, nil
}

func fromAppointmentRow(row db.Appointment) portalsvc.Appointment {
	return portalsvc.Appointment{
		ID:                row.ID,
		PatientID:         row.ProfileID,
		DoctorID:          row.DoctorProfileID,
		AppointmentTypeID: row.AppointmentTypeID,
		StartTime:         row.ScheduledStart,
		EndTime:           row.ScheduledEnd,
		Status:            row.Status,
		Notes:             textPtr(row.VisitNotes),
	}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}
