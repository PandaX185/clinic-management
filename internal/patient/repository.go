package patient

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Create(ctx context.Context, in CreateInput) (*Patient, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Patient, error)
	Update(ctx context.Context, id uuid.UUID, in UpdateInput) (*Patient, error)
	List(ctx context.Context, q ListQuery) ([]Patient, int64, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
