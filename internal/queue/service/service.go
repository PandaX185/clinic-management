package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

// Service is the clinic queue use case surface.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// CheckIn adds a patient to the line. appointmentID is optional; a walk-in has
// none yet. priority lifts an entry (higher = called first).
func (s *Service) CheckIn(ctx context.Context, profileID uuid.UUID, appointmentID *uuid.UUID, priority int32) (*Entry, error) {
	if profileID == uuid.Nil {
		return nil, apperr.Invalid("profile_id is required")
	}
	if appointmentID != nil && *appointmentID == uuid.Nil {
		appointmentID = nil
	}
	if priority < 0 {
		return nil, apperr.Invalid("priority must be non-negative")
	}
	entry, err := s.repo.CreateEntry(ctx, profileID, appointmentID, priority)
	if err != nil {
		return nil, err
	}
	if entry.Active() {
		pos, err := s.repo.Position(ctx, entry.Priority, entry.CheckedInAt)
		if err != nil {
			return nil, err
		}
		entry.Position = pos
	}
	return entry, nil
}

// ListActive returns the current line (active entries only), newest
// check-ins ordered last, with pagination normalized to the page limits.
func (s *Service) ListActive(ctx context.Context, q ListQuery) ([]Entry, int64, error) {
	if q.Limit < 1 || q.Limit > 100 {
		q.Limit = 20
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return s.repo.ListActive(ctx, q)
}

// Transition moves an entry through its status machine, setting the matching
// timestamp. Invalid moves (e.g. waiting -> completed) are rejected.
func (s *Service) Transition(ctx context.Context, id uuid.UUID, target string) (*Entry, error) {
	cur, err := s.repo.GetEntry(ctx, id)
	if err != nil {
		return nil, err
	}
	if !CanTransition(cur.Status, target) {
		return nil, apperr.Conflict(fmt.Sprintf("cannot move queue entry from %q to %q", cur.Status, target))
	}
	entry, err := s.repo.Transition(ctx, id, target)
	if err != nil {
		return nil, err
	}
	if entry.Active() {
		pos, err := s.repo.Position(ctx, entry.Priority, entry.CheckedInAt)
		if err != nil {
			return nil, err
		}
		entry.Position = pos
	}
	return entry, nil
}
