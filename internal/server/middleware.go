package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/metrics"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		c.Set("request_id", id)
		c.Writer.Header().Set("X-Request-ID", id)
		c.Next()
	}
}

func MetricsMiddleware(m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := strconv.Itoa(c.Writer.Status())
		m.HTTPRequestsTotal.WithLabelValues(c.Request.Method, route, status).Inc()
		m.HTTPRequestLatency.WithLabelValues(c.Request.Method, route).Observe(time.Since(start).Seconds())
	}
}

// RateLimit is a fixed-window limiter backed by Redis; when Redis is
// unavailable the request proceeds (graceful degradation, NFR-REL-01).
func RateLimit(rdb *redis.Client, perMinute int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rdb == nil || perMinute <= 0 {
			c.Next()
			return
		}
		key := "ratelimit:" + c.ClientIP()
		ctx := c.Request.Context()

		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			c.Next()
			return
		}
		if count == 1 {
			rdb.Expire(ctx, key, time.Minute)
		}
		if count > int64(perMinute) {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}

// ErrorHandler converts typed domain errors into consistent JSON responses.
// Internal errors log details but never leak them to clients (SEC-06).
func ErrorHandler(logger Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		err := c.Errors.Last().Err
		appErr := apperr.From(err)
		if appErr.Kind == apperr.KindInternal {
			logger.Error("request failed", "error", err.Error(), "path", c.Request.URL.Path, "request_id", requestIDOf(c))
		}
		c.JSON(apperr.HTTPStatus(appErr.Kind), gin.H{"error": publicMessage(appErr)})
	}
}

type Logger interface {
	Error(msg string, args ...any)
}

func publicMessage(e *apperr.Error) string {
	if e.Kind == apperr.KindInternal {
		return "internal server error"
	}
	if e.Err != nil && e.Message == "unexpected failure" {
		return e.Message
	}
	return e.Message
}

func requestIDOf(c *gin.Context) string {
	v, ok := c.Get("request_id")
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
