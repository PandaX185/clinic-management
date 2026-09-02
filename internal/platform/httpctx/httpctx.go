// Package httpctx centralizes gin request-context helpers shared across
// HTTP handler layers: identity claims written by the auth/JWT middleware
// and UUID parsing from request inputs.
package httpctx

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

// Gin context keys written by the auth/JWT middleware and read by feature
// handlers. Keeping them here lets every HTTP layer share one source of
// truth without importing the auth feature package.
const (
	CtxUserID  = "auth_user_id"
	CtxRoles   = "auth_roles"
	CtxClaims  = "auth_claims"
)

// ParseUUID parses a remote id string, mapping malformed input to a 400.
func ParseUUID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, apperr.Invalid("invalid id format")
	}
	return id, nil
}

// ParseUUIDParam parses the path parameter named in c, defaulting to "id".
func ParseUUIDParam(c *gin.Context, name string) (uuid.UUID, error) {
	if name == "" {
		name = "id"
	}
	return ParseUUID(c.Param(name))
}

// UserID returns the authenticated user id set by JwtMiddleware.
func UserID(c *gin.Context) (uuid.UUID, error) {
	v, ok := c.Get(CtxUserID)
	if !ok {
		return uuid.Nil, apperr.Unauthorized("unauthenticated")
	}
	s, ok := v.(string)
	if !ok {
		return uuid.Nil, apperr.Invalid("invalid user id")
	}
	return ParseUUID(s)
}

// Roles returns the tenant-scoped roles set by TenantMiddleware.
func Roles(c *gin.Context) []string {
	raw, ok := c.Get(CtxRoles)
	if !ok {
		return nil
	}
	roles, _ := raw.([]string)
	return roles
}