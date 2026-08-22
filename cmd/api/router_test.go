package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/axiom/clinic-appointment/internal/auth"
	"github.com/axiom/clinic-appointment/internal/platform/config"
	"github.com/axiom/clinic-appointment/internal/platform/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ---- fakes ----

type stubUsers struct{}

func (stubUsers) CreateUser(_ context.Context, email, _, fullName, _ string) (*auth.User, error) {
	return &auth.User{ID: uuid.New(), Email: email, FullName: fullName, IsActive: true}, nil
}
func (stubUsers) GetUserByEmail(_ context.Context, _ string) (*auth.User, error) {
	return nil, fmt.Errorf("not found")
}
func (stubUsers) GetUserRoles(_ context.Context, _ uuid.UUID) ([]string, error) { return nil, nil }

type stubTokens struct{}

func (stubTokens) SaveRefresh(_ context.Context, _, _ string, _ time.Duration) error { return nil }
func (stubTokens) ConsumeRefresh(_ context.Context, _ string) (string, error) {
	return "", auth.ErrRefreshNotFound
}
func (stubTokens) DeleteRefresh(_ context.Context, _ string) error { return nil }

// ---- helpers ----

var testConfig = &config.Config{
	Port:             "0",
	JWTSecret:        "test-access-secret",
	JWTRefreshSecret: "test-refresh-secret",
	JWTAccessTTL:     15 * time.Minute,
	JWTRefreshTTL:    168 * time.Hour,
}

// healthyPinger is a Pinger stub that always succeeds.
type healthyPinger struct{}

func (healthyPinger) Ping(_ context.Context) error { return nil }

func newTestServer() *gin.Engine {
	gin.SetMode(gin.TestMode)
	logr := logger.New("error")
	return newRouter(logr, stubUsers{}, stubTokens{}, testConfig, healthyPinger{}, healthyPinger{})
}

func doGet(r *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func doPost(r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestRouteTable asserts every registered route by path+method+expected
// status. This pins the route contract; drift fails here.
func TestRouteTable(t *testing.T) {
	r := newTestServer()

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"health", http.MethodGet, "/health", "", http.StatusOK},
		{"ready", http.MethodGet, "/ready", "", http.StatusOK},
		{"metrics", http.MethodGet, "/metrics", "", http.StatusOK},
		{"register", http.MethodPost, "/api/v1/auth/register", `{}`, http.StatusBadRequest},
		{"login", http.MethodPost, "/api/v1/auth/login", `{}`, http.StatusBadRequest},
		{"refresh", http.MethodPost, "/api/v1/auth/refresh", `{}`, http.StatusBadRequest},
		{"logout unauthenticated", http.MethodPost, "/api/v1/auth/logout", `{}`, http.StatusUnauthorized},
		{"unknown api route 404s", http.MethodGet, "/api/v1/patients", "", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rec *httptest.ResponseRecorder
			if tc.method == http.MethodGet {
				rec = doGet(r, tc.path)
			} else {
				rec = doPost(r, tc.path, tc.body)
			}
			if rec.Code != tc.want {
				t.Fatalf("%s %s: want %d got %d: %s", tc.method, tc.path, tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestHealthReadyMetrics covers the ops endpoints with stubbed dependencies.
func TestHealthReadyMetrics(t *testing.T) {
	r := newTestServer()

	if rec := doGet(r, "/health"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Fatalf("/health must be 200 ok, got %d %s", rec.Code, rec.Body.String())
	}
	if rec := doGet(r, "/ready"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ready"`) {
		t.Fatalf("/ready must be 200 ready with healthy deps, got %d %s", rec.Code, rec.Body.String())
	}

	// /metrics must be scrapeable and expose the promauto request counter.
	doGet(r, "/api/v1/nope") // generate one request through the metrics middleware
	rec := doGet(r, "/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics must be 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "http_requests_total") {
		t.Fatal("/metrics body must contain http_requests_total")
	}
}

// TestReadyDegraded checks the dependency-failure path.
func TestReadyDegraded(t *testing.T) {
	failing := failingPinger{}
	r := newRouter(logger.New("error"), stubUsers{}, stubTokens{}, testConfig, failing, healthyPinger{})
	if rec := doGet(r, "/ready"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/ready with failing db must be 503, got %d", rec.Code)
	}
}

type failingPinger struct{}

func (failingPinger) Ping(_ context.Context) error { return fmt.Errorf("down") }
