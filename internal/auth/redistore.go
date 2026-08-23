package auth

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
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
