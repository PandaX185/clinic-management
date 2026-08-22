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
	// RefreshExists reports whether the refresh token jti is still valid.
	RefreshExists(ctx context.Context, jti string) (bool, error)
	// DeleteRefresh revokes the refresh token jti.
	DeleteRefresh(ctx context.Context, jti string) error
}

var ErrDuplicateEmail = errors.New("auth: email already registered")

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

func (s *RedisTokenStore) RefreshExists(ctx context.Context, jti string) (bool, error) {
	n, err := s.rdb.Exists(ctx, refreshKey(jti)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *RedisTokenStore) DeleteRefresh(ctx context.Context, jti string) error {
	return s.rdb.Del(ctx, refreshKey(jti)).Err()
}
