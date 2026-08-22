package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordFormat(t *testing.T) {
	h, err := HashPassword("s3cret-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=") {
		t.Fatalf("unexpected PHC prefix: %q", h[:20])
	}
	if !strings.Contains(h, ",t=3,p=4$") {
		t.Fatalf("missing argon2 params in %q", h)
	}
	parts := strings.Split(h, "$")
	if len(parts) != 6 || parts[4] == "" || parts[5] == "" {
		t.Fatalf("expected salt and hash segments in PHC string: %q", h)
	}
}

func TestPasswordRoundtrip(t *testing.T) {
	cases := []string{"correct horse battery staple", "p@ssw0rd!", "üñíçödé-påsswörd"}
	for _, pw := range cases {
		h, err := HashPassword(pw)
		if err != nil {
			t.Fatalf("HashPassword(%q): %v", pw, err)
		}
		if err := VerifyPassword(pw, h); err != nil {
			t.Errorf("VerifyPassword(correct) for %q: %v", pw, err)
		}
		if err := VerifyPassword(pw+"-wrong", h); err == nil {
			t.Errorf("VerifyPassword(wrong) for %q should fail", pw)
		}
	}
}

func TestVerifyPasswordUniqueSalts(t *testing.T) {
	h1, _ := HashPassword("same")
	h2, _ := HashPassword("same")
	if h1 == h2 {
		t.Fatal("two hashes of the same password must differ (random salt)")
	}
}

func TestVerifyPasswordTamperedHash(t *testing.T) {
	h, _ := HashPassword("hunter2")
	bad := h[:len(h)-3] + "aaa"
	if err := VerifyPassword("hunter2", bad); err == nil {
		t.Fatal("tampered hash must not verify")
	}
	if err := VerifyPassword("hunter2", "not-a-phc-string"); err == nil {
		t.Fatal("malformed hash must not verify")
	}
}
