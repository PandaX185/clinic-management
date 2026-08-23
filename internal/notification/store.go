package notification

import (
	"context"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/google/uuid"
)

// PostgresStore persists notification records; nats_msg_id carries a unique
// constraint so duplicate publishes collapse into one row (FR-NOT-05).
type PostgresStore struct {
	q *db.Queries
}

func NewPostgresStore(pool db.DBTX) *PostgresStore {
	return &PostgresStore{q: db.New(pool)}
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
	row, err := s.q.InsertNotification(ctx, db.InsertNotificationParams{
		ID:            msg.ID,
		AppointmentID: apptID,
		Channel:       string(msg.Channel),
		Recipient:     msg.Recipient,
		Subject:       subjectStr,
		Body:          msg.Body,
		MsgID:         msg.ID.String(),
	})
	if err != nil {
		return uuid.Nil, err
	}
	return row.ID, nil
}

func (s *PostgresStore) GetByMsgID(ctx context.Context, id uuid.UUID) (*Record, error) {
	idStr := id.String()
	row, err := s.q.GetNotificationByMsgID(ctx, &idStr)
	if err != nil {
		return nil, err
	}
	return &Record{ID: row.ID, Status: row.Status, Attempts: row.Attempts}, nil
}

func (s *PostgresStore) MarkStatus(ctx context.Context, id uuid.UUID, status string, lastErr *string) error {
	switch status {
	case StatusSent:
		return s.q.MarkNotificationSent(ctx, id)
	case StatusDeadLetter:
		return s.q.MarkNotificationDead(ctx, db.MarkNotificationDeadParams{ID: id, LastError: lastErr})
	default:
		return s.q.MarkNotificationFailed(ctx, db.MarkNotificationFailedParams{ID: id, LastError: lastErr})
	}
}
