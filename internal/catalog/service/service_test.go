package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/catalog/service"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

func isInvalid(err error) bool {
	var ae *apperr.Error
	return errors.As(err, &ae) && ae.Kind == apperr.KindInvalid
}

var _ service.Repo = (*stubRepo)(nil)

type stubRepo struct {
	weekly []service.WeeklyHour
	exs    []service.ScheduleException
	ex     *service.ScheduleException
}

func (s *stubRepo) ListProfiles(context.Context) ([]service.Profile, error) { return nil, nil }
func (s *stubRepo) CreateProfile(context.Context, uuid.UUID, string, string) (*service.Profile, error) {
	return nil, nil
}
func (s *stubRepo) ListDoctors(context.Context) ([]service.Profile, error) { return nil, nil }
func (s *stubRepo) ListAppointmentTypes(context.Context) ([]service.AppointmentType, error) {
	return nil, nil
}
func (s *stubRepo) GetAppointmentType(context.Context, uuid.UUID) (*service.AppointmentType, error) {
	return nil, nil
}
func (s *stubRepo) CreateAppointmentType(context.Context, service.AppointmentType) (*service.AppointmentType, error) {
	return nil, nil
}
func (s *stubRepo) UpdateAppointmentType(context.Context, service.AppointmentType) (*service.AppointmentType, error) {
	return nil, nil
}

func (s *stubRepo) ListDoctorSchedule(context.Context, uuid.UUID) ([]service.WeeklyHour, error) {
	return s.weekly, nil
}

func (s *stubRepo) ReplaceWeeklySchedule(_ context.Context, _ uuid.UUID, hours []service.WeeklyHour) error {
	s.weekly = hours
	return nil
}

func (s *stubRepo) ListScheduleExceptions(context.Context, uuid.UUID) ([]service.ScheduleException, error) {
	return s.exs, nil
}

func (s *stubRepo) AddScheduleException(_ context.Context, _ uuid.UUID, ex service.ScheduleException) (*service.ScheduleException, error) {
	ex.ID = uuid.New()
	s.ex = nil
	cp := ex
	s.ex = &cp
	return &cp, nil
}

func (s *stubRepo) RemoveScheduleException(context.Context, uuid.UUID) error { return nil }

func TestSetWeeklySchedule_NormalizesDedupesAndDefaultsDuration(t *testing.T) {
	stub := &stubRepo{}
	svc := service.NewService(stub)

	err := svc.SetWeeklySchedule(context.Background(), uuid.New(), []service.WeeklyHour{
		{DayOfWeek: 1, StartMin: 540, EndMin: 720, SlotDuration: 0},
		{DayOfWeek: 1, StartMin: 540, EndMin: 720}, // duplicate, dropped
		{DayOfWeek: 5, StartMin: 600, EndMin: 780, SlotDuration: 15},
	})
	if err != nil {
		t.Fatalf("SetWeeklySchedule: %v", err)
	}
	if len(stub.weekly) != 2 {
		t.Fatalf("want 2 normalized windows, got %+v", stub.weekly)
	}
	if stub.weekly[0].SlotDuration != 30 {
		t.Errorf("first window slot_duration = %d, want default 30", stub.weekly[0].SlotDuration)
	}
	if !stub.weekly[0].IsActive {
		t.Error("normalized window should default is_active=true")
	}
	if !stub.weekly[1].IsActive {
		t.Error("second window should default is_active=true")
	}
}

func TestSetWeeklySchedule_RejectsBadWindow(t *testing.T) {
	svc := service.NewService(&stubRepo{})
	cases := []struct {
		name  string
		hours []service.WeeklyHour
	}{
		{"bad day", []service.WeeklyHour{{DayOfWeek: 7, StartMin: 0, EndMin: 60}}},
		{"negative start", []service.WeeklyHour{{DayOfWeek: 0, StartMin: -1, EndMin: 60}}},
		{"zero length", []service.WeeklyHour{{DayOfWeek: 0, StartMin: 60, EndMin: 60}}},
		{"overnight", []service.WeeklyHour{{DayOfWeek: 0, StartMin: 1380, EndMin: 60}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.SetWeeklySchedule(context.Background(), uuid.New(), tc.hours)
			if !isInvalid(err) {
				t.Fatalf("want Invalid error, got %v", err)
			}
		})
	}
}

func TestCreateScheduleException_ValidatesTypeAndHours(t *testing.T) {
	svc := service.NewService(&stubRepo{})
	date := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	if _, err := svc.CreateScheduleException(context.Background(), uuid.New(), service.ScheduleException{
		Date: date, Type: "holiday", StartMin: 540, EndMin: 720,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	bad := []service.ScheduleException{
		{Type: "holiday", StartMin: 540, EndMin: 720},             // zero date
		{Date: date, Type: "sick", StartMin: 540, EndMin: 720},    // unknown type
		{Date: date, Type: "leave", StartMin: 720, EndMin: 720},   // empty range
		{Date: date, Type: "leave", StartMin: -1, EndMin: 720},    // negative
		{Date: date, Type: "leave", StartMin: 1380, EndMin: 1500}, // past midnight
	}
	for _, ex := range bad {
		if _, err := svc.CreateScheduleException(context.Background(), uuid.New(), ex); !isInvalid(err) {
			t.Errorf("%+v: want Invalid error, got %v", ex, err)
		}
	}
}
