package notification

import (
	"context"

	"github.com/PandaX185/clinic-management/internal/platform/database"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore persists notification records; nats_msg_id carries a unique
// constraint so duplicate publishes collapse into one row (FR-NOT-05).
type PostgresStore struct {
	scoped *database.ScopedPool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{scoped: database.NewScopedPool(pool)}
}

// withTenant runs fn against the tenant schema from context. The
// EventForwarder publishes from within a tenant-scoped request, so the
// notifications row lands in the right clinic's schema.
func (s *PostgresStore) withTenant(ctx context.Context, fn func(q *db.Queries) error) error {
	return s.scoped.WithSchema(ctx, database.TenantSlugFrom(ctx), func(tx pgx.Tx) error {
		return fn(db.New(tx))
	})
}

// CreatePending persists a notification record using msg.ID as the primary
// key AND the dedupe key, so exactly one identity flows through the pipeline.
func (s *PostgresStore) CreatePending(ctx context.Context, msg Message) (uuid.UUID, error) {
	if msg.ID == uuid.Nil {
		msg.ID = uuid.New()
	}
	subjectStr := msg.Subject
	var apptID *uuid.UUID
	if msg.AppointmentID != uuid.Nil {
		id := msg.AppointmentID
		apptID = &id
	}
	var outID uuid.UUID
	err := s.withTenant(ctx, func(qq *db.Queries) error {
		row, err := qq.InsertNotification(ctx, db.InsertNotificationParams{
			ID:            msg.ID,
			AppointmentID: apptID,
			Channel:       string(msg.Channel),
			Recipient:     msg.Recipient,
			Subject:       subjectStr,
			Body:          msg.Body,
			MsgID:         msg.ID.String(),
		})
		if err != nil {
			return err
		}
		outID = row.ID
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	return outID, nil
}

func (s *PostgresStore) GetByMsgID(ctx context.Context, id uuid.UUID) (*Record, error) {
	var rec *Record
	err := s.withTenant(ctx, func(qq *db.Queries) error {
		idStr := id.String()
		row, err := qq.GetNotificationByMsgID(ctx, &idStr)
		if err != nil {
			return err
		}
		rec = &Record{
			ID: row.ID, Status: row.Status, Attempts: row.Attempts,
			Channel: Channel(row.Channel), Recipient: row.Recipient, Body: row.Body,
		}
		if row.Subject != nil {
			rec.Subject = *row.Subject
		}
		return nil
	})
	return rec, err
}

func (s *PostgresStore) MarkStatus(ctx context.Context, id uuid.UUID, status string, lastErr *string) error {
	return s.withTenant(ctx, func(qq *db.Queries) error {
		switch status {
		case StatusSent:
			return qq.MarkNotificationSent(ctx, id)
		case StatusDeadLetter:
			return qq.MarkNotificationDead(ctx, db.MarkNotificationDeadParams{ID: id, LastError: lastErr})
		default:
			return qq.MarkNotificationFailed(ctx, db.MarkNotificationFailedParams{ID: id, LastError: lastErr})
		}
	})
}
