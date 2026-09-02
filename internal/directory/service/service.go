package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

// Service is the clinic directory use case surface.
type Service struct {
	repo Repo
}

func NewService(repo Repo) *Service { return &Service{repo: repo} }

func (s *Service) ListProfiles(ctx context.Context) ([]Profile, error) {
	return s.repo.ListProfiles(ctx)
}

func (s *Service) ListDoctors(ctx context.Context) ([]Profile, error) {
	return s.repo.ListDoctors(ctx)
}

func (s *Service) CreateProfile(ctx context.Context, userID uuid.UUID, name, role string) (*Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperr.Invalid("display_name is required")
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		role = "patient"
	}
	if !standardRoles[role] {
		return nil, apperr.Invalid("unknown role: " + role)
	}
	return s.repo.CreateProfile(ctx, userID, name, role)
}

func (s *Service) ListAppointmentTypes(ctx context.Context) ([]AppointmentType, error) {
	return s.repo.ListAppointmentTypes(ctx)
}

func (s *Service) CreateAppointmentType(ctx context.Context, in AppointmentType) (*AppointmentType, error) {
	if err := validateType(in); err != nil {
		return nil, err
	}
	return s.repo.CreateAppointmentType(ctx, in)
}

func (s *Service) UpdateAppointmentType(ctx context.Context, id uuid.UUID, in AppointmentType) (*AppointmentType, error) {
	in.ID = id
	if err := validateType(in); err != nil {
		return nil, err
	}
	return s.repo.UpdateAppointmentType(ctx, in)
}

func validateType(in AppointmentType) error {
	if strings.TrimSpace(in.Name) == "" {
		return apperr.Invalid("name is required")
	}
	if in.DurationMinutes <= 0 {
		return apperr.Invalid("duration_minutes must be positive")
	}
	return nil
}

// standardRoles mirrors tenant.service.standardRoles: the roles seeded into
// every clinic schema on provision.
var standardRoles = map[string]bool{
	"admin": true, "staff": true, "doctor": true,
	"nurse": true, "manager": true, "patient": true,
}
