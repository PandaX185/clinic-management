package auth

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Config carries the auth tunables shared by service and middleware.
type Config struct {
	AccessSecret  []byte
	RefreshSecret []byte
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
}

// Session is the result of a successful Register/Login/Refresh: a minted
// token pair plus the authenticated user's data for the response.
type Session struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64 // access TTL in seconds
	User         *User
	Roles        []string
}

// AuthService holds the auth business logic. Transport (handler.go) depends
// on it; it depends only on the ports in ports.go.
type AuthService struct {
	users  UserStore
	tokens TokenStore
	cfg    Config
}

func NewService(users UserStore, tokens TokenStore, cfg Config) *AuthService {
	return &AuthService{users: users, tokens: tokens, cfg: cfg}
}

// NormalizeEmail lowercases and trims an email for canonical storage/lookup.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Register creates a new user and issues them a token session.
func (s *AuthService) Register(ctx context.Context, email, password, fullName, phone string) (*Session, error) {
	email = NormalizeEmail(email)

	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	user, err := s.users.CreateUser(ctx, email, hash, fullName, phone)
	if err != nil {
		return nil, err // ErrDuplicateEmail propagates to the transport mapper
	}
	return s.issueSession(ctx, user)
}

// Login verifies credentials and returns a token session. Unknown email and
// wrong password yield the same ErrInvalidCredentials (no user enumeration).
func (s *AuthService) Login(ctx context.Context, email, password string) (*Session, error) {
	email = NormalizeEmail(email)

	user, err := s.users.GetUserByEmail(ctx, email)
	if err != nil || VerifyPassword(password, user.PasswordHash) != nil {
		return nil, ErrInvalidCredentials
	}
	if !user.IsActive {
		return nil, ErrAccountDisabled
	}
	return s.issueSession(ctx, user)
}

// Refresh validates the refresh token, checks it has not been revoked,
// then rotates it: the old jti is consumed and a fresh pair issued.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*Session, error) {
	claims, err := ParseToken(s.cfg.RefreshSecret, refreshToken)
	if err != nil || claims.Type != TokenTypeRefresh {
		return nil, ErrRefreshInvalid
	}
	// Atomically check-and-consume so two concurrent requests with the same
	// token cannot both rotate (GETDEL semantics).
	storedUserID, err := s.tokens.ConsumeRefresh(ctx, claims.ID)
	if err != nil {
		if errors.Is(err, ErrRefreshNotFound) {
			return nil, ErrRefreshRevoked
		}
		return nil, err
	}
	if storedUserID != claims.Subject {
		return nil, ErrRefreshRevoked
	}

	user, err := s.users.GetUserByEmail(ctx, claims.Email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	return s.issueSession(ctx, user)
}

// Logout revokes the supplied refresh token if it parses as valid.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	claims, err := ParseToken(s.cfg.RefreshSecret, refreshToken)
	if err != nil || claims.Type != TokenTypeRefresh {
		return nil // nothing to revoke; idempotent success
	}
	return s.tokens.DeleteRefresh(ctx, claims.ID)
}

// issueSession loads roles, mints the token pair, and persists the refresh
// jti for revocation tracking.
func (s *AuthService) issueSession(ctx context.Context, user *User) (*Session, error) {
	roles, err := s.users.GetUserRoles(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if roles == nil {
		roles = []string{}
	}

	access, refresh, refreshJTI, err := IssuePair(
		s.cfg.AccessSecret, s.cfg.RefreshSecret,
		user.ID.String(), user.Email, roles,
		s.cfg.AccessTTL, s.cfg.RefreshTTL,
	)
	if err != nil {
		return nil, err
	}
	if err := s.tokens.SaveRefresh(ctx, refreshJTI, user.ID.String(), s.cfg.RefreshTTL); err != nil {
		return nil, err
	}
	return &Session{
		AccessToken: access, RefreshToken: refresh,
		ExpiresIn: int64(s.cfg.AccessTTL.Seconds()),
		User:      user, Roles: roles,
	}, nil
}
