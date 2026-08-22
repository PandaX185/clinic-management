package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Config carries the tunables for the auth handler.
type Config struct {
	AccessSecret  []byte
	RefreshSecret []byte
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
}

type Handler struct {
	users  UserStore
	tokens TokenStore
	cfg    Config
}

func NewHandler(users UserStore, tokens TokenStore, cfg Config) *Handler {
	return &Handler{users: users, tokens: tokens, cfg: cfg}
}

// errBody is the uniform JSON error envelope.
type errBody struct {
	Error string `json:"error"`
}

type tokenResponse struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	TokenType    string  `json:"token_type"`
	ExpiresIn    int64   `json:"expires_in"` // access TTL in seconds
	User         userDTO `json:"user"`
}

type userDTO struct {
	ID       uuid.UUID `json:"id"`
	Email    string    `json:"email"`
	FullName string    `json:"full_name"`
	Phone    string    `json:"phone,omitempty"`
	Roles    []string  `json:"roles"`
}

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	FullName string `json:"full_name" binding:"required"`
	Phone    string `json:"phone"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
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

// Register handles POST /api/v1/auth/register.
func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if !bindJSON(c, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	hash, err := HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errBody{"failed to hash password"})
		return
	}
	user, err := h.users.CreateUser(c.Request.Context(), req.Email, hash, req.FullName, req.Phone)
	if err != nil {
		if errors.Is(err, ErrDuplicateEmail) {
			c.JSON(http.StatusConflict, errBody{"email already registered"})
			return
		}
		c.JSON(http.StatusInternalServerError, errBody{"failed to create user"})
		return
	}
	_ = h.issueAndSave(c, user.ID.String(), user.Email)
}

// Login handles POST /api/v1/auth/login.
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if !bindJSON(c, &req) {
		return
	}
	user, err := h.users.GetUserByEmail(c.Request.Context(), strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil || VerifyPassword(req.Password, user.PasswordHash) != nil {
		// Same error for unknown email and wrong password (no user enumeration).
		c.JSON(http.StatusUnauthorized, errBody{"invalid credentials"})
		return
	}
	if !user.IsActive {
		c.JSON(http.StatusUnauthorized, errBody{"account disabled"})
		return
	}
	_ = h.issueAndSave(c, user.ID.String(), user.Email)
}

// Refresh handles POST /api/v1/auth/refresh. Validates the refresh token,
// checks it has not been revoked in Redis, then rotates: deletes the old jti
// and issues a fresh pair.
func (h *Handler) Refresh(c *gin.Context) {
	var req refreshRequest
	if !bindJSON(c, &req) {
		return
	}
	claims, err := ParseToken(h.cfg.RefreshSecret, req.RefreshToken)
	if err != nil || claims.Type != TokenTypeRefresh {
		c.JSON(http.StatusUnauthorized, errBody{"invalid or expired refresh token"})
		return
	}
	// Atomically check-and-consume the refresh token so that two concurrent
	// requests with the same token cannot both rotate (GETDEL semantics).
	storedUserID, err := h.tokens.ConsumeRefresh(c.Request.Context(), claims.ID)
	if err != nil {
		if errors.Is(err, ErrRefreshNotFound) {
			c.JSON(http.StatusUnauthorized, errBody{"refresh token revoked"})
			return
		}
		c.JSON(http.StatusInternalServerError, errBody{"token store unavailable"})
		return
	}
	if storedUserID != claims.Subject {
		c.JSON(http.StatusUnauthorized, errBody{"refresh token revoked"})
		return
	}
	_ = h.issueAndSave(c, claims.Subject, claims.Email)
}

// Logout handles POST /api/v1/auth/logout (requires a valid access token).
// Revokes the supplied refresh token.
func (h *Handler) Logout(c *gin.Context) {
	var req logoutRequest
	if !bindJSON(c, &req) {
		return
	}
	claims, err := ParseToken(h.cfg.RefreshSecret, req.RefreshToken)
	if err == nil && claims.Type == TokenTypeRefresh {
		if err := h.tokens.DeleteRefresh(c.Request.Context(), claims.ID); err != nil {
			c.JSON(http.StatusInternalServerError, errBody{"failed to revoke refresh token"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

// issueAndSave mints a token pair, persists the refresh jti in Redis, and
// writes the JSON response.
func (h *Handler) issueAndSave(c *gin.Context, userID, email string) error {
	roles, err := h.users.GetUserRoles(c.Request.Context(), uuid.MustParse(userID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, errBody{"failed to load roles"})
		return err
	}
	if roles == nil {
		roles = []string{}
	}

	access, refresh, refreshJTI, err := IssuePair(
		h.cfg.AccessSecret, h.cfg.RefreshSecret,
		userID, email, roles,
		h.cfg.AccessTTL, h.cfg.RefreshTTL,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errBody{"failed to issue tokens"})
		return err
	}
	if err := h.tokens.SaveRefresh(c.Request.Context(), refreshJTI, userID, h.cfg.RefreshTTL); err != nil {
		c.JSON(http.StatusInternalServerError, errBody{"failed to persist refresh token"})
		return err
	}

	uid := uuid.MustParse(userID)
	fullName, phone := "", ""
	if u, _ := h.users.GetUserByEmail(c.Request.Context(), email); u != nil && u.ID == uid {
		fullName, phone = u.FullName, u.Phone
	}

	c.JSON(http.StatusOK, tokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(h.cfg.AccessTTL.Seconds()),
		User:         userDTO{ID: uid, Email: email, FullName: fullName, Phone: phone, Roles: roles},
	})
	return nil
}
