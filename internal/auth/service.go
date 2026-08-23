package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
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

func (s *Service) Register(ctx context.Context, in RegisterInput) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return s.repo.CreateUser(ctx, strings.ToLower(in.Email), string(hash), in.FullName, in.Phone, in.InitialRole)
}

func (s *Service) Login(ctx context.Context, in LoginInput) (*TokenPair, error) {
	user, err := s.repo.GetUserByEmail(ctx, strings.ToLower(in.Email))
	if err != nil {
		return nil, err
	}
	if !user.isActive() {
		return nil, apperr.Unauthorized("account is deactivated")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)); err != nil {
		return nil, apperr.Unauthorized("invalid credentials")
	}
	return s.tokens.Issue(user)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := s.tokens.Parse(refreshToken, TokenTypeRefresh)
	if err != nil {
		return nil, apperr.Unauthorized("invalid or expired refresh token")
	}
	user, err := s.repo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if !user.isActive() {
		return nil, apperr.Unauthorized("account is deactivated")
	}
	return s.tokens.Issue(user)
}

func (s *Service) ParseAccessToken(token string) (*Claims, error) {
	return s.tokens.Parse(token, TokenTypeAccess)
}

func (m *TokenManager) Issue(u *User) (*TokenPair, error) {
	roles := make([]string, len(u.Roles))
	for i, r := range u.Roles {
		roles[i] = string(r)
	}
	now := time.Now()

	access := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid":   u.ID.String(),
		"roles": roles,
		"typ":   TokenTypeAccess,
		"iat":   now.Unix(),
		"exp":   now.Add(m.accessTTL).Unix(),
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
	typ, _ := mapClaims["typ"].(string)
	if typ != expectedType {
		return nil, errors.New("wrong token type")
	}
	uidStr, _ := mapClaims["uid"].(string)
	uid, err := uuid.Parse(uidStr)
	if err != nil {
		return nil, errors.New("invalid subject")
	}
	var roles []string
	if raw, ok := mapClaims["roles"].([]any); ok {
		for _, r := range raw {
			if rs, ok := r.(string); ok {
				roles = append(roles, rs)
			}
		}
	}
	return &Claims{UserID: uid, Roles: roles, Type: typ}, nil
}

func (u *User) isActive() bool { return u.IsActive }
