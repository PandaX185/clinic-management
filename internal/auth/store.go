package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// User mirrors the users table row (subset of sqlc-generated model, kept
// decoupled so handlers can be unit tested without a database).
type User struct {
	ID            uuid.UUID
	Email         string
	PasswordHash  string
	FullName      string
	Phone         string
	IsActive      bool
	EmailVerified bool
}

// UserStore is the persistence surface the auth handler needs.
type UserStore interface {
	CreateUser(ctx context.Context, email, passwordHash, fullName, phone string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserRoles(ctx context.Context, userID uuid.UUID) ([]string, error)
}

// TokenStore manages refresh-token revocation state in Redis.
type TokenStore interface {
	// SaveRefresh stores jti -> userID with the given TTL.
	SaveRefresh(ctx context.Context, jti, userID string, ttl time.Duration) error
	// ConsumeRefresh atomically checks-and-deletes the refresh token jti,
	// returning the stored userID. Returns ErrRefreshNotFound if the token
	// was already consumed/revoked. This prevents the TOCTOU race where two
	// concurrent requests both observe the token before either deletes it.
	ConsumeRefresh(ctx context.Context, jti string) (userID string, err error)
	// DeleteRefresh revokes the refresh token jti without consuming it
	// (used by logout).
	DeleteRefresh(ctx context.Context, jti string) error
}

var (
	ErrDuplicateEmail  = errors.New("auth: email already registered")
	ErrRefreshNotFound = errors.New("auth: refresh token not found")
)

const refreshKeyPrefix = "refresh:"

func refreshKey(jti string) string { return refreshKeyPrefix + jti }

// RedisTokenStore is the production TokenStore backed by go-redis.
type RedisTokenStore struct {
	rdb *redis.Client
}

func NewRedisTokenStore(rdb *redis.Client) *RedisTokenStore {
	return &RedisTokenStore{rdb: rdb}
}

func (s *RedisTokenStore) SaveRefresh(ctx context.Context, jti, userID string, ttl time.Duration) error {
	return s.rdb.Set(ctx, refreshKey(jti), userID, ttl).Err()
}

// ConsumeRefresh atomically consumes (GETDEL) the refresh token jti and
// returns the stored userID. Requires Redis >= 6.2.
func (s *RedisTokenStore) ConsumeRefresh(ctx context.Context, jti string) (string, error) {
	userID, err := s.rdb.GetDel(ctx, refreshKey(jti)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrRefreshNotFound
	}
	if err != nil {
		return "", err
	}
	return userID, nil
}

func (s *RedisTokenStore) DeleteRefresh(ctx context.Context, jti string) error {
	return s.rdb.Del(ctx, refreshKey(jti)).Err()
}
