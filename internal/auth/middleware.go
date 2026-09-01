package auth

import (
	"context"
	"strings"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GlobalAdminChecker resolves the global super-admin flag. It is distinct
// from per-tenant roles (TenantMiddleware + RequireRoles); this gate governs
// registry-level operations like provisioning a clinic.
type GlobalAdminChecker interface {
	IsGlobalAdmin(ctx context.Context, userID uuid.UUID) (bool, error)
}

const (
	CtxUserID = "auth_user_id"
	CtxRoles  = "auth_roles"
	CtxClaims = "auth_claims"
)

// JwtMiddleware verifies the JWT access token and extracts the user ID.
// Roles are NOT in the JWT — they're resolved per-request by TenantMiddleware
// using the X-Tenant-ID header and the tenant's profiles table.
func JwtMiddleware(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(apperr.HTTPStatus(apperr.KindUnauthorized), gin.H{
				"error": "missing or malformed authorization header",
			})
			return
		}
		claims, err := svc.ParseAccessToken(strings.TrimPrefix(header, "Bearer "))
		if err != nil || claims.Type != TokenTypeAccess {
			c.AbortWithStatusJSON(apperr.HTTPStatus(apperr.KindUnauthorized), gin.H{
				"error": "invalid or expired token",
			})
			return
		}
		c.Set(CtxUserID, claims.UserID.String())
		c.Set("auth_claims", claims)
		c.Next()
	}
}

// RequireRoles checks that the caller has one of the required roles.
// These are set by TenantMiddleware from the tenant-scoped DB query,
// not from the JWT.
func RequireRoles(roles ...string) gin.HandlerFunc {
	required := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		required[r] = struct{}{}
	}
	return func(c *gin.Context) {
		raw, exists := c.Get(CtxRoles)
		if !exists {
			c.AbortWithStatusJSON(apperr.HTTPStatus(apperr.KindForbidden), gin.H{"error": "insufficient permissions"})
			return
		}
		userRoles, _ := raw.([]string)
		for _, ur := range userRoles {
			if _, ok := required[ur]; ok {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(apperr.HTTPStatus(apperr.KindForbidden), gin.H{"error": "insufficient permissions"})
	}
}

func UserIDFrom(c *gin.Context) (string, bool) {
	v, ok := c.Get(CtxUserID)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// RequireGlobalAdmin denies the request unless the authenticated caller is a
// global super-admin (users.is_admin). Must be composed AFTER JwtMiddleware so
// auth_user_id is populated. Unlike RequireRoles it needs no X-Tenant-ID
// because it checks a global flag, not a per-tenant profile.
func RequireGlobalAdmin(checker GlobalAdminChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, err := uuid.Parse(c.GetString(CtxUserID))
		if err != nil {
			c.AbortWithStatusJSON(apperr.HTTPStatus(apperr.KindForbidden), gin.H{"error": "insufficient permissions"})
			return
		}
		admin, err := checker.IsGlobalAdmin(c.Request.Context(), uid)
		if err != nil || !admin {
			c.AbortWithStatusJSON(apperr.HTTPStatus(apperr.KindForbidden), gin.H{"error": "insufficient permissions"})
			return
		}
		c.Next()
	}
}
