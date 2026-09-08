package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/PandaX185/lahza/internal/platform/apperr"
	queuesvc "github.com/PandaX185/lahza/internal/queue/service"
)

// stubQueue is an in-memory Repository focused on the service's decision
// logic; it does not model positions (the list/position behavior is tested at
// the API/repo layer).
type stubQueue struct {
	entry   queuesvc.Entry
	current string
}

var _ queuesvc.Repository = (*stubQueue)(nil)

func (s *stubQueue) get() *queuesvc.Entry {
	e := s.entry
	e.Status = s.current
	if s.current == "waiting" || s.current == "called" || s.current == "in_progress" {
		e.Position = 1
	}
	return &e
}

func (s *stubQueue) CreateEntry(_ context.Context, profileID uuid.UUID, _ *uuid.UUID, _ int32) (*queuesvc.Entry, error) {
	s.entry = queuesvc.Entry{ID: uuid.New(), ProfileID: profileID, Status: "waiting", CheckedInAt: time.Now()}
	s.current = "waiting"
	return s.get(), nil
}
func (s *stubQueue) GetEntry(context.Context, uuid.UUID) (*queuesvc.Entry, error) {
	if s.entry.ID == uuid.Nil {
		return nil, apperr.NotFound("queue entry not found")
	}
	return s.get(), nil
}
func (s *stubQueue) ListActive(context.Context, queuesvc.ListQuery) ([]queuesvc.Entry, int64, error) {
	return nil, 0, nil
}
func (s *stubQueue) ListForProfile(context.Context, uuid.UUID) ([]queuesvc.Entry, error) {
	return nil, nil
}
func (s *stubQueue) Position(context.Context, int32, time.Time) (int64, error) {
	return 1, nil
}
func (s *stubQueue) Transition(_ context.Context, _ uuid.UUID, target string) (*queuesvc.Entry, error) {
	s.current = target
	s.entry.Status = target
	return s.get(), nil
}

func TestCheckIn_Validity(t *testing.T) {
	svc := queuesvc.NewService(&stubQueue{})

	if _, err := svc.CheckIn(context.Background(), uuid.Nil, nil, 0); !isInvalid(err) {
		t.Fatalf("profile_id=zero: want Invalid, got %v", err)
	}
	if _, err := svc.CheckIn(context.Background(), uuid.New(), nil, -1); !isInvalid(err) {
		t.Fatalf("negative priority: want Invalid, got %v", err)
	}
	e, err := svc.CheckIn(context.Background(), uuid.New(), nil, 0)
	if err != nil {
		t.Fatalf("valid check-in: %v", err)
	}
	if e.Status != "waiting" || e.Position != 1 {
		t.Fatalf("unexpected check-in entry: %+v", e)
	}
}

func TestTransition_StatusMachine(t *testing.T) {
	stub := &stubQueue{}
	svc := queuesvc.NewService(stub)
	if _, err := svc.CheckIn(context.Background(), uuid.New(), nil, 0); err != nil {
		t.Fatalf("check-in: %v", err)
	}

	cases := []struct {
		from, to string
		ok       bool
	}{
		{"waiting", "called", true},
		{"called", "in_progress", true},
		{"in_progress", "completed", true},
		{"waiting", "skipped", true},
		{"called", "skipped", true},
		{"waiting", "cancelled", true},
		{"waiting", "completed", false},   // cannot skip to done
		{"called", "completed", false},    // must start first
		{"completed", "called", false},    // terminal
		{"in_progress", "waiting", false}, // not reversible
	}
	for _, tc := range cases {
		stub.current = tc.from
		entry, err := svc.Transition(context.Background(), stub.entry.ID, tc.to)
		if tc.ok {
			if err != nil {
				t.Errorf("transition %s->%s should be allowed: %v", tc.from, tc.to, err)
				continue
			}
			if entry.Status != tc.to {
				t.Errorf("transition %s->%s landed on %s", tc.from, tc.to, entry.Status)
			}
		} else if err == nil {
			t.Errorf("transition %s->%s should have been rejected", tc.from, tc.to)
		}
	}
}

func isInvalid(err error) bool {
	ae := apperr.From(err)
	return ae.Kind == apperr.KindInvalid
}
