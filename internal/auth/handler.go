package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler is the HTTP transport for the auth module. It binds requests,
// delegates to AuthService, and maps domain errors to status codes. All
// business logic lives in service.go.
type Handler struct {
	svc *AuthService
}

func NewHandler(svc *AuthService) *Handler {
	return &Handler{svc: svc}
}

// bindJSON decodes the request body into dst. Oversized bodies (from
// MaxBody) get a clean 413; malformed or invalid payloads get a clean 400.
func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, errBody{"request body too large"})
			return false
		}
		c.AbortWithStatusJSON(http.StatusBadRequest, errBody{"invalid request body"})
		return false
	}
	return true
}

func respondSession(c *gin.Context, sess *Session) {
	c.JSON(http.StatusOK, tokenResponse{
		AccessToken:  sess.AccessToken,
		RefreshToken: sess.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    sess.ExpiresIn,
		User:         newUserDTO(sess.User, sess.Roles),
	})
}

// writeAuthError maps sentinel domain errors to HTTP responses.
func writeAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrDuplicateEmail):
		c.JSON(http.StatusConflict, errBody{"email already registered"})
	case errors.Is(err, ErrInvalidCredentials):
		c.JSON(http.StatusUnauthorized, errBody{"invalid credentials"})
	case errors.Is(err, ErrAccountDisabled):
		c.JSON(http.StatusUnauthorized, errBody{"account disabled"})
	case errors.Is(err, ErrRefreshRevoked):
		c.JSON(http.StatusUnauthorized, errBody{"refresh token revoked"})
	case errors.Is(err, ErrRefreshInvalid):
		c.JSON(http.StatusUnauthorized, errBody{"invalid or expired refresh token"})
	default:
		c.JSON(http.StatusInternalServerError, errBody{"internal error"})
	}
}

// Register handles POST /api/v1/auth/register.
func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if !bindJSON(c, &req) {
		return
	}
	sess, err := h.svc.Register(c.Request.Context(), req.Email, req.Password, req.FullName, req.Phone)
	if err != nil {
		writeAuthError(c, err)
		return
	}
	respondSession(c, sess)
}

// Login handles POST /api/v1/auth/login.
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if !bindJSON(c, &req) {
		return
	}
	sess, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		writeAuthError(c, err)
		return
	}
	respondSession(c, sess)
}

// Refresh handles POST /api/v1/auth/refresh.
func (h *Handler) Refresh(c *gin.Context) {
	var req refreshRequest
	if !bindJSON(c, &req) {
		return
	}
	sess, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		writeAuthError(c, err)
		return
	}
	respondSession(c, sess)
}

// Logout handles POST /api/v1/auth/logout (requires a valid access token).
func (h *Handler) Logout(c *gin.Context) {
	var req logoutRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, errBody{"failed to revoke refresh token"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}
