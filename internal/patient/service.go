package patient

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*Patient, error) {
	if in.DateOfBirth != nil && in.DateOfBirth.After(time.Now()) {
		return nil, invalid("date of birth cannot be in the future")
	}
	return s.repo.Create(ctx, in)
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Patient, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (*Patient, error) {
	return s.repo.Update(ctx, id, in)
}

func (s *Service) List(ctx context.Context, q ListQuery) ([]Patient, int64, error) {
	if q.Limit == 0 {
		q.Limit = 20
	}
	return s.repo.List(ctx, q)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}
