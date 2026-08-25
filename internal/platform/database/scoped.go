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
	defer tx.Rollback(ctx)

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
