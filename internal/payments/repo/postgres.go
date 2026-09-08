// Package repo implements the payments persistence port against the tenant
// schema. Every query runs inside the schema named by the context's tenant
// slug (database.TenantSlugFrom); the slug is set by the caller via
// database.WithTenantSlug, never derived from client input.
package repo

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	paymentsvc "github.com/PandaX185/lahza/internal/payments/service"
	"github.com/PandaX185/lahza/internal/platform/apperr"
	"github.com/PandaX185/lahza/internal/platform/database"
	db "github.com/PandaX185/lahza/internal/platform/db/sqlc"
)

// PostgresRepository persists payments in the appointment's tenant schema.
type PostgresRepository struct {
	scoped *database.ScopedPool
}

// NewPostgresRepository creates a payments repository on a shared pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{scoped: database.NewScopedPool(pool)}
}

func (r *PostgresRepository) GetAppointmentForPayment(ctx context.Context, userID, apptID uuid.UUID) (*paymentsvc.Appointment, error) {
	var out *paymentsvc.Appointment
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		row, err := db.New(tx).GetAppointmentByIDAndUser(ctx, db.GetAppointmentByIDAndUserParams{
			ID:     apptID,
			UserID: userID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.NotFound("appointment not found")
			}
			return err
		}
		atype, err := db.New(tx).GetAppointmentTypeByID(ctx, row.AppointmentTypeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.NotFound("appointment type not found")
			}
			return err
		}
		out = &paymentsvc.Appointment{
			ID:        row.ID,
			PatientID: row.ProfileID,
			DoctorID:  row.DoctorProfileID,
			TypeID:    row.AppointmentTypeID,
			StartTime: row.ScheduledStart,
			EndTime:   row.ScheduledEnd,
			Status:    row.Status,
			Price:     numericString(atype.Price),
			Currency:  "EGP",
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepository) InsertPending(ctx context.Context, in paymentsvc.PayInput, amount, currency string) (*paymentsvc.Payment, error) {
	money, err := decimalMoney(amount)
	if err != nil {
		return nil, err
	}
	var reference *string
	if in.Reference != "" {
		reference = &in.Reference
	}
	var out *paymentsvc.Payment
	err = r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		row, err := db.New(tx).InsertPayment(ctx, db.InsertPaymentParams{
			AppointmentID: in.AppointmentID,
			Amount:        money,
			Currency:      currency,
			Method:        string(in.Method),
			Status:        string(paymentsvc.StatusPending),
			Reference:     reference,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // unique constraint: already exists
			}
			return err
		}
		p := fromRow(row)
		out = &p
		return nil
	})
	if err != nil {
		return nil, apperrInternal(err)
	}
	return out, nil
}

func (r *PostgresRepository) GetForAppointment(ctx context.Context, appointmentID uuid.UUID) (*paymentsvc.Payment, error) {
	return r.getOne(ctx, func(q *db.Queries) (db.Payment, error) {
		return q.GetPaymentForAppointment(ctx, appointmentID)
	})
}

func (r *PostgresRepository) GetByID(ctx context.Context, paymentID uuid.UUID) (*paymentsvc.Payment, error) {
	return r.getOne(ctx, func(q *db.Queries) (db.Payment, error) {
		return q.GetPaymentByID(ctx, paymentID)
	})
}

func (r *PostgresRepository) MarkPaid(ctx context.Context, paymentID uuid.UUID) (*paymentsvc.Payment, error) {
	return r.getOne(ctx, func(q *db.Queries) (db.Payment, error) {
		return q.MarkPaymentPaid(ctx, paymentID)
	})
}

func (r *PostgresRepository) MarkRefunded(ctx context.Context, paymentID uuid.UUID) (*paymentsvc.Payment, error) {
	return r.getOne(ctx, func(q *db.Queries) (db.Payment, error) {
		return q.MarkPaymentRefunded(ctx, paymentID)
	})
}

// getOne runs a payment lookup inside the tenant schema. A missing row
// (pgx.ErrNoRows) is reported as a nil payment rather than an error so the
// service can distinguish "no record" from a real failure.
func (r *PostgresRepository) getOne(ctx context.Context, fn func(q *db.Queries) (db.Payment, error)) (*paymentsvc.Payment, error) {
	var out *paymentsvc.Payment
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		row, err := fn(db.New(tx))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		p := fromRow(row)
		out = &p
		return nil
	})
	if err != nil {
		return nil, apperrInternal(err)
	}
	return out, nil
}

func fromRow(row db.Payment) paymentsvc.Payment {
	return paymentsvc.Payment{
		ID:            row.ID,
		AppointmentID: row.AppointmentID,
		Amount:        numericString(row.Amount),
		Currency:      row.Currency,
		Method:        paymentsvc.Method(row.Method),
		Status:        paymentsvc.Status(row.Status),
		PaidAt:        row.PaidAt,
		Reference:     row.Reference,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func apperrInternal(err error) error {
	return apperr.Internal(err)
}

// decimalMoney parses a decimal string into a pgtype.Numeric for storage. A
// blank string becomes zero.
func decimalMoney(s string) (pgtype.Numeric, error) {
	s = strings.TrimSpace(s)
	var n pgtype.Numeric
	if s == "" {
		s = "0"
	}
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, apperr.Invalid("amount must be a valid decimal")
	}
	return n, nil
}

func numericString(n pgtype.Numeric) string {
	if !n.Valid {
		return "0"
	}
	v, err := n.Value()
	if err != nil {
		return "0"
	}
	if s, ok := v.(string); ok {
		return s
	}
	if f, ok := v.(float64); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return "0"
}
