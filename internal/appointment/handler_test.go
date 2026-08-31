package appointment

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	auth "github.com/PandaX185/clinic-management/internal/auth"
)

// newGateRouter mounts the appointment routes behind a stub auth that injects
// a fixed role set, isolating the RequireRoles gate from the service layer.
// The handlers short-circuit on the role guard before touching the (nil)
// service, so a 403 proves the gate; anything else proves the gate passed.
func newGateRouter(roles ...string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(auth.CtxUserID, "11111111-1111-1111-1111-111111111111")
		c.Set(auth.CtxRoles, roles)
		c.Next()
	})
	h := NewHandler(nil)
	h.RegisterRoutes(r.Group("/api/v1"))
	return r
}

// P0 regression: a patient must NOT reach confirm/complete/no-show — those
// are staff/clinic transitions.
func TestStaffOnlyMutations_BlockedForPatient(t *testing.T) {
	r := newGateRouter("patient")
	for _, path := range []string{
		"/api/v1/appointments/x/confirm",
		"/api/v1/appointments/x/complete",
		"/api/v1/appointments/x/no-show",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for patient on %s, got %d", path, w.Code)
		}
	}
}

// P0 regression: an admin passes the gate (role guard allows; the nil service
// would only matter past the gate, which is not what we assert here).
func TestStaffOnlyMutations_AllowedForAdmin(t *testing.T) {
	r := newGateRouter("admin")
	for _, path := range []string{
		"/api/v1/appointments/x/confirm",
		"/api/v1/appointments/x/complete",
		"/api/v1/appointments/x/no-show",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
		if w.Code == http.StatusForbidden {
			t.Fatalf("admin wrongly blocked on %s (got 403)", path)
		}
	}
}

// Patient self-service (cancel/reschedule own appointment) is not role-gated.
func TestPatientSelfService_Reachable(t *testing.T) {
	r := newGateRouter("patient")
	for _, path := range []string{
		"/api/v1/appointments/x/cancel",
		"/api/v1/appointments/x/reschedule",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
		if w.Code == http.StatusForbidden {
			t.Fatalf("patient wrongly blocked on %s (got 403)", path)
		}
	}
}

var _ = json.Marshal
