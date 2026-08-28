package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

type TokenManager struct {
	secret        []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
}

func NewTokenManager(secret, refreshSecret string, accessTTL, refreshTTL time.Duration) *TokenManager {
	return &TokenManager{
		secret:        []byte(secret),
		refreshSecret: []byte(refreshSecret),
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
	}
}

type Service struct {
	repo   Repository
	tokens *TokenManager
}

func NewService(repo Repository, tokens *TokenManager) *Service {
	return &Service{repo: repo, tokens: tokens}
}

// ListTenants returns all tenants the user belongs to, with their role in each.
func (s *Service) ListTenants(ctx context.Context, userID uuid.UUID) ([]UserTenant, error) {
	return s.repo.ListTenantsForUser(ctx, userID)
}

// HashRefreshToken converts a raw refresh token to a SHA-256 hash for DB storage.
func HashRefreshToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// Issue generates a new token pair for a user (no roles in JWT; resolved per-request via tenant).
func (m *TokenManager) Issue(u *User) (*TokenPair, error) {
	now := time.Now()

	access := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid": u.ID.String(),
		"typ": TokenTypeAccess,
		"iat": now.Unix(),
		"exp": now.Add(m.accessTTL).Unix(),
	})
	accessStr, err := access.SignedString(m.secret)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	refresh := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid": u.ID.String(),
		"typ": TokenTypeRefresh,
		"iat": now.Unix(),
		"exp": now.Add(m.refreshTTL).Unix(),
	})
	refreshStr, err := refresh.SignedString(m.refreshSecret)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	return &TokenPair{
		AccessToken:  accessStr,
		RefreshToken: refreshStr,
		TokenType:    "Bearer",
		ExpiresIn:    int64(m.accessTTL.Seconds()),
	}, nil
}

// Parse verifies and decodes a token.
func (m *TokenManager) Parse(token, expectedType string) (*Claims, error) {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		if expectedType == TokenTypeRefresh {
			return m.refreshSecret, nil
		}
		return m.secret, nil
	})
	if err != nil || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	mapClaims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid claims")
	}
	uidStr, _ := mapClaims["uid"].(string)
	uid, err := uuid.Parse(uidStr)
	if err != nil {
		return nil, errors.New("invalid subject")
	}
	return &Claims{UserID: uid, Type: mapClaims["typ"].(string)}, nil
}

// ParseAccessToken verifies and decodes an access token.
func (s *Service) ParseAccessToken(token string) (*Claims, error) {
	return s.tokens.Parse(token, TokenTypeAccess)
}

// Login authenticates a user and returns a token pair with refresh token stored.
func (s *Service) Login(ctx context.Context, in LoginInput) (*TokenPair, error) {
	user, err := s.repo.GetUserByPhone(ctx, normalizePhone(in.Phone))
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, apperr.Unauthorized("account is deactivated")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)); err != nil {
		return nil, apperr.Unauthorized("invalid credentials")
	}

	pair, err := s.tokens.Issue(user)
	if err != nil {
		return nil, err
	}

	// Store the hashed refresh token for validation/revocation
	expiry := time.Now().Add(s.tokens.refreshTTL)
	if err := s.repo.StoreRefreshToken(ctx, user.ID, HashRefreshToken(pair.RefreshToken), expiry); err != nil {
		return nil, apperr.Internal(err)
	}
	return pair, nil
}

// Me returns the authenticated user's profile.
func (s *Service) Me(ctx context.Context, userID uuid.UUID) (*User, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, apperr.Unauthorized("account is deactivated")
	}
	return user, nil
}

// Refresh exchanges a refresh token for a new access token pair.
// The old token is validated against the DB and then revoked (rotated).
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := s.tokens.Parse(refreshToken, TokenTypeRefresh)
	if err != nil {
		return nil, apperr.Unauthorized("invalid or expired refresh token")
	}

	// Validate the refresh token against the database
	tokenHash := HashRefreshToken(refreshToken)
	if err := s.repo.ValidateRefreshToken(ctx, claims.UserID, tokenHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.Unauthorized("refresh token was revoked or doesn't exist")
		}
		return nil, apperr.Unauthorized("refresh token validation failed")
	}

	user, err := s.repo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, apperr.Unauthorized("account is deactivated")
	}

	// Revoke old token (rotation)
	if err := s.repo.DeleteRefreshToken(ctx, claims.UserID, tokenHash); err != nil {
		return nil, apperr.Internal(err)
	}

	// Issue new pair
	pair, err := s.tokens.Issue(user)
	if err != nil {
		return nil, err
	}

	// Store new refresh token
	expiry := time.Now().Add(s.tokens.refreshTTL)
	if err := s.repo.StoreRefreshToken(ctx, user.ID, HashRefreshToken(pair.RefreshToken), expiry); err != nil {
		return nil, apperr.Internal(err)
	}
	return pair, nil
}

// Logout revokes a refresh token.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	claims, err := s.tokens.Parse(refreshToken, TokenTypeRefresh)
	if err != nil {
		return apperr.Unauthorized("invalid or expired refresh token")
	}

	tokenHash := HashRefreshToken(refreshToken)
	if err := s.repo.DeleteRefreshToken(ctx, claims.UserID, tokenHash); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// Register creates a new user account and returns a token pair.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	user, err := s.repo.CreateUser(ctx, normalizePhone(in.Phone), string(hash), in.FullName)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// normalizePhone converts phone to E.164 format.
func normalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if strings.HasPrefix(phone, "+") {
		return phone
	}
	if strings.HasPrefix(phone, "0") {
		return "+2" + phone
	}
	if len(phone) == 11 && strings.HasPrefix(phone, "20") {
		return "+" + phone
	}
	return "+" + phone
}