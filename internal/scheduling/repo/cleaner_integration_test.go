// Integration test for the idempotency-key cleaner against a real Postgres.
// Regression for the bug where the sweep ran DELETE against the raw pool's
// default search path (public), where idempotency_keys does not exist, so
// expired keys were never purged from tenant schemas.
package repo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PandaX185/lahza/internal/platform/database"
	db "github.com/PandaX185/lahza/internal/platform/db/sqlc"
	"github.com/jackc/pgx/v5/pgxpool"
)

var itPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(runCleanerIntegration(m))
}

func runCleanerIntegration(m *testing.M) int {
	ctx := context.Background()
	adminURL := os.Getenv("TEST_PG_URL")
	if adminURL == "" {
		adminURL = "postgres://lahza:lahza@localhost:5432/postgres?sslmode=disable"
	}

	admin, err := pgxpool.New(ctx, adminURL)
	if err == nil {
		err = admin.Ping(ctx)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cleaner integration tests require a running Postgres (docker compose up):", err)
		return 1
	}
	defer admin.Close()

	testDB := "clinic_it_" + randHex(4)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+testDB); err != nil {
		fmt.Fprintln(os.Stderr, "cleaner integration: create database:", err)
		return 1
	}
	drop := func() { _, _ = admin.Exec(context.Background(), "DROP DATABASE "+testDB+" WITH (FORCE)") }

	cfg, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		drop()
		fmt.Fprintln(os.Stderr, "cleaner integration: parse url:", err)
		return 1
	}
	cfg.ConnConfig.Database = testDB
	itPool, err = pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		drop()
		fmt.Fprintln(os.Stderr, "cleaner integration: connect test db:", err)
		return 1
	}

	// Global migrations create public.users (idempotency_keys references it)
	// and public.tenants, which ProvisionTenant needs alongside.
	if err := applyGlobalMigrations(ctx, itPool); err != nil {
		itPool.Close()
		drop()
		fmt.Fprintln(os.Stderr, "cleaner integration: apply global migrations:", err)
		return 1
	}

	code := m.Run()
	itPool.Close()
	drop()
	return code
}

func applyGlobalMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	matches, err := filepath.Glob(filepath.Join("..", "..", "..", "db", "migrations", "global", "*.up.sql"))
	if err != nil {
		return err
	}
	for _, name := range matches {
		sqlBytes, err := os.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func TestIdempotencyCleaner_PurgesExpiredKeysInTenantSchemas(t *testing.T) {
	ctx := context.Background()
	slug := "cleaner_it"

	if _, err := itPool.Exec(ctx, "INSERT INTO tenants (name, slug) VALUES ('Cleaner IT', $1)", slug); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	if err := database.ProvisionTenant(ctx, itPool, slug); err != nil {
		t.Fatalf("provision tenant: %v", err)
	}

	insert := func(key string, expiresAt time.Time) {
		t.Helper()
		err := database.WithTenantSchema(ctx, itPool, slug, func(q db.DBTX) error {
			_, err := q.Exec(ctx, `
				INSERT INTO idempotency_keys (key, endpoint, request_hash, response_status, expires_at)
				VALUES ($1, $2, 'sha256-hash', 201, $3)`, key, "POST /api/v1/appointments", expiresAt)
			return err
		})
		if err != nil {
			t.Fatalf("insert idempotency key %q: %v", key, err)
		}
	}
	count := func(key string) int {
		t.Helper()
		var n int
		err := database.WithTenantSchema(ctx, itPool, slug, func(q db.DBTX) error {
			return q.QueryRow(ctx, "SELECT count(*) FROM idempotency_keys WHERE key = $1", key).Scan(&n)
		})
		if err != nil {
			t.Fatalf("count key %q: %v", key, err)
		}
		return n
	}

	now := time.Now().UTC()
	insert("key-expired", now.Add(-time.Hour))
	insert("key-live", now.Add(time.Hour))

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	NewIdempotencyCleaner(itPool, time.Hour).purge(ctx, log)

	if got := count("key-expired"); got != 0 {
		t.Fatalf("expected expired key purged, still present (%d rows)", got)
	}
	if got := count("key-live"); got != 1 {
		t.Fatalf("expected live key retained, got %d rows", got)
	}
}
