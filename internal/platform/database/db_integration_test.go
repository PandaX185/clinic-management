// Integration tests against a real PostgreSQL that prove the migration
// pipeline and the database-level invariants (BR-01 booking exclusion, derived
// queue positions) actually hold on committed SQL. Requires a running Postgres
// (docker compose up). Each run creates and drops a throwaway database
// (clinic_it_<hex>) so the dev database is never touched.
package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/PandaX185/lahza/internal/platform/db/sqlc"
)

var itPool *pgxpool.Pool

const itSlug = "acme_it"

func TestMain(m *testing.M) {
	os.Exit(runIntegration(m))
}

func runIntegration(m *testing.M) int {
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
		fmt.Fprintln(os.Stderr, "integration tests require a running Postgres (docker compose up):", err)
		return 1
	}
	defer admin.Close()

	testDB := "clinic_it_" + randHex(4)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+testDB); err != nil {
		fmt.Fprintln(os.Stderr, "integration: create database:", err)
		return 1
	}
	drop := func() { _, _ = admin.Exec(context.Background(), "DROP DATABASE "+testDB+" WITH (FORCE)") }

	cfg, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		drop()
		fmt.Fprintln(os.Stderr, "integration: parse url:", err)
		return 1
	}
	cfg.ConnConfig.Database = testDB
	itPool, err = pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		drop()
		fmt.Fprintln(os.Stderr, "integration: connect test db:", err)
		return 1
	}

	if err := applyGlobalMigrations(ctx, itPool); err != nil {
		itPool.Close()
		drop()
		fmt.Fprintln(os.Stderr, "integration: apply global migrations:", err)
		return 1
	}

	code := m.Run()
	itPool.Close()
	drop()
	return code
}

// applyGlobalMigrations executes db/migrations/global/*.up.sql in lexical
// order. With no arguments pgx uses the simple protocol, so multi-statement
// files (CREATE FUNCTION bodies included) apply in a single Exec.
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

// ---- tests -------------------------------------------------------------

func TestGlobalMigrations_CreateDependencies(t *testing.T) {
	ctx := context.Background()

	var gist, crypto string
	err := itPool.QueryRow(ctx, "SELECT (SELECT extname FROM pg_extension WHERE extname='btree_gist'), (SELECT extname FROM pg_extension WHERE extname='pgcrypto')").Scan(&gist, &crypto)
	if err != nil {
		t.Fatalf("query extensions: %v", err)
	}
	if gist != "btree_gist" || crypto != "pgcrypto" {
		t.Fatalf("expected btree_gist & pgcrypto, got %q / %q", gist, crypto)
	}

	var v uuid.UUID
	if err := itPool.QueryRow(ctx, "SELECT uuid_generate_v7()").Scan(&v); err != nil {
		t.Fatalf("uuid_generate_v7 not available: %v", err)
	}
	if v == uuid.Nil {
		t.Fatal("uuid_generate_v7 returned nil")
	}

	var defaultTenant int
	if err := itPool.QueryRow(ctx, "SELECT count(*) FROM tenants WHERE slug='default'").Scan(&defaultTenant); err != nil {
		t.Fatal(err)
	}
	if defaultTenant != 1 {
		t.Fatalf("expected seeded default tenant, got %d", defaultTenant)
	}
}

func TestProvisionTenant_CreatesClinicalSchema(t *testing.T) {
	ctx := context.Background()
	if _, err := itPool.Exec(ctx, "INSERT INTO tenants (name, slug) VALUES ('Acme IT', $1)", itSlug); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	if err := ProvisionTenant(ctx, itPool, itSlug); err != nil {
		t.Fatalf("provision tenant: %v", err)
	}

	scope := func(fn func(q db.DBTX) error) error {
		return WithTenantSchema(ctx, itPool, itSlug, fn)
	}
	needs := []string{"profiles", "appointments", "queue_entries", "doctor_schedules", "schedule_exceptions", "idempotency_keys"}
	err := scope(func(q db.DBTX) error {
		for _, tbl := range needs {
			var exists bool
			if err := q.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", "tenant_"+itSlug+".\""+tbl+"\"").Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("clinical table %s missing after provisioning", tbl)
			}
		}
		var roles int
		if err := q.QueryRow(ctx, "SELECT count(*) FROM roles").Scan(&roles); err != nil {
			return err
		}
		if roles != 6 {
			return fmt.Errorf("expected 6 seeded roles, got %d", roles)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAppointmentExclusionConstraint_RejectsOverlap(t *testing.T) {
	ctx := context.Background()
	doctor, patient, apptType := seedBooking(t)
	start := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Minute)

	insert := func(s, e time.Time) error {
		return WithTenantSchema(ctx, itPool, itSlug, func(q db.DBTX) error {
			_, err := q.Exec(ctx,
				`INSERT INTO appointments
				 (profile_id, doctor_profile_id, appointment_type_id, scheduled_start, scheduled_end, status, version)
				 VALUES ($1,$2,$3,$4,$5,'scheduled',1)`,
				patient, doctor, apptType, s, e)
			return err
		})
	}

	if err := insert(start, start.Add(30*time.Minute)); err != nil {
		t.Fatalf("first slot should insert: %v", err)
	}

	// BR-01: an overlapping active slot must be rejected by the database.
	err := insert(start.Add(15*time.Minute), start.Add(45*time.Minute))
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23P01" {
		t.Fatalf("expected exclusion violation 23P01 for overlap, got %v", err)
	}

	// An adjacent, non-overlapping slot is fine.
	if err := insert(start.Add(30*time.Minute), start.Add(60*time.Minute)); err != nil {
		t.Fatalf("adjacent slot should insert: %v", err)
	}

	// Cancelling the first frees the range (status leaves the accepted set).
	cancelFirst := func() error {
		return WithTenantSchema(ctx, itPool, itSlug, func(q db.DBTX) error {
			_, err := q.Exec(ctx,
				`UPDATE appointments SET status='cancelled', cancellation_reason='it'
				 WHERE scheduled_start=$1 AND doctor_profile_id=$2`, start, doctor)
			return err
		})
	}
	if err := cancelFirst(); err != nil {
		t.Fatalf("cancel first: %v", err)
	}
	if err := insert(start, start.Add(30*time.Minute)); err != nil {
		t.Fatalf("range should be reusable after cancellation: %v", err)
	}
}

