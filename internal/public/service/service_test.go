package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/public/service"
)

func TestGetAvailableSlots_WalksScheduleAndSkipsConflicts(t *testing.T) {
	day := time.Date(2027, 7, 12, 0, 0, 0, 0, time.UTC) // Monday
	clinicID, doctorID := uuid.New(), uuid.New()
	typeID := uuid.New()
	busyStart := day.Add(10*time.Hour + 0*time.Minute)

	fake := &fakePublicRepo{
		services: []service.ServiceItem{{ID: typeID, Name: "Check-up", DurationMin: 60}},
		schedules: []service.Schedule{
			{DayOfWeek: 1, StartMin: 9 * 60, EndMin: 12 * 60}, // 09:00-12:00
		},
		appts: []service.Appointment{{Start: busyStart, End: busyStart.Add(30 * time.Minute)}},
	}
	svc := service.NewService(fake)

	slots, err := svc.GetAvailableSlots(context.Background(), clinicID, service.SlotQuery{
		DoctorID: doctorID,
		Date:     day,
	})
	if err != nil {
		t.Fatalf("GetAvailableSlots: %v", err)
	}

	// 09:00, 10:00, 11:00 candidates; the 10:00 slot collides with a booking.
	if len(slots) != 2 {
		t.Fatalf("want 2 slots, got %d: %+v", len(slots), slots)
	}
	wantStart := []time.Duration{9 * time.Hour, 11 * time.Hour}
	for i, d := range wantStart {
		if got := slots[i].Start; !got.Equal(day.Add(d)) {
			t.Errorf("slot %d start = %v, want %v", i, got, day.Add(d))
		}
		if !slots[i].End.Equal(slots[i].Start.Add(time.Hour)) {
			t.Errorf("slot %d should be 60 minutes", i)
		}
		if slots[i].DoctorID != doctorID || slots[i].AppointmentTypeID != typeID {
			t.Errorf("slot %d missing doctor/type ids", i)
		}
	}
}

func TestGetAvailableSlots_RespectsRequestedTypeDuration(t *testing.T) {
	day := time.Date(2027, 7, 12, 0, 0, 0, 0, time.UTC)
	clinicID, doctorID := uuid.New(), uuid.New()
	typeID := uuid.New()

	fake := &fakePublicRepo{
		schedules: []service.Schedule{{DayOfWeek: 1, StartMin: 9 * 60, EndMin: 12 * 60}},
		apptType:  &service.ServiceItem{ID: typeID, Name: "Consult", DurationMin: 90},
	}
	svc := service.NewService(fake)

	slots, err := svc.GetAvailableSlots(context.Background(), clinicID, service.SlotQuery{
		DoctorID:          doctorID,
		Date:              day,
		AppointmentTypeID: typeID,
	})
	if err != nil {
		t.Fatalf("GetAvailableSlots: %v", err)
	}

	// 09:00, 10:30 for a 90-minute type in a 3-hour window.
	if len(slots) != 2 {
		t.Fatalf("want 2 slots, got %d", len(slots))
	}
	if want := day.Add(9*time.Hour + 90*time.Minute); !slots[1].Start.Equal(want) {
		t.Errorf("slot 1 start = %v, want %v", slots[1].Start, want)
	}
}

func TestGetAvailableSlots_IncludesSlotEndingExactlyAtScheduleEnd(t *testing.T) {
	day := time.Date(2027, 7, 12, 0, 0, 0, 0, time.UTC)
	clinicID, doctorID := uuid.New(), uuid.New()

	fake := &fakePublicRepo{
		services: []service.ServiceItem{{ID: uuid.New(), DurationMin: 90}},
		schedules: []service.Schedule{
			{DayOfWeek: 1, StartMin: 9 * 60, EndMin: 10*60 + 30}, // 09:00-10:30
		},
	}
	svc := service.NewService(fake)

	slots, err := svc.GetAvailableSlots(context.Background(), clinicID, service.SlotQuery{
		DoctorID: doctorID,
		Date:     day,
	})
	if err != nil {
		t.Fatalf("GetAvailableSlots: %v", err)
	}
	if len(slots) != 1 {
		t.Fatalf("want 1 slot ending exactly at 10:30, got %d", len(slots))
	}
	if want := day.Add(10*time.Hour + 30*time.Minute); !slots[0].End.Equal(want) {
		t.Errorf("slot end = %v, want %v", slots[0].End, want)
	}
}

func TestGetAvailableSlots_RequiresDoctorAndDate(t *testing.T) {
	svc := service.NewService(&fakePublicRepo{})

	if _, err := svc.GetAvailableSlots(context.Background(), uuid.New(), service.SlotQuery{Date: time.Now()}); err == nil {
		t.Error("expected error when doctor_id is missing")
	}
	if _, err := svc.GetAvailableSlots(context.Background(), uuid.New(), service.SlotQuery{DoctorID: uuid.New()}); err == nil {
		t.Error("expected error when date is missing")
	}
}

