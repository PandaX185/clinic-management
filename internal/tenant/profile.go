package tenant

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresProfileStore is a ProfileStore implementation that reads from
// the active tenant schema via ScopedPool.
type PostgresProfileStore struct {
	pool *pgxpool.Pool
}

// NewScopedProfileStore creates a ProfileStore from the raw pool.
func NewScopedProfileStore(pool *pgxpool.Pool) *PostgresProfileStore {
	return &PostgresProfileStore{pool: pool}
}

// RoleForUser returns the caller's role in the active tenant; empty
// string means no profile yet (patient-level access).
func (s *PostgresProfileStore) RoleForUser(ctx context.Context, userID uuid.UUID) (string, error) {
	// In v2, role resolution happens inside the ScopedPool in the appointment
	// package's identity resolution. This is a no-op stub for future per-tenant RBAC.
	return "", nil
}

// EnsurePatientProfile is a no-op in v2 — the booking flow auto-provisions
// profiles. Retained for the ProfileStore interface contract.
func (s *PostgresProfileStore) EnsurePatientProfile(ctx context.Context, userID uuid.UUID) error {
	return nil
}
