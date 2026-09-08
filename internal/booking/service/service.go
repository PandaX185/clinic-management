package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/PandaX185/lahza/internal/platform/apperr"
)

const (
	maxPageSize  = 100
	defaultSlots = 30 * time.Minute
)

// Service is the public clinic discovery use case surface.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// ListClinics returns active clinics with a simple page/size pagination.
func (s *Service) ListClinics(ctx context.Context, page, size int) ([]Clinic, int64, error) {
	page, size = normalizePage(page, size)
	items, total, err := s.repo.ListClinics(ctx, (page-1)*size, size)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GetClinicDetail returns a clinic with its services and doctors.
func (s *Service) GetClinicDetail(ctx context.Context, id uuid.UUID) (*Clinic, error) {
	clinic, err := s.repo.GetClinic(ctx, id)
	if err != nil {
		return nil, err
	}
	services, err := s.repo.ListClinicServices(ctx, id)
	if err != nil {
		return nil, err
	}
	doctors, _, err := s.repo.ListClinicDoctors(ctx, id, 0, 200)
	if err != nil {
		return nil, err
	}
	clinic.Services = services
	clinic.Doctors = doctors
	return clinic, nil
}

// ListDoctors returns the doctors working at a clinic.
func (s *Service) ListDoctors(ctx context.Context, clinicID uuid.UUID, page, size int) ([]Doctor, int64, error) {
	page, size = normalizePage(page, size)
	items, total, err := s.repo.ListClinicDoctors(ctx, clinicID, (page-1)*size, size)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GetDoctor returns a doctor no matter which clinic they practise at.
func (s *Service) GetDoctor(ctx context.Context, id uuid.UUID) (*Doctor, error) {
	return s.repo.FindDoctor(ctx, id)
}

// SlotQuery describes the slots a caller wants to see.
type SlotQuery struct {
	DoctorID          uuid.UUID
	Date              time.Time
	AppointmentTypeID uuid.UUID
}

// GetAvailableSlots builds the open booking windows for a doctor on a date by
// walking their schedule windows, stepping by the requested appointment
// type's duration, and dropping windows that collide with existing bookings.
func (s *Service) GetAvailableSlots(ctx context.Context, clinicID uuid.UUID, q SlotQuery) ([]Slot, error) {
	if q.DoctorID == uuid.Nil {
		return nil, apperr.Invalid("doctor_id is required")
	}
	if q.Date.IsZero() {
		return nil, apperr.Invalid("date is required")
	}

	schedules, err := s.repo.ListDoctorSchedules(ctx, clinicID, q.DoctorID, q.Date)
	if err != nil {
		return nil, err
	}
	exceptions, err := s.repo.ListScheduleDayExceptions(ctx, clinicID, q.DoctorID, q.Date)
	if err != nil {
		return nil, err
	}
	effective := applyExceptions(schedules, exceptions)

	dayStart := startOfDay(q.Date)
	dayEnd := dayStart.Add(24 * time.Hour)
	busy, err := s.repo.ListDoctorAppointments(ctx, clinicID, q.DoctorID, dayStart, dayEnd)
	if err != nil {
		return nil, err
	}

	durationMin, typeID, err := s.durationFor(ctx, clinicID, q)
	if err != nil {
		return nil, err
	}
	duration := time.Duration(durationMin) * time.Minute

	now := time.Now()
	out := make([]Slot, 0, 32)
	for _, sched := range effective {
		start := dayStart.Add(time.Duration(sched.StartMin) * time.Minute)
		end := dayStart.Add(time.Duration(sched.EndMin) * time.Minute)
		// Overnight/wrapping schedules are not supported yet.
		if !end.After(start) {
			continue
		}
		for t := start; t.Add(duration).Before(end) || t.Add(duration).Equal(end); t = t.Add(duration) {
			// Never offer already-elapsed windows for today.
			if !t.After(now) {
				continue
			}
			slotEnd := t.Add(duration)
			if overlaps(t, slotEnd, busy) {
				continue
			}
			out = append(out, Slot{
				Start:             t,
				End:               slotEnd,
				DoctorID:          q.DoctorID,
				AppointmentTypeID: typeID,
			})
		}
	}
	return out, nil
}

// durationFor picks the slot length: the explicit appointment type if given,
// otherwise the clinic's first service, otherwise a 30-minute default.
func (s *Service) durationFor(ctx context.Context, clinicID uuid.UUID, q SlotQuery) (int, uuid.UUID, error) {
	if q.AppointmentTypeID != uuid.Nil {
		at, err := s.repo.GetAppointmentType(ctx, clinicID, q.AppointmentTypeID)
		if err != nil {
			return 0, uuid.Nil, err
		}
		return at.DurationMin, at.ID, nil
	}
	services, err := s.repo.ListClinicServices(ctx, clinicID)
	if err != nil {
		return 0, uuid.Nil, err
	}
	if len(services) > 0 {
		return services[0].DurationMin, services[0].ID, nil
	}
	return int(defaultSlots.Minutes()), uuid.Nil, nil
}

// overlaps reports whether [start, end) collides with any busy window.
func overlaps(start, end time.Time, busy []Appointment) bool {
	for _, b := range busy {
		if start.Before(b.End) && end.After(b.Start) {
			return true
		}
	}
	return false
}

func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	// (page-1)*size must not overflow int; PG offsets are int64 anyway.
	if page > 1_000_000 {
		page = 1_000_000
	}
	if size < 1 || size > maxPageSize {
		size = 20
	}
	return page, size
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// applyExceptions folds a doctor's one-off date overrides into their recurring
// weekly windows. blocked types (leave, holiday, unavailable) carve minutes
// out of the windows; extra_hours adds a window.
func applyExceptions(schedules []Schedule, exceptions []Exception) []Schedule {
	effective := make([]Schedule, 0, len(schedules)+2)
	effective = append(effective, schedules...)
	for _, ex := range exceptions {
		if ex.Type != "extra_hours" {
			effective = carve(effective, ex.StartMin, ex.EndMin)
		} else {
			effective = append(effective, Schedule{StartMin: ex.StartMin, EndMin: ex.EndMin})
		}
	}
	return effective
}

// carve removes [from, to) from every window in minutes, splitting windows
// that straddle the range. Empty results are dropped.
func carve(windows []Schedule, from, to int) []Schedule {
	out := make([]Schedule, 0, len(windows)+1)
	for _, w := range windows {
		if to <= w.StartMin || from >= w.EndMin || to <= from {
			if !emptySchedule(w) {
				out = append(out, w)
			}
			continue
		}
		if from > w.StartMin {
			out = append(out, Schedule{StartMin: w.StartMin, EndMin: from})
		}
		if to < w.EndMin {
			out = append(out, Schedule{StartMin: to, EndMin: w.EndMin})
		}
	}
	return out
}

func emptySchedule(w Schedule) bool {
	return w.EndMin <= w.StartMin
}
