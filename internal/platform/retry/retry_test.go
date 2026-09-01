package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDo_RetriesUntilSuccess(t *testing.T) {
	ctx := context.Background()
	var calls int
	err := Do(ctx, Config{Timeout: time.Second, Attempts: 3, Backoff: time.Millisecond}, func(ctx context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("boom")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDo_ExhaustsAttempts(t *testing.T) {
	ctx := context.Background()
	var calls int
	err := Do(ctx, Config{Timeout: time.Second, Attempts: 3, Backoff: time.Millisecond}, func(ctx context.Context) error {
		calls++
		return errors.New("always fails")
	})
	if err == nil {
		t.Fatal("expected error after exhausting attempts")
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDo_CancelledContextAborts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Do(ctx, Config{Timeout: time.Second, Attempts: 5, Backoff: time.Hour}, func(ctx context.Context) error {
		return errors.New("should not proceed")
	})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestWithDefaults_FillsZeros(t *testing.T) {
	d := WithDefaults(Config{})
	want := DefaultConfig()
	if d.Timeout != want.Timeout || d.Attempts != want.Attempts || d.Backoff != want.Backoff {
		t.Fatalf("defaults not applied: %+v", d)
	}
}
