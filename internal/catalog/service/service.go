package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	idsvc "github.com/PandaX185/lahza/internal/identity/service"
	"github.com/PandaX185/lahza/internal/platform/apperr"
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
	if !idsvc.StandardRoles()[role] {
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

// GetDoctorSchedule returns a doctor's weekly windows and date exceptions.
func (s *Service) GetDoctorSchedule(ctx context.Context, doctorID uuid.UUID) (*Schedule, error) {
	weekly, err := s.repo.ListDoctorSchedule(ctx, doctorID)
	if err != nil {
		return nil, err
	}
	exceptions, err := s.repo.ListScheduleExceptions(ctx, doctorID)
	if err != nil {
		return nil, err
	}
	return &Schedule{Weekly: weekly, Exceptions: exceptions}, nil
}

// SetWeeklySchedule replaces a doctor's recurring windows. The rows are
// normalized (times bounded, durations defaulted) and deduplicated on
// (day_of_week, start, end) before the repo swaps the set in a transaction.
func (s *Service) SetWeeklySchedule(ctx context.Context, doctorID uuid.UUID, hours []WeeklyHour) error {
	normalized, err := normalizeHours(hours)
	if err != nil {
		return err
	}
	return s.repo.ReplaceWeeklySchedule(ctx, doctorID, normalized)
}

// CreateScheduleException registers a one-off override for a doctor.
func (s *Service) CreateScheduleException(ctx context.Context, doctorID uuid.UUID, in ScheduleException) (*ScheduleException, error) {
	if in.Date.IsZero() {
		return nil, apperr.Invalid("date is required")
	}
	if err := validateExceptionType(in.Type); err != nil {
		return nil, err
	}
	if in.StartMin < 0 || in.EndMin <= in.StartMin || in.EndMin > 24*60 {
		return nil, apperr.Invalid("exception hours must be a positive range within the day")
	}
	return s.repo.AddScheduleException(ctx, doctorID, in)
}

// DeleteScheduleException removes a one-off override by id.
func (s *Service) DeleteScheduleException(ctx context.Context, exceptionID uuid.UUID) error {
	return s.repo.RemoveScheduleException(ctx, exceptionID)
}

// exceptionTypes are the overrides the slot generator understands.
func validateExceptionType(t string) error {
	switch t {
	case "leave", "holiday", "unavailable", "extra_hours":
		return nil
	}
	return apperr.Invalid("unknown exception type: " + t)
}

// normalizeHours validates and canonicalizes a weekly window set: bounded
// week days, sane times, a default 30-minute slot, and no duplicate windows.
func normalizeHours(hours []WeeklyHour) ([]WeeklyHour, error) {
	seen := make(map[[4]int]bool, len(hours))
	out := make([]WeeklyHour, 0, len(hours))
	for _, h := range hours {
		if h.DayOfWeek < 0 || h.DayOfWeek > 6 {
			return nil, apperr.Invalid("day_of_week must be between 0 and 6")
		}
		if h.StartMin < 0 || h.EndMin <= h.StartMin || h.EndMin > 24*60 {
			return nil, apperr.Invalid("window hours must be a positive range within the day")
		}
		if h.SlotDuration <= 0 {
			h.SlotDuration = 30
		}
		if h.SlotDuration > 24*60 {
			return nil, apperr.Invalid("slot_duration must be within the day")
		}
		key := [4]int{int(h.DayOfWeek), h.StartMin, h.EndMin, h.SlotDuration}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, WeeklyHour{
			DayOfWeek:    h.DayOfWeek,
			StartMin:     h.StartMin,
			EndMin:       h.EndMin,
			SlotDuration: h.SlotDuration,
			IsActive:     true,
		})
	}
	return out, nil
}
