package doctor

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

type mockRepo struct {
	doctor       Doctor
	schedules    []Schedule
	exceptions   []ScheduleExceptionRow
	appointments []AppointmentRange
	getErr       error
}

func (m *mockRepo) Create(context.Context, CreateDoctorInput) (*Doctor, error) {
	return nil, nil
}
func (m *mockRepo) GetByID(context.Context, uuid.UUID) (*Doctor, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	d := m.doctor
	return &d, nil
}
func (m *mockRepo) List(context.Context, bool, string, int, int) ([]Doctor, error) {
	return nil, nil
}
func (m *mockRepo) AddSchedule(context.Context, uuid.UUID, int, time.Time, time.Time, int) (*Schedule, error) {
	return nil, nil
}
func (m *mockRepo) GetSchedules(context.Context, uuid.UUID) ([]Schedule, error) {
	return m.schedules, nil
}
func (m *mockRepo) RemoveSchedule(context.Context, uuid.UUID) error { return nil }
func (m *mockRepo) AddException(context.Context, uuid.UUID, time.Time, bool, *time.Time, *time.Time, string) error {
	return nil
}
func (m *mockRepo) GetExceptions(context.Context, uuid.UUID, time.Time, time.Time) ([]ScheduleExceptionRow, error) {
	return m.exceptions, nil
}
func (m *mockRepo) GetActiveAppointmentsInRange(context.Context, uuid.UUID, time.Time, time.Time) ([]AppointmentRange, error) {
	return m.appointments, nil
}

var _ Repository = (*mockRepo)(nil)

func testSchedule(dayOfWeek int, startH, endH, slotMinutes int) Schedule {
	day := func(h int) time.Time {
		return time.Date(2000, 1, 2, h, 0, 0, 0, time.UTC)
	}
	return Schedule{
		ID:          uuid.New(),
		DayOfWeek:   dayOfWeek,
		StartTime:   day(startH),
		EndTime:     day(endH),
		SlotMinutes: slotMinutes,
	}
}

// monday 2026-09-07
const queryDay = "2026-09-07"

func parseDay(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	require.NoError(t, err)
	return d
}

func TestAvailability_ExpandsWeeklySchedule(t *testing.T) {
	svc := NewService(&mockRepo{
		doctor:    Doctor{ID: uuid.New(), IsActive: true},
		schedules: []Schedule{testSchedule(1, 9, 11, 60)},
	}, nil)

	slots, err := svc.Availability(context.Background(), uuid.New(), parseDay(t, queryDay), parseDay(t, queryDay), time.UTC)
	require.NoError(t, err)
	assert.Len(t, slots, 2)
	assert.Equal(t, "2026-09-07T09:00:00Z", slots[0].StartTime)
	assert.Equal(t, "2026-09-07T10:00:00Z", slots[1].StartTime)
}

func TestAvailability_FullDayExceptionClearsSlots(t *testing.T) {
	repo := &mockRepo{
		doctor:     Doctor{ID: uuid.New(), IsActive: true},
		schedules:  []Schedule{testSchedule(1, 9, 17, 30)},
		exceptions: []ScheduleExceptionRow{{Date: parseDay(t, queryDay), IsUnavailable: true}},
	}
	svc := NewService(repo, nil)

	slots, err := svc.Availability(context.Background(), uuid.New(), parseDay(t, queryDay), parseDay(t, queryDay), time.UTC)
	require.NoError(t, err)
	assert.Empty(t, slots)
}

func TestAvailability_PartialExceptionCutsMiddle(t *testing.T) {
	lunchStart := time.Date(2000, 1, 2, 12, 0, 0, 0, time.UTC)
	lunchEnd := time.Date(2000, 1, 2, 13, 0, 0, 0, time.UTC)
	repo := &mockRepo{
		doctor:    Doctor{ID: uuid.New(), IsActive: true},
		schedules: []Schedule{testSchedule(1, 9, 14, 60)},
		exceptions: []ScheduleExceptionRow{{
			Date:          parseDay(t, queryDay),
			IsUnavailable: false,
			StartTime:     &lunchStart,
			EndTime:       &lunchEnd,
		}},
	}
	svc := NewService(repo, nil)

	slots, err := svc.Availability(context.Background(), uuid.New(), parseDay(t, queryDay), parseDay(t, queryDay), time.UTC)
	require.NoError(t, err)
	starts := make([]string, 0, len(slots))
	for _, s := range slots {
		starts = append(starts, s.StartTime)
	}
	assert.Equal(t, []string{"2026-09-07T09:00:00Z", "2026-09-07T10:00:00Z", "2026-09-07T11:00:00Z", "2026-09-07T13:00:00Z"}, starts)
}

func TestAvailability_BookedAppointmentsRemoveConflictingSlots(t *testing.T) {
	repo := &mockRepo{
		doctor:    Doctor{ID: uuid.New(), IsActive: true},
		schedules: []Schedule{testSchedule(1, 9, 12, 60)},
		appointments: []AppointmentRange{{
			ID:        uuid.New(),
			StartTime: time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 9, 7, 11, 0, 0, 0, time.UTC),
		}},
	}
	svc := NewService(repo, nil)

	slots, err := svc.Availability(context.Background(), uuid.New(), parseDay(t, queryDay), parseDay(t, queryDay), time.UTC)
	require.NoError(t, err)
	for _, s := range slots {
		assert.NotEqual(t, "2026-09-07T10:00:00Z", s.StartTime, "booked slot must not be offered")
	}
	assert.Len(t, slots, 2)
}

func TestAvailability_InactiveDoctorHasNoSlots(t *testing.T) {
	svc := NewService(&mockRepo{
		doctor:    Doctor{ID: uuid.New(), IsActive: false},
		schedules: []Schedule{testSchedule(1, 9, 17, 30)},
	}, nil)

	slots, err := svc.Availability(context.Background(), uuid.New(), parseDay(t, queryDay), parseDay(t, queryDay), time.UTC)
	require.NoError(t, err)
	assert.Empty(t, slots)
}

func TestAvailability_RejectsInvalidRange(t *testing.T) {
	svc := NewService(&mockRepo{}, nil)

	from := parseDay(t, "2026-09-07")
	to := from.AddDate(0, 0, -1)
	_, err := svc.Availability(context.Background(), uuid.New(), from, to, time.UTC)
	require.Error(t, err)

	appErr := apperr.From(err)
	assert.Equal(t, apperr.KindInvalid, appErr.Kind)

	to = from.AddDate(0, 0, 32)
	_, err = svc.Availability(context.Background(), uuid.New(), from, to, time.UTC)
	require.Error(t, err)
}

func TestExpandSchedules_OnlyCompleteSlotsFit(t *testing.T) {
	sch := testSchedule(1, 9, 10, 25)
	dayStart := parseDay(t, queryDay)
	intervals := expandSchedules([]Schedule{sch}, dayStart.Weekday(), dayStart)

	require.Len(t, intervals, 2, "a 60-minute window fits exactly two complete 25-minute slots")
	assert.Equal(t, "09:00", intervals[0].start.Format("15:04"))
	assert.Equal(t, "09:50", intervals[1].stop.Format("15:04"))
}

func TestCut_SplitsAroundBlock(t *testing.T) {
	base := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	slots := []interval{{start: base, stop: base.Add(2 * time.Hour)}}
	blockFrom := base.Add(30 * time.Minute)
	blockTo := base.Add(90 * time.Minute)

	out := cut(slots, blockFrom, blockTo)
	require.Len(t, out, 2)
	assert.Equal(t, base.Add(90*time.Minute), out[1].start)
}
