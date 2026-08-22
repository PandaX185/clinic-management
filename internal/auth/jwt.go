package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrTokenExpired = errors.New("auth: token expired")
	ErrTokenInvalid = errors.New("auth: token invalid")
)

// TokenType distinguishes access tokens from refresh tokens so a refresh
// token can never be used as an access token and vice versa.
type TokenType string

const TokenTypeAccess TokenType = "access"
const TokenTypeRefresh TokenType = "refresh"

// Claims is the JWT payload for both access and refresh tokens.
type Claims struct {
	Subject   string    `json:"sub"`
	Email     string    `json:"email"`
	Roles     []string  `json:"roles"`
	ID        string    `json:"jti"`
	Type      TokenType `json:"typ"`
	IssuedAt  int64     `json:"iat"`
	ExpiresAt int64     `json:"exp"`
}

// NewJTI returns a random 128-bit identifier encoded as base64url.
func NewJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate jti: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SignToken produces a signed HS256 JWT with the given claims. exp/iat must
// already be set by the caller.
func SignToken(secret []byte, c Claims) (string, error) {
	if c.IssuedAt == 0 {
		c.IssuedAt = time.Now().Unix()
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadJSON, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := header + "." + payload
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	sig := mac.Sum(nil)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// ParseToken validates signature and expiry and returns the claims.
func ParseToken(secret []byte, token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrTokenInvalid
	}
	signingInput := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return nil, ErrTokenInvalid
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, ErrTokenInvalid
	}
	if time.Now().Unix() >= c.ExpiresAt {
		return nil, ErrTokenExpired
	}
	return &c, nil
}

// IssuePair mints a new access+refresh token pair for a user.
func IssuePair(accessSecret, refreshSecret []byte, userID, email string, roles []string, accessTTL, refreshTTL time.Duration) (access, refresh string, refreshJTI string, err error) {
	jti, err := NewJTI()
	if err != nil {
		return "", "", "", err
	}
	now := time.Now()
	accessJTI, err := NewJTI()
	if err != nil {
		return "", "", "", err
	}
	accessClaims := Claims{
		Subject: userID, Email: email, Roles: roles,
		ID: accessJTI, Type: TokenTypeAccess,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(accessTTL).Unix(),
	}
	if access, err = SignToken(accessSecret, accessClaims); err != nil {
		return "", "", "", err
	}
	refreshClaims := Claims{
		Subject: userID, Email: email, Roles: roles,
		ID: jti, Type: TokenTypeRefresh,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(refreshTTL).Unix(),
	}
	if refresh, err = SignToken(refreshSecret, refreshClaims); err != nil {
		return "", "", "", err
	}
	return access, refresh, jti, nil
}
