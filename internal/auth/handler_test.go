package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ---- in-memory fakes ----

type fakeUserStore struct {
	mu    sync.Mutex
	users map[string]*User // by email
	roles map[string][]string
}

func newFakeUsers() *fakeUserStore {
	return &fakeUserStore{users: map[string]*User{}, roles: map[string][]string{}}
}

func (f *fakeUserStore) CreateUser(_ context.Context, email, hash, fullName, phone string) (*User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.users[email]; ok {
		return nil, ErrDuplicateEmail
	}
	u := &User{ID: uuid.New(), Email: email, PasswordHash: hash, FullName: fullName, Phone: phone, IsActive: true}
	f.users[email] = u
	return u, nil
}

func (f *fakeUserStore) GetUserByEmail(_ context.Context, email string) (*User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.users[email]; ok {
		return u, nil
	}
	return nil, fmt.Errorf("not found")
}

func (f *fakeUserStore) GetUserRoles(_ context.Context, id uuid.UUID) ([]string, error) {
	return nil, nil
}

type fakeTokenStore struct {
	mu     sync.Mutex
	tokens map[string]string // jti -> userID
}

func newFakeTokens() *fakeTokenStore { return &fakeTokenStore{tokens: map[string]string{}} }

func (f *fakeTokenStore) SaveRefresh(_ context.Context, jti, userID string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens[jti] = userID
	return nil
}
func (f *fakeTokenStore) ConsumeRefresh(_ context.Context, jti string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	userID, ok := f.tokens[jti]
	if !ok {
		return "", ErrRefreshNotFound
	}
	delete(f.tokens, jti)
	return userID, nil
}
func (f *fakeTokenStore) DeleteRefresh(_ context.Context, jti string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.tokens, jti)
	return nil
}

// ---- helpers ----

var testCfg = Config{
	AccessSecret:  []byte("access-secret"),
	RefreshSecret: []byte("refresh-secret"),
	AccessTTL:     15 * time.Minute,
	RefreshTTL:    168 * time.Hour,
}

func newTestHandler() (*Handler, *fakeUserStore, *fakeTokenStore) {
	users, tokens := newFakeUsers(), newFakeTokens()
	return NewHandler(users, tokens, testCfg), users, tokens
}

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestRouter builds a gin engine wired exactly like cmd/api/router.go,
// with logout behind a stub auth middleware that injects fixed claims.
func newTestRouter(h *Handler) *gin.Engine {
	r := gin.New()
	authPub := r.Group("/auth")
	{
		authPub.POST("/register", h.Register)
		authPub.POST("/login", h.Login)
		authPub.POST("/refresh", h.Refresh)
	}
	protected := r.Group("/auth", func(c *gin.Context) {
		claims := accessClaims("someone", []string{"patient"})
		c.Set(claimsKey, &claims)
		c.Next()
	})
	protected.POST("/logout", h.Logout)
	return r
}

func postJSON(t *testing.T, r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return m
}

const validRegister = `{"email":"alice@example.com","password":"supersecret1","full_name":"Alice Doe","phone":"+155****1111"}`

