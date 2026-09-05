package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/portal/service"
	schedsvc "github.com/PandaX185/clinic-management/internal/scheduling/service"
)

func TestBook_ProvisionsProfileAndBooksInClinic(t *testing.T) {
	userID, clinicID, doctorID, patientID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	start := time.Now().Add(time.Hour)
	fake := &fakePatientRepo{
		clinic:    &service.ClinicRef{ID: clinicID, Name: "Acme Clinic", Slug: "acme"},
		profileID: patientID,
	}
	apptRepo := &fakeApptRepo{
		appt: &schedsvc.Appointment{
			ID: uuid.New(), PatientID: patientID, DoctorID: doctorID,
			StartTime: start, EndTime: start.Add(30 * time.Minute), Status: "scheduled",
		},
	}
	aptSvc := schedsvc.NewServiceWithIdentity(apptRepo, nil, fixedIdentity{patientID: patientID}, time.Minute)
	svc := service.NewService(fake, aptSvc)

	got, err := svc.Book(context.Background(), userID, service.BookInput{
		ClinicID: clinicID, DoctorID: doctorID, StartTime: start, DurationMinutes: 30,
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}

	if fake.ensureSlug != "acme" || fake.ensureUserID != userID {
		t.Errorf("EnsurePatientProfile called with slug=%q user=%v", fake.ensureSlug, fake.ensureUserID)
	}
	if apptRepo.booked == nil {
		t.Fatal("appointment service was not called")
	}
	if apptRepo.booked.DoctorID != doctorID {
		t.Errorf("booked wrong doctor: %v", apptRepo.booked.DoctorID)
	}
	if got.ClinicName != "Acme Clinic" || got.ClinicID != clinicID {
		t.Errorf("booked appointment lacks clinic link: %+v", got)
	}
}

func TestBook_UnknownClinicFails(t *testing.T) {
	fake := &fakePatientRepo{clinicErr: apperr.NotFound("clinic not found")}
	aptSvc := schedsvc.NewService(&fakeApptRepo{}, nil, nil, time.Minute)
	svc := service.NewService(fake, aptSvc)

	_, err := svc.Book(context.Background(), uuid.New(), service.BookInput{
		ClinicID: uuid.New(), DoctorID: uuid.New(), StartTime: time.Now().Add(time.Hour), DurationMinutes: 30,
	})
	ae := apperr.From(err)
	if ae.Kind != apperr.KindNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestCancel_CancelsPatientsOwnAppointment(t *testing.T) {
	userID, clinicID, apptID, patientID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	fake := &fakePatientRepo{clinic: &service.ClinicRef{ID: clinicID, Name: "Acme", Slug: "acme"}}
	apptRepo := &fakeApptRepo{
		appt: &schedsvc.Appointment{ID: apptID, PatientID: patientID, Status: "scheduled"},
	}
	aptSvc := schedsvc.NewServiceWithIdentity(apptRepo, nil, fixedIdentity{patientID: patientID}, time.Minute)
	svc := service.NewService(fake, aptSvc)

	got, err := svc.Cancel(context.Background(), userID, clinicID, apptID, "changed my mind")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got.ID != apptID {
		t.Errorf("cancelled wrong appointment: %v", got.ID)
	}
	if len(apptRepo.transitions) != 1 {
		t.Fatalf("want 1 transition, got %d", len(apptRepo.transitions))
	}
	tr := apptRepo.transitions[0]
	if tr.CancellationReason == nil || *tr.CancellationReason != "changed my mind" {
		t.Errorf("cancellation reason not forwarded: %+v", tr.CancellationReason)
	}
}

func TestCancel_OthersAppointmentForbidden(t *testing.T) {
	userID, clinicID, apptID := uuid.New(), uuid.New(), uuid.New()
	apptOwner, actorPID := uuid.New(), uuid.New()
	fake := &fakePatientRepo{clinic: &service.ClinicRef{ID: clinicID, Name: "Acme", Slug: "acme"}}
	apptRepo := &fakeApptRepo{
		appt: &schedsvc.Appointment{ID: apptID, PatientID: apptOwner, Status: "scheduled"},
	}
	aptSvc := schedsvc.NewServiceWithIdentity(apptRepo, nil, fixedIdentity{patientID: actorPID}, time.Minute)
	svc := service.NewService(fake, aptSvc)

	// The actor resolves to a patient grant that does not match the
	// appointment's owner, so the appointment service denies the mutation.
	_, err := svc.Cancel(context.Background(), userID, clinicID, apptID, "nope")
	ae := apperr.From(err)
	if ae.Kind != apperr.KindForbidden {
		t.Fatalf("expected Forbidden, got %v", err)
	}
}

func TestReschedule_MovesAppointment(t *testing.T) {
	userID, clinicID, apptID, patientID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	newStart := time.Now().Add(2 * time.Hour)
	fake := &fakePatientRepo{clinic: &service.ClinicRef{ID: clinicID, Name: "Acme", Slug: "acme"}}
	apptRepo := &fakeApptRepo{
		appt: &schedsvc.Appointment{ID: apptID, PatientID: patientID, Status: "scheduled"},
	}
	aptSvc := schedsvc.NewServiceWithIdentity(apptRepo, nil, fixedIdentity{patientID: patientID}, time.Minute)
	svc := service.NewService(fake, aptSvc)

	got, err := svc.Reschedule(context.Background(), userID, clinicID, apptID, service.RescheduleInput{
		StartTime: newStart, DurationMinutes: 60,
	})
	if err != nil {
		t.Fatalf("Reschedule: %v", err)
	}
	if got.ID != apptID {
		t.Errorf("rescheduled wrong appointment")
	}
	if len(apptRepo.transitions) != 1 {
		t.Fatalf("want 1 transition, got %d", len(apptRepo.transitions))
	}
	tr := apptRepo.transitions[0]
	if tr.NewStartTime == nil || !tr.NewStartTime.Equal(newStart) {
		t.Errorf("new start time not forwarded: %+v", tr.NewStartTime)
	}
}

func TestMe_RejectsDeactivatedAccount(t *testing.T) {
	fake := &fakePatientRepo{user: &service.User{ID: uuid.New(), IsActive: false}}
	svc := service.NewService(fake, schedsvc.NewService(&fakeApptRepo{}, nil, nil, time.Minute))

	_, err := svc.Me(context.Background(), uuid.New())
	if ae := apperr.From(err); ae.Kind != apperr.KindUnauthorized {
		t.Fatalf("expected Unauthorized, got %v", err)
	}
}

var _ service.Repository = (*fakePatientRepo)(nil)

type fakePatientRepo struct {
	user      *service.User
	updateErr error
	clinic    *service.ClinicRef
	clinicErr error
	appts     []service.Appointment
	appt      *service.Appointment
	apptErr   error
	profileID uuid.UUID

	ensureSlug   string
	ensureUserID uuid.UUID
}

func (f *fakePatientRepo) Me(_ context.Context, userID uuid.UUID) (*service.User, error) {
	if f.user == nil {
		return &service.User{ID: userID, IsActive: true}, nil
	}
	return f.user, nil
}

func (f *fakePatientRepo) UpdateMe(_ context.Context, _ uuid.UUID, _ service.UpdateUserInput) error {
	return f.updateErr
}

func (f *fakePatientRepo) GetClinic(_ context.Context, _ uuid.UUID) (*service.ClinicRef, error) {
	return f.clinic, f.clinicErr
}

func (f *fakePatientRepo) ListAppointments(_ context.Context, _ uuid.UUID) ([]service.Appointment, error) {
	return f.appts, nil
}

func (f *fakePatientRepo) GetAppointment(_ context.Context, _ uuid.UUID, _, _ uuid.UUID) (*service.Appointment, error) {
	return f.appt, f.apptErr
}

func (f *fakePatientRepo) EnsurePatientProfile(_ context.Context, slug string, userID uuid.UUID) (uuid.UUID, error) {
	f.ensureSlug, f.ensureUserID = slug, userID
	return f.profileID, nil
}

var _ schedsvc.Repository = (*fakeApptRepo)(nil)

type fakeApptRepo struct {
	appt        *schedsvc.Appointment
	booked      *schedsvc.BookTxParams
	transitions []schedsvc.TransitionParams
}

func (f *fakeApptRepo) GetByID(_ context.Context, _ uuid.UUID) (*schedsvc.Appointment, error) {
	return f.appt, nil
}

func (f *fakeApptRepo) List(_ context.Context, _ schedsvc.ListQuery) ([]schedsvc.Appointment, int64, error) {
	return nil, 0, nil
}

func (f *fakeApptRepo) BookTx(_ context.Context, params schedsvc.BookTxParams) (schedsvc.BookingResult, error) {
	f.booked = &params
	return schedsvc.BookingResult{Appointment: f.appt}, nil
}

func (f *fakeApptRepo) Transition(_ context.Context, params schedsvc.TransitionParams) (*schedsvc.Appointment, error) {
	f.transitions = append(f.transitions, params)
	return f.appt, nil
}

var _ schedsvc.IdentityResolver = fixedIdentity{}

type fixedIdentity struct{ patientID uuid.UUID }

func (i fixedIdentity) PatientIDForUser(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
	return i.patientID, nil
}

func (i fixedIdentity) DoctorIDForUser(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
	return uuid.Nil, nil
}
