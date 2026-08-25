package notification

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
)

func TestCreatePendingLive(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("no DATABASE_URL")
	}
	store := setupStore(t, url)
	msg := Message{
		ID:        uuid.New(),
		Channel:   ChannelEmail,
		Recipient: "live@test",
		Body:      "ping",
	}
	id, err := store.CreatePending(context.Background(), msg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	rec, err := store.GetByMsgID(context.Background(), id)
	if err != nil {
		t.Fatalf("lookup by msg id: %v", err)
	}
	if rec.ID != id {
		t.Fatalf("id mismatch: row=%s msg=%s", rec.ID, id)
	}
	t.Logf("OK row=%s status=%s", rec.ID, rec.Status)
}
