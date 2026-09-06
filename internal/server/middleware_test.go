package server

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/metrics"
)

func TestBodyLimit_CapsRequestBody(t *testing.T) {
	r := gin.New()
	gin.SetMode(gin.TestMode)
	r.Use(BodyLimit(8))
	r.POST("/echo", func(c *gin.Context) {
		if _, err := io.ReadAll(c.Request.Body); err != nil {
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("0123456789")))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body status = %d, want 413", w.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	r := gin.New()
	gin.SetMode(gin.TestMode)
	r.Use(SecurityHeaders())
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	for _, h := range []string{"X-Content-Type-Options", "X-Frame-Options", "Cache-Control"} {
		if w.Header().Get(h) == "" {
			t.Errorf("missing security header %s", h)
		}
	}
}

func TestRequestID_PropagatesOrGenerates(t *testing.T) {
	r := gin.New()
	gin.SetMode(gin.TestMode)
	r.Use(RequestID())
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "my-id")
	r.ServeHTTP(w, req)
	if got := w.Header().Get("X-Request-ID"); got != "my-id" {
		t.Fatalf("expected propagated id, got %q", got)
	}

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := w2.Header().Get("X-Request-ID"); got == "" {
		t.Fatal("expected generated request id")
	}
}

func TestMetricsMiddleware_RecordsRouteAndStatus(t *testing.T) {
	m := metrics.New()
	r := gin.New()
	r.Use(MetricsMiddleware(m))
	r.GET("/things/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/things/1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	// Unmatched paths must be labelled "unmatched" instead of panicking on
	// an empty FullPath().
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/no/such/route", nil))
	if w2.Code != http.StatusNotFound {
		t.Fatalf("unmatched status = %d", w2.Code)
	}

	reqCounter := m.HTTPRequestsTotal.With(prometheus.Labels{"method": "GET", "route": "/things/:id", "status": "200"})
	if got := testutil.ToFloat64(reqCounter); got != 1 {
		t.Fatalf("matched route counter = %v, want 1", got)
	}
	unmatched := m.HTTPRequestsTotal.With(prometheus.Labels{"method": "GET", "route": "unmatched", "status": "404"})
	if got := testutil.ToFloat64(unmatched); got != 1 {
		t.Fatalf("unmatched counter = %v, want 1", got)
	}
}

func TestRateLimit_GracefulDegradation(t *testing.T) {
	r := gin.New()
	gin.SetMode(gin.TestMode)
	// Nil Redis and disabled limiter both must let requests through.
	r.Use(RateLimit(nil, 10))
	r.Use(RateLimit(&redis.Client{}, 0))
	hit := false
	r.GET("/", func(c *gin.Context) { hit = true; c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK || !hit {
		t.Fatalf("rate limit degraded incorrectly: %d hit=%v", w.Code, hit)
	}
}

func TestErrorHandler_MapsTypedErrors(t *testing.T) {
	r := gin.New()
	gin.SetMode(gin.TestMode)
	r.Use(ErrorHandler(noopLogger{}))
	r.GET("/conflict", func(c *gin.Context) {
		_ = c.Error(apperr.Conflict("slot taken"))
	})
	r.GET("/internal", func(c *gin.Context) {
		_ = c.Error(apperr.Internal(errors.New("secrets")))
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/conflict", nil))
	if w.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d, want 409", w.Code)
	}
	if !strings.Contains(w.Body.String(), "slot taken") {
		t.Fatalf("body = %s", w.Body.String())
	}

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/internal", nil))
	if w2.Code != http.StatusInternalServerError {
		t.Fatalf("internal status = %d, want 500", w2.Code)
	}
	if strings.Contains(w2.Body.String(), "secrets") {
		t.Fatal("internal error details leaked to the client")
	}
	if !strings.Contains(w2.Body.String(), "internal server error") {
		t.Fatalf("body = %s", w2.Body.String())
	}
}

func TestErrorHandler_LogsInternalErrorsWithRequestID(t *testing.T) {
	var logged atomic.Value
	r := gin.New()
	gin.SetMode(gin.TestMode)
	r.Use(RequestID())
	r.Use(ErrorHandler(loggerFunc{fn: func(args ...any) {
		var b bytes.Buffer
		for _, a := range args {
			b.WriteString(stringify(a))
			b.WriteByte(' ')
		}
		logged.Store(b.String())
	}}))
	r.GET("/fail", func(c *gin.Context) { _ = c.Error(apperr.Internal(errors.New("x"))) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/fail", nil))
	_ = w
	if got := logged.Load(); got == nil || !strings.Contains(got.(string), "request_id") {
		t.Fatalf("expected log line with request_id, got %v", got)
	}
}

func stringify(a any) string {
	if s, ok := a.(string); ok {
		return s
	}
	return ""
}

type loggerFunc struct{ fn func(args ...any) }

func (l loggerFunc) Error(msg string, args ...any) { l.fn(append([]any{msg}, args...)...) }
