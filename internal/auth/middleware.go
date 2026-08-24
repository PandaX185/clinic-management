package auth

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

func Middleware(svc *Service) gin.HandlerFunc {
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
		c.Set(CtxRoles, claims.Roles)
		c.Set("auth_claims", claims)
		c.Next()
	}
}

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
