// Package database — multi-tenant provisioning and schema management.
//
// Strategy: schema-per-tenant. Global identity lives in `public`; every
// clinic gets its own schema (`tenant_<slug>`) holding all clinical tables.
// Tenant schemas are provisioned by executing the embedded tenant migration
// SQL files in lexical order inside the new schema.
package database

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"regexp"

	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed all:migrations
var tenantMigrations embed.FS

var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// ValidSlug reports whether s may be used as part of a schema identifier.
func ValidSlug(s string) bool { return slugPattern.MatchString(s) }

// SchemaName maps a validated tenant slug to its Postgres schema name.
func SchemaName(slug string) string { return "tenant_" + slug }

func sortedMigrationFiles(fsys embed.FS) ([]string, error) {
	matches, err := fs.Glob(fsys, "migrations/*.up.sql")
	if err != nil {
		return nil, err
	}
	// fs.Glob returns sorted results.
	return matches, nil
}

// ProvisionTenant creates the physical schema for a tenant and applies the
// embedded clinical migrations inside it. The tenants row must already
// exist; call within or after the transaction that inserted it.
func ProvisionTenant(ctx context.Context, pool *pgxpool.Pool, slug string) error {
	if !ValidSlug(slug) {
		return fmt.Errorf("invalid tenant slug %q", slug)
	}
	schema := SchemaName(slug)

	files, err := sortedMigrationFiles(tenantMigrations)
	if err != nil {
		return fmt.Errorf("list tenant migrations: %w", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin provisioning tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Identifier is built only from a regex-validated slug, never raw input.
	if _, err := tx.Exec(ctx, fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schema)); err != nil {
		return fmt.Errorf("create schema %s: %w", schema, err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL search_path TO %s, public", schema)); err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}

	for _, name := range files {
		sqlBytes, err := tenantMigrations.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s to %s: %w", name, schema, err)
		}
	}

	return tx.Commit(ctx)
}

// DropTenant removes a clinic's schema entirely (GDPR-style erasure).
// The tenants row must be removed by the caller.
func DropTenant(ctx context.Context, pool *pgxpool.Pool, slug string) error {
	if !ValidSlug(slug) {
		return fmt.Errorf("invalid tenant slug %q", slug)
	}
	_, err := pool.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", SchemaName(slug)))
	return err
}

// ListTenantSchemas returns slugs of all provisioned tenant schemas by
// diffing the tenants table against information_schema.
func ListTenantSchemas(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := pool.Query(ctx,
		`SELECT t.slug FROM tenants t
		 JOIN pg_namespace n ON n.nspname = 'tenant_' || t.slug
		 WHERE t.is_active ORDER BY t.slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var slugs []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		slugs = append(slugs, s)
	}
	return slugs, rows.Err()
}

// TenantConn wraps a transaction bound to one tenant's schema. All sqlc
// queries executed through it resolve unqualified table names against the
// tenant schema first (set_config local resets automatically on commit).
type TenantConn struct {
	schema string
}

// WithTenantSchema runs fn with a transaction whose search_path targets the
// given tenant schema. This is the single choke point through which every
// tenant-scoped DB operation must flow.
func WithTenantSchema(ctx context.Context, pool *pgxpool.Pool, slug string, fn func(q db.DBTX) error) error {
	if !ValidSlug(slug) {
		return fmt.Errorf("invalid tenant slug %q", slug)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tenant tx: %w", err)
	}
	defer tx.Rollback(ctx)

	schema := SchemaName(slug)
	if _, err := tx.Exec(ctx, "SELECT set_config('search_path', $1, true)", schema+", public"); err != nil {
		return fmt.Errorf("scope search_path to %s: %w", schema, err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
