package auth

import (
	"testing"
	"time"
)

var testSecret = []byte("test-secret-key")

func TestSignParseRoundtrip(t *testing.T) {
	claims := Claims{
		Subject:   "018f3c1e-1234-7abc-9def-000000000001",
		Email:     "dr@clinic.test",
		Roles:     []string{"doctor", "admin"},
		ID:        "jti-1",
		Type:      TokenTypeAccess,
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(15 * time.Minute).Unix(),
	}
	tok, err := SignToken(testSecret, claims)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	got, err := ParseToken(testSecret, tok)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if got.Subject != claims.Subject || got.Email != claims.Email || got.ID != claims.ID || got.Type != claims.Type {
		t.Fatalf("claims mismatch: %+v", got)
	}
	if len(got.Roles) != 2 || got.Roles[0] != "doctor" || got.Roles[1] != "admin" {
		t.Fatalf("roles mismatch: %v", got.Roles)
	}
}

func TestParseTokenExpired(t *testing.T) {
	tok, _ := SignToken(testSecret, Claims{
		Subject: "u", ID: "j", Type: TokenTypeAccess,
		IssuedAt:  time.Now().Add(-2 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(-1 * time.Hour).Unix(),
	})
	if _, err := ParseToken(testSecret, tok); err != ErrTokenExpired {
		t.Fatalf("want ErrTokenExpired, got %v", err)
	}
}

func TestParseTokenWrongSecret(t *testing.T) {
	tok, _ := SignToken(testSecret, Claims{
		Subject: "u", ID: "j", Type: TokenTypeAccess,
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if _, err := ParseToken([]byte("other-secret"), tok); err == nil {
		t.Fatal("token signed with different secret must not parse")
	}
}

func TestParseTokenTampered(t *testing.T) {
	tok, _ := SignToken(testSecret, Claims{
		Subject: "u", ID: "j", Type: TokenTypeAccess,
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	tampered := tok[:len(tok)-4] + "AAAA" // corrupt the signature segment
	if _, err := ParseToken(testSecret, tampered); err == nil {
		t.Fatal("tampered token must fail")
	}
	if _, err := ParseToken(testSecret, "garbage"); err == nil {
		t.Fatal("malformed token must fail")
	}
}

func TestIssuePair(t *testing.T) {
	access, refresh, jti, err := IssuePair(
		testSecret, []byte("refresh-secret"),
		"user-1", "a@b.c", []string{"patient"},
		15*time.Minute, 168*time.Hour,
	)
	if err != nil {
		t.Fatalf("IssuePair: %v", err)
	}
	ac, err := ParseToken(testSecret, access)
	if err != nil || ac.Type != TokenTypeAccess {
		t.Fatalf("access token parse: %v (type=%v)", err, acType(ac))
	}
	rc, err := ParseToken([]byte("refresh-secret"), refresh)
	if err != nil || rc.Type != TokenTypeRefresh {
		t.Fatalf("refresh token parse failed or wrong type")
	}
	if rc.ID != jti {
		t.Fatalf("refresh jti mismatch: %q vs %q", rc.ID, jti)
	}
	// Cross-type use must be rejected by type checks in middleware/handler.
	if ac.Type == TokenTypeRefresh || rc.Type == TokenTypeAccess {
		t.Fatal("token types must not overlap")
	}
	// Refresh TTL ~168h.
	want := time.Now().Add(168 * time.Hour).Unix()
	if rc.ExpiresAt < want-5 || rc.ExpiresAt > want+5 {
		t.Fatalf("refresh exp not ~168h out: %d", rc.ExpiresAt)
	}
}

func acType(c *Claims) TokenType {
	if c == nil {
		return ""
	}
	return c.Type
}

func TestNewJTIUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		j, err := NewJTI()
		if err != nil {
			t.Fatalf("NewJTI: %v", err)
		}
		if seen[j] {
			t.Fatalf("duplicate jti %q", j)
		}
		seen[j] = true
	}
}
