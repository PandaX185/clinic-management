package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// claimsKey is the gin context key under which RequireAuth stores claims.
const claimsKey = "claims"

// ClaimsFromCtx returns the JWT claims injected by RequireAuth, or nil.
func ClaimsFromCtx(c *gin.Context) *Claims {
	v, ok := c.Get(claimsKey)
	if !ok {
		return nil
	}
	claims, _ := v.(*Claims)
	return claims
}

// UserIDFromCtx returns the authenticated user's ID, if any.
func UserIDFromCtx(c *gin.Context) (string, bool) {
	claims := ClaimsFromCtx(c)
	if claims == nil {
		return "", false
	}
	return claims.Subject, true
}

// RequireAuth parses the Bearer access token, validates signature and expiry,
// and injects the claims into the gin context. Responds 401 on failure.
func RequireAuth(accessSecret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c.Request)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errBody{"missing or malformed Authorization header"})
			return
		}
		claims, err := ParseToken(accessSecret, token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errBody{"invalid or expired token"})
			return
		}
		if claims.Type != TokenTypeAccess {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errBody{"invalid token type"})
			return
		}
		c.Set(claimsKey, claims)
		c.Next()
	}
}

// RequireRole allows the request through only when the authenticated user has
// at least one of the allowed roles; otherwise responds 403.
func RequireRole(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := ClaimsFromCtx(c)
		if claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errBody{"authentication required"})
			return
		}
		for _, have := range claims.Roles {
			for _, want := range allowed {
				if have == want {
					c.Next()
					return
				}
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, errBody{"insufficient permissions"})
	}
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

// MaxBodyBytes caps JSON request bodies at 1MB.
const MaxBodyBytes int64 = 1 << 20

// MaxBody is a global body-limit middleware that wraps c.Request.Body in
// http.MaxBytesReader so oversized requests are rejected with 413 instead of
// being read fully.
func MaxBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}
