package apperr

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestConstructorsSetKindAndMessage(t *testing.T) {
	cases := []struct {
		name string
		got  *Error
		kind Kind
		msg  string
		http int
	}{
		{"invalid", Invalid("bad"), KindInvalid, "bad", http.StatusBadRequest},
		{"notfound", NotFound("missing"), KindNotFound, "missing", http.StatusNotFound},
		{"conflict", Conflict("taken"), KindConflict, "taken", http.StatusConflict},
		{"unauthorized", Unauthorized("login"), KindUnauthorized, "login", http.StatusUnauthorized},
		{"forbidden", Forbidden("nope"), KindForbidden, "nope", http.StatusForbidden},
	}
	for _, tc := range cases {
		if tc.got.Kind != tc.kind || tc.got.Message != tc.msg {
			t.Errorf("%s: kind=%v msg=%q", tc.name, tc.got.Kind, tc.got.Message)
		}
		if got := HTTPStatus(tc.got.Kind); got != tc.http {
			t.Errorf("%s: HTTPStatus = %d, want %d", tc.name, got, tc.http)
		}
	}
}

func TestInternalMapsTo500(t *testing.T) {
	if got := HTTPStatus(Internal(errors.New("boom")).Kind); got != http.StatusInternalServerError {
		t.Fatalf("HTTPStatus = %d, want 500", got)
	}
	if got := HTTPStatus(Kind(999)); got != http.StatusInternalServerError {
		t.Fatalf("unknown kind should fall back to 500, got %d", got)
	}
}

func TestErrorTextAndUnwrap(t *testing.T) {
	base := errors.New("db down")
	e := Wrap(KindInternal, base)
	if e.Error() != "unexpected failure: db down" {
		t.Errorf("Error() = %q", e.Error())
	}
	if !errors.Is(e, base) {
		t.Fatal("Unwrap must expose the wrapped error")
	}
	if e.Message != "unexpected failure" {
		t.Fatal("Wrap should set the generic placeholder message")
	}
	if got := New(KindInvalid, "hi").Error(); got != "hi" {
		t.Errorf("bare error text = %q", got)
	}
}

func TestFromClassifies(t *testing.T) {
	if e := From(NotFound("gone")); e.Kind != KindNotFound {
		t.Fatalf("expected KindNotFound, got %v", e.Kind)
	}
	wrapped := errors.New("will wrap")
	if e := From(wrapped); e.Kind != KindInternal {
		t.Fatalf("plain errors must wrap as internal, got %v", e.Kind)
	}
	if e := From(nil); e == nil {
		t.Fatal("From(nil) must still return an error for the error handler")
	}

	// Kind must survive being wrapped in another error (errors.As unwraps).
	if e := From(fmt.Errorf("outer: %w", Conflict("x"))); e.Kind != KindConflict {
		t.Fatalf("nested conflict lost, got %v", e.Kind)
	}
}
