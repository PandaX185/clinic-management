package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/PandaX185/clinic-management/internal/platform/config"
	"github.com/PandaX185/clinic-management/internal/platform/database"
	natsclient "github.com/PandaX185/clinic-management/internal/platform/nats"
)

type Health struct {
	cfg  config.Config
	pool *database.Pool
	rdb  *redis.Client
	nats *natsclient.Client
}

func NewHealth(cfg config.Config, pool *database.Pool, rdb *redis.Client, nats *natsclient.Client) *Health {
	return &Health{cfg: cfg, pool: pool, rdb: rdb, nats: nats}
}

func (h *Health) RegisterRoutes(r gin.IRouter) {
	r.GET("/health", h.Liveness)
	r.GET("/ready", h.Readiness)
}

func (h *Health) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Readiness reports each dependency separately so orchestration can act
// precisely (NFR-REL-05 / OPS-07). Postgres is critical; losing Redis or
// NATS degrades the service instead of taking it down (NFR-REL-01).
func (h *Health) Readiness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	checks := gin.H{}
	dbOK := h.pool.Ping(ctx) == nil
	checks["postgres"] = statusLabel(dbOK)
	redisOK := false
	if h.rdb != nil {
		redisOK = h.rdb.Ping(ctx).Err() == nil
	}
	checks["redis"] = statusLabel(redisOK)
	natsOK := h.nats != nil && h.nats.Conn != nil && !h.nats.Conn.IsClosed()
	checks["nats"] = statusLabel(natsOK)

	status := http.StatusOK
	state := "ready"
	if !dbOK {
		state = "unhealthy"
		status = http.StatusServiceUnavailable
	} else if !redisOK || !natsOK {
		state = "degraded"
	}

	c.JSON(status, gin.H{"status": state, "checks": checks})
}

func statusLabel(ok bool) string {
	if ok {
		return "ok"
	}
	return "unavailable"
}
