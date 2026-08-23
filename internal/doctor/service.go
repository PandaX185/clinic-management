package doctor

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

type UserCreator interface {
	CreateDoctorUser(ctx context.Context, email, passwordHash, fullName string) (*CreatedUser, error)
}

type CreatedUser struct {
	ID uuid.UUID
}

type Service struct {
	repo  Repository
	users UserCreator
}

func NewService(repo Repository, users UserCreator) *Service {
	return &Service{repo: repo, users: users}
}

// CreateDoctor provisions the underlying login account plus the clinical profile.
func (s *Service) CreateDoctor(ctx context.Context, email, password, fullName, specialization, license string, bio *string) (*Doctor, error) {
	user, err := s.users.CreateDoctorUser(ctx, email, password, fullName)
	if err != nil {
		return nil, err
	}
	created, err := s.repo.Create(ctx, CreateDoctorInput{
		UserID:         user.ID,
		Specialization: specialization,
		LicenseNumber:  license,
		Bio:            bio,
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, created.ID)
}

func (s *Service) Create(ctx context.Context, in CreateDoctorInput) (*Doctor, error) {
	return s.repo.Create(ctx, in)
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Doctor, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context, activeOnly bool, specialization string, limit, offset int) ([]Doctor, error) {
	if limit == 0 {
		limit = 20
	}
	return s.repo.List(ctx, activeOnly, specialization, limit, offset)
}

func (s *Service) AddSchedule(ctx context.Context, doctorID uuid.UUID, in AddScheduleInput) (*Schedule, error) {
	slot := in.SlotMinutes
	if slot == 0 {
		slot = 30
	}
	start := time.Date(2000, 1, 2, in.StartHour, in.StartMinute, 0, 0, time.UTC)
	end := time.Date(2000, 1, 2, in.EndHour, in.EndMinute, 0, 0, time.UTC)
	if !end.After(start) {
		return nil, apperr.Invalid("end_time must be after start_time")
	}
	return s.repo.AddSchedule(ctx, doctorID, in.DayOfWeek, start, end, slot)
}

func (s *Service) GetSchedules(ctx context.Context, doctorID uuid.UUID) ([]Schedule, error) {
	return s.repo.GetSchedules(ctx, doctorID)
}

func (s *Service) RemoveSchedule(ctx context.Context, scheduleID uuid.UUID) error {
	return s.repo.RemoveSchedule(ctx, scheduleID)
}

func (s *Service) AddException(ctx context.Context, doctorID uuid.UUID, date time.Time, isUnavailable bool, start, end *time.Time, reason string) error {
	return s.repo.AddException(ctx, doctorID, date, isUnavailable, start, end, reason)
}

// Availability computes free bookable slots for a doctor between from and to.
// Recurring weekly schedules are expanded into concrete slots per day,
// exceptions (full-day or partial blocks) are subtracted, past times dropped,
// and finally booked appointments remove conflicting slots.
func (s *Service) Availability(ctx context.Context, doctorID uuid.UUID, from, to time.Time, loc *time.Location) ([]AvailabilitySlot, error) {
	if to.Before(from) {
		return nil, apperr.Invalid("end of range must not be before start")
	}
	if to.Sub(from) > 31*24*time.Hour {
		return nil, apperr.Invalid("availability range must not exceed 31 days")
	}

	doctor, err := s.repo.GetByID(ctx, doctorID)
	if err != nil {
		return nil, err
	}
	if !doctor.IsActive {
		return []AvailabilitySlot{}, nil
	}

	schedules, err := s.repo.GetSchedules(ctx, doctorID)
	if err != nil {
		return nil, err
	}
	exceptions, err := s.repo.GetExceptions(ctx, doctorID, dateOnly(from), dateOnly(to))
	if err != nil {
		return nil, err
	}

	now := time.Now()
	slotsByDay := make(map[string][]interval)
	var dayOrder []string

	for day := dateOnly(from); !day.After(dateOnly(to)); day = day.AddDate(0, 0, 1) {
		localDay := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
		key := localDay.Format("2006-01-02")
		dayOrder = append(dayOrder, key)

		daySlots := expandSchedules(schedules, localDay.Weekday(), localDay)
		daySlots = applyExceptions(daySlots, exceptions, localDay)
		daySlots = removePast(daySlots, now)
		slotsByDay[key] = daySlots
	}

	rangeStart := dateOnly(from).AddDate(0, 0, -1)
	rangeEnd := dateOnly(to).AddDate(0, 0, 1)
	booked, err := s.repo.GetActiveAppointmentsInRange(ctx, doctorID, rangeStart, rangeEnd)
	if err != nil {
		return nil, err
	}

	result := make([]AvailabilitySlot, 0)
	for _, key := range dayOrder {
		for _, iv := range slotsByDay[key] {
			if overlapsAny(iv, booked) {
				continue
			}
			result = append(result, AvailabilitySlot{
				StartTime: iv.start.Format(time.RFC3339),
				EndTime:   iv.stop.Format(time.RFC3339),
			})
		}
	}
	return result, nil
}

type interval struct {
	start time.Time
	stop  time.Time
}

func expandSchedules(schedules []Schedule, weekday time.Weekday, dayStart time.Time) []interval {
	target := int(weekday)
	out := make([]interval, 0)
	for _, sch := range schedules {
		if sch.DayOfWeek != target {
			continue
		}
		offset := func(t time.Time) time.Duration {
			return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute + time.Duration(t.Second())*time.Second
		}
		slotDur := time.Duration(sch.SlotMinutes) * time.Minute
		cur := dayStart.Add(offset(sch.StartTime))
		endAt := dayStart.Add(offset(sch.EndTime))
		for ; cur.Add(slotDur).Before(endAt) || cur.Add(slotDur).Equal(endAt); cur = cur.Add(slotDur) {
			out = append(out, interval{start: cur, stop: cur.Add(slotDur)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].start.Before(out[j].start) })
	return out
}

func applyExceptions(slots []interval, exceptions []ScheduleExceptionRow, dayStart time.Time) []interval {
	var relevant []ScheduleExceptionRow
	for _, e := range exceptions {
		if sameDay(e.Date, dayStart) {
			relevant = append(relevant, e)
		}
	}
	out := slots
	for _, exc := range relevant {
		if exc.IsUnavailable {
			return nil
		}
		if exc.StartTime != nil && exc.EndTime != nil {
			out = cut(out, combine(exc.StartTime, dayStart), combine(exc.EndTime, dayStart))
		}
	}
	return out
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func combine(t *time.Time, dayStart time.Time) time.Time {
	return time.Date(dayStart.Year(), dayStart.Month(), dayStart.Day(), t.Hour(), t.Minute(), t.Second(), 0, dayStart.Location())
}

func cut(slots []interval, from, to time.Time) []interval {
	out := make([]interval, 0, len(slots))
	for _, s := range slots {
		if !s.stop.After(from) || !s.start.Before(to) {
			out = append(out, s)
			continue
		}
		if s.start.Before(from) {
			out = append(out, interval{start: s.start, stop: from})
		}
		if s.stop.After(to) {
			out = append(out, interval{start: to, stop: s.stop})
		}
	}
	return out
}

func removePast(slots []interval, now time.Time) []interval {
	out := make([]interval, 0, len(slots))
	for _, s := range slots {
		if s.stop.After(now) {
			out = append(out, s)
		}
	}
	return out
}

func overlapsAny(iv interval, booked []AppointmentRange) bool {
	for _, b := range booked {
		if iv.start.Before(b.EndTime) && iv.stop.After(b.StartTime) {
			return true
		}
	}
	return false
}
