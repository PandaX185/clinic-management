// Package repo implements the clinic queue persistence port against the
// active tenant schema. Position is computed with a window function over the
// active subset, so entries never carry a stored line number.
package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/lahza/internal/platform/apperr"
	"github.com/PandaX185/lahza/internal/platform/database"
	db "github.com/PandaX185/lahza/internal/platform/db/sqlc"
	queuesvc "github.com/PandaX185/lahza/internal/queue/service"
)

// PostgresRepository persists queue entries in a tenant schema.
type PostgresRepository struct {
	scoped *database.ScopedPool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{scoped: database.NewScopedPool(pool)}
}

func (r *PostgresRepository) CreateEntry(ctx context.Context, profileID uuid.UUID, appointmentID *uuid.UUID, priority int32) (*queuesvc.Entry, error) {
	var out *queuesvc.Entry
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		q := db.New(tx)
		created, err := q.CreateQueueEntry(ctx, db.CreateQueueEntryParams{
			ProfileID:     profileID,
			AppointmentID: appointmentID,
			Priority:      priority,
		})
		if err != nil {
			return wrapFK(err, "profile not found")
		}
		row, err := q.GetQueueEntryByID(ctx, created.ID)
		if err != nil {
			return apperr.Internal(err)
		}
		e := fromRow(row)
		out = &e
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepository) GetEntry(ctx context.Context, id uuid.UUID) (*queuesvc.Entry, error) {
	var out *queuesvc.Entry
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		row, err := db.New(tx).GetQueueEntryByID(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.NotFound("queue entry not found")
			}
			return apperr.Internal(err)
		}
		e := fromRow(row)
		out = &e
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepository) ListActive(ctx context.Context, q queuesvc.ListQuery) ([]queuesvc.Entry, int64, error) {
	var out []queuesvc.Entry
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		rows, err := db.New(tx).ListActiveQueue(ctx, db.ListActiveQueueParams{
			From:   q.From,
			Limit:  q.Limit,
			Offset: q.Offset,
		})
		if err != nil {
			return apperr.Internal(err)
		}
		out = make([]queuesvc.Entry, 0, len(rows))
		for _, row := range rows {
			e := fromActiveRow(row)
			e.Position = row.Position
			e.ActiveTotal = row.ActiveTotal
			out = append(out, e)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	total := int64(0)
	if len(out) > 0 {
		total = out[0].ActiveTotal
	}
	return out, total, nil
}

func (r *PostgresRepository) ListForProfile(ctx context.Context, profileID uuid.UUID) ([]queuesvc.Entry, error) {
	var out []queuesvc.Entry
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		rows, err := db.New(tx).ListQueueEntriesForProfile(ctx, profileID)
		if err != nil {
			return apperr.Internal(err)
		}
		out = make([]queuesvc.Entry, 0, len(rows))
		for _, row := range rows {
			out = append(out, fromProfileRow(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Position returns the entry's 1-based spot in the active line given its
// priority and check-in time. Callers use it only for active entries.
func (r *PostgresRepository) Position(ctx context.Context, priority int32, checkedInAt time.Time) (int64, error) {
	var pos int64
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		p, err := db.New(tx).GetQueueEntryPosition(ctx, db.GetQueueEntryPositionParams{
			Priority:    priority,
			CheckedInAt: checkedInAt,
		})
		if err != nil {
			return apperr.Internal(err)
		}
		pos = int64(p)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return pos, nil
}

func (r *PostgresRepository) Transition(ctx context.Context, id uuid.UUID, target string) (*queuesvc.Entry, error) {
	var out *queuesvc.Entry
	err := r.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		q := db.New(tx)
		if _, err := q.UpdateQueueEntryStatus(ctx, db.UpdateQueueEntryStatusParams{
			ID:      id,
			Column2: target,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.NotFound("queue entry not found")
			}
			return apperr.Internal(err)
		}
		row, err := q.GetQueueEntryByID(ctx, id)
		if err != nil {
			return apperr.Internal(err)
		}
		e := fromRow(row)
		out = &e
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func fromActiveRow(row db.ListActiveQueueRow) queuesvc.Entry {
	return queuesvc.Entry{
		ID:            row.ID,
		ProfileID:     row.ProfileID,
		AppointmentID: row.AppointmentID,
		PatientName:   row.PatientName,
		Status:        row.Status,
		Priority:      row.Priority,
		CheckedInAt:   row.CheckedInAt,
		CalledAt:      row.CalledAt,
		StartedAt:     row.StartedAt,
		CompletedAt:   row.CompletedAt,
	}
}

func fromRow(row db.GetQueueEntryByIDRow) queuesvc.Entry {
	return queuesvc.Entry{
		ID:            row.ID,
		ProfileID:     row.ProfileID,
		AppointmentID: row.AppointmentID,
		PatientName:   row.PatientName,
		Status:        row.Status,
		Priority:      row.Priority,
		CheckedInAt:   row.CheckedInAt,
		CalledAt:      row.CalledAt,
		StartedAt:     row.StartedAt,
		CompletedAt:   row.CompletedAt,
	}
}

func fromProfileRow(row db.ListQueueEntriesForProfileRow) queuesvc.Entry {
	return queuesvc.Entry{
		ID:            row.ID,
		ProfileID:     row.ProfileID,
		AppointmentID: row.AppointmentID,
		PatientName:   row.PatientName,
		Status:        row.Status,
		Priority:      row.Priority,
		CheckedInAt:   row.CheckedInAt,
		CalledAt:      row.CalledAt,
		StartedAt:     row.StartedAt,
		CompletedAt:   row.CompletedAt,
	}
}

func wrapFK(err error, msg string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return apperr.NotFound(msg)
	}
	return apperr.Internal(err)
}

var _ queuesvc.Repository = (*PostgresRepository)(nil)