func TestListClinics_NormalizesPagination(t *testing.T) {
	fake := &fakePublicRepo{total: 2}
	svc := service.NewService(fake)

	if _, _, err := svc.ListClinics(context.Background(), 0, 500); err != nil {
		t.Fatalf("ListClinics: %v", err)
	}
	if fake.lastOffset != 0 || fake.lastLimit != 20 {
		t.Errorf("pagination normalized wrong: offset=%d limit=%d", fake.lastOffset, fake.lastLimit)
	}
}

func TestListClinics_AppliesPageSizeOffset(t *testing.T) {
	fake := &fakePublicRepo{}
	svc := service.NewService(fake)

	if _, _, err := svc.ListClinics(context.Background(), 3, 25); err != nil {
		t.Fatalf("ListClinics: %v", err)
	}
	if fake.lastOffset != 50 || fake.lastLimit != 25 {
		t.Errorf("expected offset=50 limit=25, got %d %d", fake.lastOffset, fake.lastLimit)
	}
}

func TestGetClinicDetail_ComposesServicesAndDoctors(t *testing.T) {
	clinicID := uuid.New()
	fake := &fakePublicRepo{
		clinic:   &service.Clinic{ID: clinicID, Name: "Acme"},
		services: []service.ServiceItem{{ID: uuid.New(), DurationMin: 30}},
		doctors:  []service.Doctor{{ID: uuid.New(), Name: "Dr. X"}},
	}
	svc := service.NewService(fake)

	clinic, err := svc.GetClinicDetail(context.Background(), clinicID)
	if err != nil {
		t.Fatalf("GetClinicDetail: %v", err)
	}
	if len(clinic.Services) != 1 || len(clinic.Doctors) != 1 {
		t.Errorf("clinic composition incomplete: %+v", clinic)
	}
}

func TestGetClinicDetail_PropagatesNotFound(t *testing.T) {
	fake := &fakePublicRepo{err: apperr.NotFound("clinic not found")}
	svc := service.NewService(fake)

	if _, err := svc.GetClinicDetail(context.Background(), uuid.New()); err == nil {
		t.Error("expected not-found error")
	}
}

func TestGetDoctor_PassesThrough(t *testing.T) {
	docID := uuid.New()
	fake := &fakePublicRepo{doctor: &service.Doctor{ID: docID, Name: "Dr. Y"}}
	svc := service.NewService(fake)

	doc, err := svc.GetDoctor(context.Background(), docID)
	if err != nil {
		t.Fatalf("GetDoctor: %v", err)
	}
	if doc.ID != docID || doc.Name != "Dr. Y" {
		t.Errorf("doctor passthrough mangled: %+v", doc)
	}
	if fake.gotDoctorID != docID {
		t.Errorf("GetDoctor called with %v, want %v", fake.gotDoctorID, docID)
	}
}

var _ service.Repository = (*fakePublicRepo)(nil)

type fakePublicRepo struct {
	clinics     []service.Clinic
	total       int64
	clinic      *service.Clinic
	services    []service.ServiceItem
	doctors     []service.Doctor
	doctorTotal int64
	doctor      *service.Doctor
	schedules   []service.Schedule
	appts       []service.Appointment
	apptType    *service.ServiceItem
	err         error

	lastOffset, lastLimit int
	gotDoctorID           uuid.UUID
}

func (f *fakePublicRepo) failIf(err error) error {
	if f.err != nil {
		return f.err
	}
	return err
}

func (f *fakePublicRepo) ListClinics(_ context.Context, offset, limit int) ([]service.Clinic, int64, error) {
	f.lastOffset, f.lastLimit = offset, limit
	return f.clinics, f.total, f.failIf(nil)
}

func (f *fakePublicRepo) GetClinic(_ context.Context, _ uuid.UUID) (*service.Clinic, error) {
	return f.clinic, f.failIf(nil)
}

func (f *fakePublicRepo) ListClinicServices(_ context.Context, _ uuid.UUID) ([]service.ServiceItem, error) {
	return f.services, f.failIf(nil)
}

func (f *fakePublicRepo) ListClinicDoctors(_ context.Context, _ uuid.UUID, offset, limit int) ([]service.Doctor, int64, error) {
	return f.doctors, f.doctorTotal, f.failIf(nil)
}

func (f *fakePublicRepo) FindDoctor(_ context.Context, id uuid.UUID) (*service.Doctor, error) {
	f.gotDoctorID = id
	return f.doctor, f.failIf(nil)
}

func (f *fakePublicRepo) ListDoctorSchedules(_ context.Context, _, _ uuid.UUID, _ time.Time) ([]service.Schedule, error) {
	return f.schedules, f.failIf(nil)
}

func (f *fakePublicRepo) ListDoctorAppointments(_ context.Context, _, _ uuid.UUID, _, _ time.Time) ([]service.Appointment, error) {
	return f.appts, f.failIf(nil)
}

func (f *fakePublicRepo) GetAppointmentType(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*service.ServiceItem, error) {
	if f.apptType == nil {
		return nil, errors.New("type not found")
	}
	return f.apptType, f.failIf(nil)
}
