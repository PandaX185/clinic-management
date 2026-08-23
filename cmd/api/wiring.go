package main

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/PandaX185/clinic-management/internal/appointment"
	auth "github.com/PandaX185/clinic-management/internal/auth"
	doctor "github.com/PandaX185/clinic-management/internal/doctor"
	notification "github.com/PandaX185/clinic-management/internal/notification"

	"github.com/PandaX185/clinic-management/internal/platform/database"
	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
)

// doctorUserAdapter bridges the doctor module's UserCreator port to the
// auth repository, hashing credentials before persistence (SEC-01).
type doctorUserAdapter struct {
	authRepo auth.Repository
	cost     int
}

func newDoctorUserAdapter(repo auth.Repository, cost int) *doctorUserAdapter {
	return &doctorUserAdapter{authRepo: repo, cost: cost}
}

func (a *doctorUserAdapter) CreateDoctorUser(ctx context.Context, email, password, fullName string) (*doctor.CreatedUser, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), a.cost)
	if err != nil {
		return nil, err
	}
	user, err := a.authRepo.CreateUser(ctx, email, string(hash), fullName, nil, auth.RoleDoctor)
	if err != nil {
		return nil, err
	}
	return &doctor.CreatedUser{ID: user.ID}, nil
}

// noopPublisher is used when NATS is unavailable: bookings still succeed and
// notifications are skipped (graceful degradation, NFR-REL-01).
type noopPublisher struct{}

func (noopPublisher) PublishAppointmentEvent(context.Context, appointment.Event) {}

// auditWriter persists domain audit events (FR-APT-09). Audit failures are
// logged upstream but never block the clinical workflow.
type auditWriter struct {
	q *db.Queries
}

func newAuditWriter(pool *database.Pool) *auditWriter {
	return &auditWriter{q: db.New(pool)}
}

var _ appointment.AuditWriter = (*auditWriter)(nil)

func (w *auditWriter) Write(ctx context.Context, entry appointment.AuditEntry) error {
	var entityID *uuid.UUID
	id := entry.EntityID
	entityID = &id

	var details json.RawMessage
	if entry.Details != nil {
		details = json.RawMessage(entry.Details)
	} else {
		details = json.RawMessage("{}")
	}

	return w.q.InsertAuditLog(ctx, db.InsertAuditLogParams{
		ActorUserID: entry.ActorID,
		Action:      entry.Action,
		EntityType:  entry.EntityType,
		EntityID:    entityID,
		Details:     details,
	})
}

var (
	_ doctor.UserCreator      = (*doctorUserAdapter)(nil)
	_ notification.Store      = (*notification.PostgresStore)(nil)
	_ appointment.AuditWriter = (*auditWriter)(nil)
)

// zapLoggerAdapter exposes the logging surface the domain modules need,
// decoupled from concrete zap types.
type zapLoggerAdapter struct {
	l *zap.Logger
}

func newLogger(l *zap.Logger) zapLoggerAdapter { return zapLoggerAdapter{l: l} }

func toZapFields(args []any) []zap.Field {
	fields := make([]zap.Field, 0, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		if k, ok := args[i].(string); ok {
			fields = append(fields, zap.Any(k, args[i+1]))
		}
	}
	return fields
}

func (a zapLoggerAdapter) Info(msg string, args ...any) { a.l.Info(msg, toZapFields(args)...) }

func (a zapLoggerAdapter) Warn(msg string, args ...any) { a.l.Warn(msg, toZapFields(args)...) }

func (a zapLoggerAdapter) Error(msg string, args ...any) { a.l.Error(msg, toZapFields(args)...) }
