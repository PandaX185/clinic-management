package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	auth "github.com/PandaX185/clinic-management/internal/auth"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/database"
	"github.com/google/uuid"
)

// --- fakes ---

type fakeTenantResolver struct{ slugs map[uuid.UUID]string }

func (f fakeTenantResolver) SlugForTenant(ctx context.Context, id uuid.UUID) (string, error) {
	s, ok := f.slugs[id]
	if !ok {
		return "", apperr.NotFound("tenant not found")
	}
	return s, nil
}

type fakeProfileResolver struct {
	// (userID, slug) -> roles; missing = no profile
	roles map[string][]string
}

func (f fakeProfileResolver) RoleForUser(ctx context.Context, userID uuid.UUID) ([]string, error) {
	// The middleware pins the slug in ctx before calling us.
	slug := database.TenantSlugFrom(ctx)
	if slug == "" {
		return nil, errNoTenantScope
	}
	return f.roles[userID.String()+":"+slug], nil
}

func setupRouter(resolver TenantResolver, profiles ProfileResolver) (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorHandler(noopLogger{}))
	protected := r.Group("/api/v1")
	protected.Use(func(c *gin.Context) {
		// Simulate auth.Middleware's claims storage.
		c.Set(auth.CtxUserID, c.GetHeader("X-Test-User"))
		c.Next()
	})
	protected.Use(TenantMiddleware(resolver, profiles))
	protected.GET("/ping", func(c *gin.Context) {
		slug := c.GetString(CtxTenantSlug)
		role := ""
		if raw, ok := c.Get(auth.CtxRoles); ok {
			role = raw.([]string)[0]
		}
		c.JSON(http.StatusOK, gin.H{"slug": slug, "role": role})
	})
	w := httptest.NewRecorder()
	return r, w
}

const (
	userID   = "11111111-1111-1111-1111-111111111111"
	tenantA  = "aaaaaaaa-0000-0000-0000-00000000000a"
	tenantB  = "bbbbbbbb-0000-0000-0000-00000000000b"
	tenantCX = "cccccccc-0000-0000-0000-00000000000c" // inactive/unknown
)

func doReq(r *gin.Engine, w *httptest.ResponseRecorder, tenantHeader string) {
	req := httptest.NewRequest("GET", "/api/v1/ping", nil)
	req.Header.Set("X-Test-User", userID)
	if tenantHeader != "" {
		req.Header.Set(HeaderTenantID, tenantHeader)
	}
	r.ServeHTTP(w, req)
}

// SEC: a request without a tenant header is rejected outright.
func TestTenantMiddleware_MissingHeader(t *testing.T) {
	resolver := fakeTenantResolver{slugs: map[uuid.UUID]string{}}
	r, w := setupRouter(resolver, &fakeProfileResolver{roles: map[string][]string{}})
	doReq(r, w, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing X-Tenant-ID, got %d", w.Code)
	}
}

// Unknown or inactive tenants are rejected before any data access.
func TestTenantMiddleware_UnknownTenant(t *testing.T) {
	resolver := fakeTenantResolver{slugs: map[uuid.UUID]string{
		uuid.MustParse(tenantA): "clinic_a",
	}}
	r, w := setupRouter(resolver, &fakeProfileResolver{roles: map[string][]string{}})
	doReq(r, w, tenantCX)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown tenant, got %d", w.Code)
	}
}

// No profile in the clinic => patient-level access, correct schema pinned.
func TestTenantMiddleware_NoProfileIsPatient(t *testing.T) {
	resolver := fakeTenantResolver{slugs: map[uuid.UUID]string{
		uuid.MustParse(tenantA): "clinic_a",
	}}
	r, w := setupRouter(resolver, &fakeProfileResolver{roles: map[string][]string{}})
	doReq(r, w, tenantA)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), `"role":"patient"`) || !contains(w.Body.String(), `"slug":"clinic_a"`) {
		t.Fatalf("expected patient role + clinic_a slug, got %s", w.Body.String())
	}
}

// Staff role is resolved per-clinic from that clinic's profiles table.
func TestTenantMiddleware_StaffInOwnClinicOnly(t *testing.T) {
	resolver := fakeTenantResolver{slugs: map[uuid.UUID]string{
		uuid.MustParse(tenantA): "clinic_a",
		uuid.MustParse(tenantB): "clinic_b",
	}}
	profiles := &fakeProfileResolver{roles: map[string][]string{
		userID + ":clinic_a": {"staff"}, // staff only at clinic A
	}}
	r, _ := setupRouter(resolver, profiles)

	// Clinic A -> staff
	w := httptest.NewRecorder()
	doReq(r, w, tenantA)
	if !contains(w.Body.String(), `"role":"staff"`) {
		t.Fatalf("expected staff at clinic_a, got %s", w.Body.String())
	}

	// Clinic B -> falls back to patient (their profile doesn't exist there)
	w2 := httptest.NewRecorder()
	doReq(r, w2, tenantB)
	if !contains(w2.Body.String(), `"role":"patient"`) {
		t.Fatalf("expected patient at clinic_b, got %s", w2.Body.String())
	}
}

// The role cache must not leak roles across tenants for the same user.
func TestRoleCache_KeyedByTenant(t *testing.T) {
	c := newRoleCache()
	uid := uuid.MustParse(userID)
	tidA := uuid.MustParse(tenantA)
	tidB := uuid.MustParse(tenantB)

	c.set(uid.String(), tidA, []string{"staff"})
	if role, ok := c.get(uid.String(), tidB); ok && len(role) == 1 && role[0] == "staff" {
		t.Fatal("staff role leaked to another tenant via cache")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

var _ = time.Now // keep time import if unused after edits

var errNoTenantScope = &testError{"no tenant scope in context"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

type noopLogger struct{}

func (noopLogger) Error(string, ...any) {}

func init() { roleCacheTTL = 0 } // tests: bypass role cache
