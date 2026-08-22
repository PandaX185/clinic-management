package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

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
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Register handles POST /api/v1/auth/register.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if _, err := mail.ParseAddress(req.Email); err != nil || !strings.Contains(req.Email, "@") {
		writeError(w, http.StatusBadRequest, "valid email is required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if strings.TrimSpace(req.FullName) == "" {
		writeError(w, http.StatusBadRequest, "full_name is required")
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	user, err := h.users.CreateUser(r.Context(), req.Email, hash, req.FullName, req.Phone)
	if err != nil {
		if errors.Is(err, ErrDuplicateEmail) {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	if err := h.issueAndSave(w, r, user.ID.String(), user.Email); err != nil {
		return
	}
}

// Login handles POST /api/v1/auth/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	user, err := h.users.GetUserByEmail(r.Context(), strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil || VerifyPassword(req.Password, user.PasswordHash) != nil {
		// Same error for unknown email and wrong password (no user enumeration).
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !user.IsActive {
		writeError(w, http.StatusUnauthorized, "account disabled")
		return
	}
	if err := h.issueAndSave(w, r, user.ID.String(), user.Email); err != nil {
		return
	}
}

// Refresh handles POST /api/v1/auth/refresh. Validates the refresh token,
// checks it has not been revoked in Redis, then rotates: deletes the old jti
// and issues a fresh pair.
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}
	claims, err := ParseToken(h.cfg.RefreshSecret, req.RefreshToken)
	if err != nil || claims.Type != TokenTypeRefresh {
		writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}
	exists, err := h.tokens.RefreshExists(r.Context(), claims.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token store unavailable")
		return
	}
	if !exists {
		writeError(w, http.StatusUnauthorized, "refresh token revoked")
		return
	}
	if err := h.tokens.DeleteRefresh(r.Context(), claims.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke old refresh token")
		return
	}
	if err := h.issueAndSave(w, r, claims.Subject, claims.Email); err != nil {
		return
	}
}

// Logout handles POST /api/v1/auth/logout (requires a valid access token).
// Revokes the supplied refresh token.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}
	claims, err := ParseToken(h.cfg.RefreshSecret, req.RefreshToken)
	if err == nil && claims.Type == TokenTypeRefresh {
		if err := h.tokens.DeleteRefresh(r.Context(), claims.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to revoke refresh token")
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"logged out"}`))
}

// issueAndSave mints a token pair, persists the refresh jti in Redis, and
// writes the JSON response.
func (h *Handler) issueAndSave(w http.ResponseWriter, r *http.Request, userID, email string) error {
	roles, err := h.users.GetUserRoles(r.Context(), uuid.MustParse(userID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load roles")
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
		writeError(w, http.StatusInternalServerError, "failed to issue tokens")
		return err
	}
	if err := h.tokens.SaveRefresh(r.Context(), refreshJTI, userID, h.cfg.RefreshTTL); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist refresh token")
		return err
	}

	uid := uuid.MustParse(userID)
	fullName, phone := "", ""
	if u, _ := h.users.GetUserByEmail(r.Context(), email); u != nil && u.ID == uid {
		fullName, phone = u.FullName, u.Phone
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(tokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(h.cfg.AccessTTL.Seconds()),
		User:         userDTO{ID: uid, Email: email, FullName: fullName, Phone: phone, Roles: roles},
	})
}
