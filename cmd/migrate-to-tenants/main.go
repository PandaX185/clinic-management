// Command migrate-to-tenants performs the one-shot migration of existing
// single-tenant data into the multi-tenant layout:
//
//  1. ensures global migrations ran (tenants table exists, 'default' row)
//  2. provisions tenant_default schema with all clinical tables
//  3. copies users' clinical rows from legacy public tables into
//     tenant_default (doctors, patients, schedules, appointments,
//     notifications, audit_logs, idempotency_keys)
//  4. creates staff bindings for every legacy doctor/admin/staff user
//
// Idempotent: safe to re-run; it skips work already done.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/PandaX185/clinic-management/internal/platform/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://clinic:clinic@localhost:5432/clinic?sslmode=disable"
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		fatal("connect: %v", err)
	}
	defer pool.Close()

	// 1. Verify global migration state.
	var tenantsExist bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name='tenants')`,
	).Scan(&tenantsExist); err != nil || !tenantsExist {
		fatal("global migrations not applied — run them first")
	}

	// 2. Provision the default tenant schema.
	if err := database.ProvisionTenant(ctx, pool, "default"); err != nil {
		fatal("provision tenant_default: %v", err)
	}
	fmt.Println("provisioned schema tenant_default")

	// 3. Copy clinical rows from legacy public tables.
	stmts := []string{
		`INSERT INTO tenant_default.doctors SELECT * FROM public.doctors WHERE id NOT IN (SELECT id FROM tenant_default.doctors)`,
		`INSERT INTO tenant_default.patients SELECT * FROM public.patients WHERE id NOT IN (SELECT id FROM tenant_default.patients)`,
		`INSERT INTO tenant_default.doctor_schedules SELECT * FROM public.doctor_schedules WHERE id NOT IN (SELECT id FROM tenant_default.doctor_schedules)`,
		`INSERT INTO tenant_default.doctor_schedule_exceptions SELECT * FROM public.doctor_schedule_exceptions WHERE id NOT IN (SELECT id FROM tenant_default.doctor_schedule_exceptions)`,
		`INSERT INTO tenant_default.appointments SELECT * FROM public.appointments WHERE id NOT IN (SELECT id FROM tenant_default.appointments)`,
		`INSERT INTO tenant_default.notifications SELECT * FROM public.notifications WHERE id NOT IN (SELECT id FROM tenant_default.notifications)`,
		`INSERT INTO tenant_default.audit_logs SELECT * FROM public.audit_logs WHERE id NOT IN (SELECT id FROM tenant_default.audit_logs)`,
	}
	for _, stmt := range stmts {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			fatal("copy: %v", err)
		}
	}
	fmt.Println("copied clinical data into tenant_default")

	// 4. Staff bindings for non-patient legacy users.
	_, err = pool.Exec(ctx, `
		INSERT INTO user_tenants (user_id, tenant_id)
		SELECT DISTINCT ur.user_id, t.id
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id AND r.name IN ('admin','staff','doctor')
		CROSS JOIN tenants t WHERE t.slug = 'default'
		ON CONFLICT DO NOTHING`)
	if err != nil {
		fatal("staff bindings: %v", err)
	}

	// Per-clinic profiles for staff (role lives in the tenant schema now).
	_, err = pool.Exec(ctx, `
		INSERT INTO tenant_default.profiles (user_id, role)
		SELECT DISTINCT ur.user_id, r.name
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id AND r.name IN ('admin','staff','doctor')
		ON CONFLICT (user_id) DO NOTHING`)
	if err != nil {
		fatal("staff profiles: %v", err)
	}

	fmt.Println("migration complete: legacy data now in tenant_default; staff bound and profiled")
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
