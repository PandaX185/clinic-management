package server

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	auth "github.com/PandaX185/clinic-management/internal/auth"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/database"
)

// HeaderTenantID selects the clinic a request acts upon. Login and tenant
// browsing are global; everything else requires this header.
const HeaderTenantID = "X-Tenant-ID"

// TenantResolver resolves a tenant ID to its schema slug. Implemented by
// tenant.Service via the store.
type TenantResolver interface {
	// SlugForTenant validates the tenant exists and is active, returning
	// its slug (the only source of SQL schema names).
	SlugForTenant(ctx context.Context, tenantID uuid.UUID) (string, error)
}

// ProfileResolver returns the caller's roles inside the tenant named by the
// context's tenant slug; an empty slice means no profile yet (patient-level).
type ProfileResolver interface {
	RoleForUser(ctx context.Context, userID uuid.UUID) ([]string, error)
}

// authClaimsFrom recovers parsed token claims stored by auth.Middleware.
func authClaimsFrom(c *gin.Context) *auth.Claims {
	v, ok := c.Get("auth_claims")
	if !ok {
		return nil
	}
	claims, _ := v.(*auth.Claims)
	return claims
}

// CtxTenantSlug is the gin context key carrying the resolved schema slug.
const CtxTenantSlug = "tenant_slug"

var roleCacheTTL = 30 * time.Second

// roleCache caches (user,tenant)->roles so verified roles don't hit Postgres
// per request. Only positive results are cached; revocation is effective
// within TTL.
type roleCache struct {
	mu    sync.RWMutex
	items map[string]cachedRole
}

type cachedRole struct {
	roles   []string
	expires time.Time
}

func newRoleCache() *roleCache { return &roleCache{items: map[string]cachedRole{}} }

func (c *roleCache) get(userID string, tid uuid.UUID) ([]string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.items[userID+":"+tid.String()]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.roles, true
}

func (c *roleCache) set(userID string, tid uuid.UUID, roles []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[userID+":"+tid.String()] = cachedRole{roles: roles, expires: time.Now().Add(roleCacheTTL)}
}

// TenantMiddleware implements the multi-tenant access model:
//
//   - Identity comes from the JWT (global login, no tenant in token).
//   - The active clinic comes from X-Tenant-ID on each request.
//   - Role is looked up in that tenant's profiles table; DB always wins.
//
// A spoofed X-Tenant-ID grants nothing: without a doctor/staff/admin
// profile there, the caller has patient-level rights only — which any user
// legitimately has in every clinic.
func TenantMiddleware(resolver TenantResolver, profiles ProfileResolver) gin.HandlerFunc {
	cache := newRoleCache()

	return func(c *gin.Context) {
		rawID := strings.TrimSpace(c.GetHeader(HeaderTenantID))
		tid, err := uuid.Parse(rawID)
		if err != nil {
			c.Error(apperr.Invalid("missing or invalid " + HeaderTenantID + " header"))
			c.Abort()
			return
		}

		slug, err := resolver.SlugForTenant(c.Request.Context(), tid)
		if err != nil {
			c.Error(err)
			c.Abort()
			return
		}

		claims := authClaimsFrom(c)
		var userID uuid.UUID
		if claims != nil {
			userID = claims.UserID
		} else if rawUID, ok := c.Get(auth.CtxUserID); ok {
			if s, ok := rawUID.(string); ok {
				userID, _ = uuid.Parse(s)
			}
		}

		// Resolve role within this tenant's schema; no profile = patient level.
		roles, cached := cache.get(userID.String(), tid)
		if !cached && userID != (uuid.UUID{}) {
			// Temporarily pin the context so RoleForUser scopes correctly.
			ctx := database.WithTenantSlug(c.Request.Context(), slug)
			roles, err = profiles.RoleForUser(ctx, userID)
			if err != nil {
				c.Error(err)
				c.Abort()
				return
			}
			cache.set(userID.String(), tid, roles)
		}
		if len(roles) == 0 {
			roles = []string{"patient"} // implicit baseline everywhere
		}

		c.Set(auth.CtxRoles, roles)
		c.Set(CtxTenantSlug, slug)
		c.Set("auth_tenant_id", tid)
		c.Next()
	}
}