// login performs register+login and returns the token response body.
func login(t *testing.T, r *gin.Engine) map[string]any {
	t.Helper()
	if rec := postJSON(t, r, "/auth/register", validRegister); rec.Code != http.StatusOK {
		t.Fatalf("register failed: %d %s", rec.Code, rec.Body.String())
	}
	rec := postJSON(t, r, "/auth/login", `{"email":"alice@example.com","password":"supersecret1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
	return decodeBody(t, rec)
}

// ---- tests ----

func TestRegister(t *testing.T) {
	h, users, _ := newTestHandler()
	r := newTestRouter(h)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"valid", validRegister, http.StatusOK},
		{"duplicate email", validRegister, http.StatusConflict},
		{"invalid json", `{`, http.StatusBadRequest},
		{"bad email", `{"email":"nope","password":"supersecret1","full_name":"X"}`, http.StatusBadRequest},
		{"short password", `{"email":"b@x.com","password":"short","full_name":"X"}`, http.StatusBadRequest},
		{"missing name", `{"email":"b@x.com","password":"supersecret1","full_name":""}`, http.StatusBadRequest},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postJSON(t, r, "/auth/register", tc.body)
			if rec.Code != tc.want {
				t.Fatalf("want %d got %d: %s", tc.want, rec.Code, rec.Body.String())
			}
			if tc.want == http.StatusOK && i == 0 {
				body := decodeBody(t, rec)
				if body["access_token"] == "" || body["refresh_token"] == "" {
					t.Fatal("register must return token pair")
				}
				if _, ok := users.users["alice@example.com"]; !ok {
					t.Fatal("user not persisted")
				}
			}
		})
	}
}

func TestLoginTableDriven(t *testing.T) {
	h, _, _ := newTestHandler()
	r := newTestRouter(h)
	if rec := postJSON(t, r, "/auth/register", validRegister); rec.Code != http.StatusOK {
		t.Fatalf("setup register failed: %s", rec.Body.String())
	}

	cases := []struct {
		name string
		body string
		want int
	}{
		{"valid credentials", `{"email":"alice@example.com","password":"supersecret1"}`, http.StatusOK},
		{"wrong password", `{"email":"alice@example.com","password":"wrong-password"}`, http.StatusUnauthorized},
		{"unknown email", `{"email":"ghost@example.com","password":"supersecret1"}`, http.StatusUnauthorized},
		{"malformed json", `{"email":`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postJSON(t, r, "/auth/login", tc.body)
			if rec.Code != tc.want {
				t.Fatalf("want %d got %d: %s", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestRefreshRotationAndRevocation(t *testing.T) {
	h, _, tokens := newTestHandler()
	r := newTestRouter(h)
	first := login(t, r)

	oldRefresh := first["refresh_token"].(string)

	// Rotate.
	rec := postJSON(t, r, "/auth/refresh", `{"refresh_token":"`+oldRefresh+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh failed: %d %s", rec.Code, rec.Body.String())
	}
	second := decodeBody(t, rec)
	if second["access_token"] == first["access_token"] {
		t.Fatal("new access token must differ after rotation")
	}
	if second["refresh_token"] == oldRefresh {
		t.Fatal("refresh token must rotate")
	}

	// Old refresh token must now be revoked.
	rec = postJSON(t, r, "/auth/refresh", `{"refresh_token":"`+oldRefresh+`"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("reused refresh token must be revoked, got %d", rec.Code)
	}

	// New refresh token still works once more (then gets rotated again).
	rec = postJSON(t, r, "/auth/refresh", `{"refresh_token":"`+second["refresh_token"].(string)+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("fresh refresh token should work: %d %s", rec.Code, rec.Body.String())
	}
	// Both consumed refresh tokens (login's and the first rotation) must be
	// gone; only the register-issued session and the latest rotation remain.
	if _, ok := tokens.tokens[jtiOf(t, oldRefresh)]; ok {
		t.Fatal("old refresh jti should have been deleted from the store")
	}
}

// jtiOf parses a refresh token and returns its jti claim.
func jtiOf(t *testing.T, tok string) string {
	t.Helper()
	c, err := ParseToken([]byte("refresh-secret"), tok)
	if err != nil {
		t.Fatalf("parse refresh token: %v", err)
	}
	return c.ID
}

func TestLogoutRevokesRefresh(t *testing.T) {
	h, _, _ := newTestHandler()
	r := newTestRouter(h)
	body := login(t, r)
	refresh := body["refresh_token"].(string)

	// Logout route sits behind the stub claims-injecting middleware in
	// newTestRouter, mirroring RequireAuth in production.
	rec := postJSON(t, r, "/auth/logout", `{"refresh_token":"`+refresh+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout failed: %d %s", rec.Code, rec.Body.String())
	}

	// Refresh after logout must be rejected.
	rec = postJSON(t, r, "/auth/refresh", `{"refresh_token":"`+refresh+`"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout must be 401, got %d", rec.Code)
	}
}

func TestRefreshInvalidInputs(t *testing.T) {
	h, _, _ := newTestHandler()
	r := newTestRouter(h)
	login(t, r)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"missing field", `{}`, http.StatusBadRequest},
		{"garbage token", `{"refresh_token":"abc.def.ghi"}`, http.StatusUnauthorized},
		{"access token used as refresh", func() string {
			tok, _ := SignToken(testCfg.AccessSecret, accessClaims("x", nil))
			return `{"refresh_token":"` + tok + `"}`
		}(), http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postJSON(t, r, "/auth/refresh", tc.body)
			if rec.Code != tc.want {
				t.Fatalf("want %d got %d: %s", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestRefreshConcurrentSingleWinner(t *testing.T) {
	h, _, _ := newTestHandler()
	r := newTestRouter(h)
	body := login(t, r)
	refresh := body["refresh_token"].(string)

	const n = 8 // more contenders than 2 to stress the race
	var wg sync.WaitGroup
	codes := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/auth/refresh",
				strings.NewReader(`{"refresh_token":"`+refresh+`"}`))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(rec, req)
			codes[i] = rec.Code
		}(i)
	}
	wg.Wait()

	okCount, unauthCount := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusOK:
			okCount++
		case http.StatusUnauthorized:
			unauthCount++
		default:
			t.Fatalf("unexpected status %d", c)
		}
	}
	if okCount != 1 || unauthCount != n-1 {
		t.Fatalf("expected exactly 1 x 200 and %d x 401, got ok=%d unauth=%d (codes=%v)", n-1, okCount, unauthCount, codes)
	}
}

func TestRequestBodyTooLarge(t *testing.T) {
	h, _, _ := newTestHandler()
	r := gin.New()
	r.Use(MaxBody(MaxBodyBytes))
	r.POST("/auth/login", h.Login)

	big := strings.Repeat("a", int(MaxBodyBytes)+1024)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"`+big+`@example.com","password":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body must be 413, got %d: %s", rec.Code, rec.Body.String())
	}
}
