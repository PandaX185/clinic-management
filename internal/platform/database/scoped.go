package database

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ScopedQuerier yields a db handle whose search_path is pinned to one
// tenant's schema for the lifetime of the returned commit function.
//
// Usage pattern in repositories:
//
//	repo.withSchema(ctx, slug, func(q *db.Queries) error { ... })
type ScopedPool struct {
	pool *pgxpool.Pool
}

func NewScopedPool(pool *pgxpool.Pool) *ScopedPool { return &ScopedPool{pool: pool} }

// WithSchema runs fn with a transaction scoped to the tenant schema. SET
// LOCAL semantics via set_config(..., true) mean the search_path reverts on
// commit/rollback, so pooled connections never leak tenant context.
func (p *ScopedPool) WithSchema(ctx context.Context, slug string, fn func(tx pgx.Tx) error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		"SELECT set_config('search_path', $1, true)",
		SchemaName(slug)+", public",
	); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Pool exposes the underlying pool for global-schema (public) queries.
func (p *ScopedPool) Pool() *pgxpool.Pool { return p.pool }

// ExistingTenantSchemas returns the set of tenant schema names that actually
// exist in the database. Fan-out scans use it to skip active tenants whose
// schema was never provisioned (e.g. pre-schema-era records), which would
// otherwise fail every query against a missing schema.
func ExistingTenantSchemas(ctx context.Context, pool *pgxpool.Pool) (map[string]struct{}, error) {
	// Literal underscore in LIKE needs an escape (default backslash), so the
	// pattern is built as "tenant" + "\_%": only schemas named tenant_<slug>.
	rows, err := pool.Query(ctx, `SELECT nspname FROM pg_catalog.pg_namespace WHERE nspname LIKE $1 ORDER BY nspname`, "tenant"+`\_%`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		seen[name] = struct{}{}
	}
	return seen, rows.Err()
}
