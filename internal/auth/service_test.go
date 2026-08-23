package auth

import (
	"context"
	"errors"
	"testing"
)

// Service-level tests: business rules without any HTTP involved.
// Transport behavior (binding/status mapping) is covered in handler_test.go.

func TestRegisterIssuesSession(t *testing.T) {
	h, _, _ := newTestHandler()
	sess, err := h.svc.Register(context.Background(), "  Alice@Example.COM ", "password123", "Alice", "")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if sess.User.Email != "alice@example.com" {
		t.Errorf("email not normalized: %q", sess.User.Email)
	}
	if sess.AccessToken == "" || sess.RefreshToken == "" {
		t.Error("expected both tokens issued")
	}
	if sess.Roles == nil {
		t.Error("roles slice must be non-nil (JSON encodes as [])")
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	h, _, _ := newTestHandler()
	ctx := context.Background()
	if _, err := h.svc.Register(ctx, "a@x.test", "password123", "A", ""); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	_, err := h.svc.Register(ctx, "a@x.test", "password123", "A", "")
	if !errors.Is(err, ErrDuplicateEmail) {
		t.Errorf("want ErrDuplicateEmail, got %v", err)
	}
}

func TestLoginNoUserEnumeration(t *testing.T) {
	h, users, _ := newTestHandler()
	ctx := context.Background()
	if _, err := h.svc.Register(ctx, "a@x.test", "correct-horse", "A", ""); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if _, err := h.svc.Login(ctx, "ghost@x.test", "correct-horse"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("unknown email: want ErrInvalidCredentials, got %v", err)
	}
	if _, err := h.svc.Login(ctx, "a@x.test", "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("wrong password: want ErrInvalidCredentials, got %v", err)
	}

	users.users["a@x.test"].IsActive = false
	if _, err := h.svc.Login(ctx, "a@x.test", "correct-horse"); !errors.Is(err, ErrAccountDisabled) {
		t.Errorf("disabled account: want ErrAccountDisabled, got %v", err)
	}
}

func TestServiceRefreshRotation(t *testing.T) {
	h, _, tokens := newTestHandler()
	ctx := context.Background()

	sess1, err := h.svc.Register(ctx, "a@x.test", "password123", "A", "")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	sess2, err := h.svc.Refresh(ctx, sess1.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if sess2.RefreshToken == sess1.RefreshToken {
		t.Error("refresh token must rotate")
	}

	// Old token is consumed: replay must fail as revoked.
	if _, err := h.svc.Refresh(ctx, sess1.RefreshToken); !errors.Is(err, ErrRefreshRevoked) {
		t.Errorf("replay: want ErrRefreshRevoked, got %v", err)
	}

	// Logout revokes the current token.
	if err := h.svc.Logout(ctx, sess2.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, ok := tokens.tokens[""]; ok {
		t.Error("unexpected empty jti entry")
	}
	if _, err := h.svc.Refresh(ctx, sess2.RefreshToken); !errors.Is(err, ErrRefreshRevoked) {
		t.Errorf("post-logout refresh: want ErrRefreshRevoked, got %v", err)
	}
}

func TestLogoutIsIdempotent(t *testing.T) {
	h, _, _ := newTestHandler()
	if err := h.svc.Logout(context.Background(), "garbage-token"); err != nil {
		t.Errorf("logout with invalid token should succeed silently, got %v", err)
	}
}
