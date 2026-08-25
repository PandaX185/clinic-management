package notification

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupStore(t *testing.T, url string) *PostgresStore {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return NewPostgresStore(pool)
}
