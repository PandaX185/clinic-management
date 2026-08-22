package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type contextKey string

const claimsKey contextKey = "auth_claims"

// ClaimsFromContext returns the JWT claims injected by RequireAuth, or nil.
func ClaimsFromContext(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsKey).(*Claims)
	return c
}

// UserIDFromContext returns the authenticated user's ID, if any.
func UserIDFromContext(ctx context.Context) (string, bool) {
	c := ClaimsFromContext(ctx)
	if c == nil {
		return "", false
	}
	return c.Subject, true
}

// RequireAuth parses the Bearer access token, validates signature and expiry,
// and injects the claims into the request context. Responds 401 on failure.
func RequireAuth(accessSecret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				writeError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
				return
			}
			claims, err := ParseToken(accessSecret, token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}
			if claims.Type != TokenTypeAccess {
				writeError(w, http.StatusUnauthorized, "invalid token type")
				return
			}
			next.ServeHTTP(w, r.WithContext(contextWithClaims(r.Context(), claims)))
		})
	}
}

// RequireRole allows the request through only when the authenticated user has
// at least one of the allowed roles; otherwise responds 403.
func RequireRole(allowed ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				writeError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			for _, have := range claims.Roles {
				for _, want := range allowed {
					if have == want {
						next.ServeHTTP(w, r)
						return
					}
				}
			}
			writeError(w, http.StatusForbidden, "insufficient permissions")
		})
	}
}

func contextWithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(h, prefix))
	if token == "" {
		return "", false
	}
	return token, true
}

var ErrNoClaims = errors.New("auth: no claims in context")

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":` + jsonString(msg) + `}`))
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