func TestQueuePosition_DerivedAndCollapses(t *testing.T) {
	ctx := context.Background()
	_, patient, _ := seedBooking(t)

	now := time.Now().UTC()
	entry := func(checkedIn time.Time) uuid.UUID {
		var id uuid.UUID
		err := WithTenantSchema(ctx, itPool, itSlug, func(q db.DBTX) error {
			return q.QueryRow(ctx,
				`INSERT INTO queue_entries (profile_id, checked_in_at) VALUES ($1,$2) RETURNING id`,
				patient, checkedIn).Scan(&id)
		})
		if err != nil {
			t.Fatalf("insert queue entry: %v", err)
		}
		return id
	}

	old := entry(now.Add(-2 * time.Minute))
	mid := entry(now.Add(-1 * time.Minute))
	new := entry(now)

	list := func() []db.ListActiveQueueRow {
		var rows []db.ListActiveQueueRow
		err := WithTenantSchema(ctx, itPool, itSlug, func(q db.DBTX) error {
			var err error
			rows, err = db.New(q.(pgx.Tx)).ListActiveQueue(ctx, db.ListActiveQueueParams{
				From:   timePtr(time.Unix(0, 0)),
				Offset: 0,
				Limit:  100,
			})
			return err
		})
		if err != nil {
			t.Fatalf("list active queue: %v", err)
		}
		return rows
	}

	rows := list()
	if len(rows) != 3 {
		t.Fatalf("expected 3 waiting entries, got %d", len(rows))
	}
	// Position is derived from (priority DESC, checked_in_at ASC).
	expectOrder(t, rows, []uuid.UUID{old, mid, new})
	for i, r := range rows {
		if w, g := int64(i+1), r.Position; w != g {
			t.Errorf("entry %s: position = %d, want %d", r.ID, g, w)
		}
		if w, g := int64(3), r.ActiveTotal; w != g {
			t.Errorf("entry %s: active_total = %d, want %d", r.ID, g, w)
		}
	}

	// Completing the first collapses the remaining positions.
	err := WithTenantSchema(ctx, itPool, itSlug, func(q db.DBTX) error {
		_, err := db.New(q.(pgx.Tx)).UpdateQueueEntryStatus(ctx, db.UpdateQueueEntryStatusParams{ID: old, Column2: "completed"})
		return err
	})
	if err != nil {
		t.Fatalf("complete first: %v", err)
	}

	rows = list()
	if len(rows) != 2 {
		t.Fatalf("expected 2 remaining, got %d", len(rows))
	}
	for i, r := range rows {
		if w, g := int64(i+1), r.Position; w != g {
			t.Errorf("after completion entry %s: position = %d, want %d", r.ID, g, w)
		}
		if w, g := int64(2), r.ActiveTotal; w != g {
			t.Errorf("after completion entry %s: active_total = %d, want %d", r.ID, g, w)
		}
	}
}

// ---- helpers -----------------------------------------------------------

// seedBooking inserts a global user plus a doctor profile, a patient profile
// and an appointment type inside the test tenant; returns the ids.
func seedBooking(t *testing.T) (doctor, patient, apptType uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	var userID uuid.UUID
	err := itPool.QueryRow(ctx,
		"INSERT INTO users (phone, password_hash, full_name) VALUES ($1,'hash','IT User') RETURNING id",
		"+1999000"+randHex(3)).Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	insertProfile := func(display string) uuid.UUID {
		// profiles.user_id is UNIQUE; each profile gets its own user.
		var uid uuid.UUID
		if err := itPool.QueryRow(ctx,
			"INSERT INTO users (phone, password_hash, full_name) VALUES ($1,'hash',$2) RETURNING id",
			"+1999000"+randHex(3), display).Scan(&uid); err != nil {
			t.Fatalf("insert profile user: %v", err)
		}
		var id uuid.UUID
		err := WithTenantSchema(ctx, itPool, itSlug, func(q db.DBTX) error {
			return q.QueryRow(ctx,
				"INSERT INTO profiles (user_id, display_name) VALUES ($1,$2) RETURNING id",
				uid, display).Scan(&id)
		})
		if err != nil {
			t.Fatalf("insert profile %s: %v", display, err)
		}
		return id
	}

	doctor = insertProfile("IT Doctor")
	patient = insertProfile("IT Patient")

	err = WithTenantSchema(ctx, itPool, itSlug, func(q db.DBTX) error {
		var id uuid.UUID
		if err := q.QueryRow(ctx,
			"INSERT INTO appointment_types (name, duration_minutes, price) VALUES ('Checkup', 30, 0) RETURNING id").Scan(&id); err != nil {
			return err
		}
		apptType = id
		return nil
	})
	if err != nil {
		t.Fatalf("insert appointment type: %v", err)
	}
	return doctor, patient, apptType
}

func expectOrder(t *testing.T, rows []db.ListActiveQueueRow, want []uuid.UUID) {
	t.Helper()
	for i, w := range want {
		if rows[i].ID != w {
			t.Errorf("row %d = %s, want %s (order must be checked_in_at ASC)", i, rows[i].ID, w)
		}
	}
}

func timePtr(t time.Time) *time.Time { return &t }
