package repo

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/PandaX185/lahza/internal/platform/database"
	db "github.com/PandaX185/lahza/internal/platform/db/sqlc"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IdempotencyCleaner periodically purges expired idempotency-key rows so the
// table stays bounded (BR-07). Interval is derived from the key TTL.
type IdempotencyCleaner struct {
	pool     *pgxpool.Pool
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}
}

func NewIdempotencyCleaner(pool *pgxpool.Pool, interval time.Duration) *IdempotencyCleaner {
	if interval <= 0 {
		interval = time.Hour
	}
	return &IdempotencyCleaner{
		pool:     pool,
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Run blocks until Stop is called or ctx is cancelled; run it in a goroutine.
func (c *IdempotencyCleaner) Run(ctx context.Context, log *slog.Logger) {
	defer close(c.done)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stop:
			return
		case <-ticker.C:
			c.purge(ctx, log)
		}
	}
}

// purge deletes expired idempotency keys from every tenant schema. The keys
// live inside each clinic's schema (tenant_<slug>), never in public, so the
// sweep must scope the search path per schema rather than touching the raw
// pool — a raw-pool DELETE targets public.idempotency_keys and no-ops.
func (c *IdempotencyCleaner) purge(ctx context.Context, log *slog.Logger) {
	schemas, err := database.ExistingTenantSchemas(ctx, c.pool)
	if err != nil {
		log.Error("idempotency cleanup: list tenant schemas failed", "error", err.Error())
		return
	}

	var purged int64
	for schema := range schemas {
		slug := strings.TrimPrefix(schema, database.TenantSchemaPrefix)
		n, err := c.purgeSchema(ctx, slug)
		if err != nil {
			log.Error("idempotency cleanup failed", "schema", schema, "error", err.Error())
			continue
		}
		purged += n
	}
	if purged > 0 {
		log.Info("purged expired idempotency keys", "count", purged)
	}
}

func (c *IdempotencyCleaner) purgeSchema(ctx context.Context, slug string) (int64, error) {
	var n int64
	err := database.WithTenantSchema(ctx, c.pool, slug, func(q db.DBTX) error {
		var err error
		n, err = db.New(q).DeleteExpiredIdempotencyKeys(ctx)
		return err
	})
	return n, err
}

// Stop signals the cleaner to exit and waits for it.
func (c *IdempotencyCleaner) Stop() {
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
	<-c.done
}
