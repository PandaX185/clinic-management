package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func signAccess(t *testing.T, c Claims) string {
	t.Helper()
	tok, err := SignToken(testSecret, c)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	return tok
}

func accessClaims(id string, roles []string) Claims {
	return Claims{
		Subject: id, Email: id + "@x.test", Roles: roles,
		ID: "jti-" + id, Type: TokenTypeAccess,
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(15 * time.Minute).Unix(),
	}
}

func doAuth(t *testing.T, header string) *httptest.ResponseRecorder {
	t.Helper()
	handler := RequireAuth(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := ClaimsFromContext(r.Context())
		if c == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-User", c.Subject)
		if _, ok := UserIDFromContext(r.Context()); !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestRequireAuthTableDriven(t *testing.T) {
	valid := signAccess(t, accessClaims("user-1", []string{"patient"}))
	expiredTok, _ := SignToken(testSecret, Claims{
		Subject: "u", ID: "j", Type: TokenTypeAccess,
		IssuedAt:  time.Now().Add(-2 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(-time.Hour).Unix(),
	})
	refreshTok, _ := SignToken(testSecret, Claims{
		Subject: "u", ID: "j", Type: TokenTypeRefresh,
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"missing header", "", http.StatusUnauthorized},
		{"not bearer", "Basic abc", http.StatusUnauthorized},
		{"empty bearer", "Bearer ", http.StatusUnauthorized},
		{"garbage token", "Bearer not.a.jwt", http.StatusUnauthorized},
		{"expired token", "Bearer " + expiredTok, http.StatusUnauthorized},
		{"refresh token as access", "Bearer " + refreshTok, http.StatusUnauthorized},
		{"wrong secret", func() string { tok, _ := SignToken([]byte("nope"), accessClaims("u", nil)); return tok }(), http.StatusUnauthorized},
		{"valid token", "Bearer " + valid, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doAuth(t, tc.header)
			if rec.Code != tc.want {
				t.Fatalf("want %d, got %d", tc.want, rec.Code)
			}
			if tc.want == http.StatusOK && rec.Header().Get("X-User") != "user-1" {
				t.Fatalf("subject not injected: %q", rec.Header().Get("X-User"))
			}
		})
	}
}

func TestRequireRoleTableDriven(t *testing.T) {
	makeReq := func(roles []string) *http.Request {
		tok := signAccess(t, accessClaims("u", roles))
		claims := accessClaims("u", roles)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		return req.WithContext(contextWithClaims(req.Context(), &claims))
	}

	cases := []struct {
		name    string
		roles   []string
		allowed []string
		want    int
	}{
		{"admin allowed", []string{"admin"}, []string{"admin"}, http.StatusOK},
		{"role subset match", []string{"patient", "admin"}, []string{"admin"}, http.StatusOK},
		{"insufficient role", []string{"patient"}, []string{"admin", "doctor"}, http.StatusForbidden},
		{"no roles at all", nil, []string{"admin"}, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var called bool
			h := RequireRole(tc.allowed...)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, makeReq(tc.roles))
			if rec.Code != tc.want {
				t.Fatalf("want %d, got %d", tc.want, rec.Code)
			}
			if tc.want == http.StatusOK && !called {
				t.Fatal("next handler was not called")
			}
		})
	}
}

func TestRequireRoleWithoutAuthMiddleware(t *testing.T) {
	h := RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without claims in context, got %d", rec.Code)
	}
}
