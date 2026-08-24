package server

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	auth "github.com/PandaX185/clinic-management/internal/auth"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

// authClaimsFrom recovers the parsed token claims stored by auth.Middleware.
func authClaimsFrom(c *gin.Context) *auth.Claims {
	v, ok := c.Get("auth_claims")
	if !ok {
		return nil
	}
	claims, _ := v.(*auth.Claims)
	return claims
}

// MembershipChecker validates a user's active membership in the tenant
// claimed by their token. Implemented by tenant.Service.
type MembershipChecker interface {
	CheckMembership(userID, tenantID string) (role, slug string, err error)
}

// CachedMembership wraps a checker with a short-lived positive cache so
// verified memberships don't hit Postgres on every request. Negative results
// are never cached: a newly granted user must work immediately.
type CachedMembership struct {
	checker MembershipChecker
	ttl     time.Duration

	mu    sync.RWMutex
	cache map[string]cachedEntry
}

type cachedEntry struct {
	role    string
	slug    string
	expires time.Time
}

func NewCachedMembership(checker MembershipChecker, ttl time.Duration) *CachedMembership {
	return &CachedMembership{checker: checker, ttl: ttl, cache: make(map[string]cachedEntry)}
}

func membershipCacheKey(userID, tenantID string) string { return userID + ":" + tenantID }

func (c *CachedMembership) CheckMembership(userID, tenantID string) (string, string, error) {
	key := membershipCacheKey(userID, tenantID)
	c.mu.RLock()
	if e, ok := c.cache[key]; ok && time.Now().Before(e.expires) {
		c.mu.RUnlock()
		return e.role, e.slug, nil
	}
	c.mu.RUnlock()

	role, slug, err := c.checker.CheckMembership(userID, tenantID)
	if err != nil {
		return "", "", err
	}
	c.mu.Lock()
	c.cache[key] = cachedEntry{role: role, slug: slug, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()
	return role, slug, nil
}

// CtxTenantSlug is the gin context key carrying the resolved schema slug.
const CtxTenantSlug = "tenant_slug"

// TenantMiddleware enforces that the caller's access token names a clinic
// they hold an active membership in (SEC: cross-tenant isolation). The
// verified slug lands on the request context for DB scoping.
func TenantMiddleware(checker MembershipChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawUID, ok := c.Get(auth.CtxUserID)
		if !ok {
			c.AbortWithStatusJSON(apperr.HTTPStatus(apperr.KindUnauthorized), gin.H{"error": "unauthenticated"})
			return
		}
		userID, _ := rawUID.(string)

		claims := authClaimsFrom(c)
		if claims == nil || claims.TenantID == uuid.Nil {
			c.AbortWithStatusJSON(apperr.HTTPStatus(apperr.KindForbidden), gin.H{
				"error": "no clinic selected; use POST /api/v1/auth/select-tenant",
			})
			return
		}

		role, slug, err := checker.CheckMembership(userID, claims.TenantID.String())
		if err != nil {
			c.Error(err)
			c.Abort()
			return
		}
		// The token's role claim is advisory only; the DB membership row is
		// authoritative and always wins.
		c.Set(auth.CtxRoles, []string{role})
		c.Set(CtxTenantSlug, slug)
		c.Next()
	}
}
