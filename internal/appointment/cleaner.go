package appointment

import (
	"context"
	"time"

	db "github.com/PandaX185/clinic-management/internal/platform/db/sqlc"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
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
func (c *IdempotencyCleaner) Run(ctx context.Context, log *zap.Logger) {
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
			n, err := db.New(c.pool).DeleteExpiredIdempotencyKeys(ctx)
			if err != nil {
				log.Error("idempotency key cleanup failed", zap.Error(err))
				continue
			}
			if n > 0 {
				log.Info("purged expired idempotency keys", zap.Int64("count", n))
			}
		}
	}
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
