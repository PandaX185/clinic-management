package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	paymentsvc "github.com/PandaX185/clinic-management/internal/payments/service"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/portal/service"
	queuesvc "github.com/PandaX185/clinic-management/internal/queue/service"
	schedsvc "github.com/PandaX185/clinic-management/internal/scheduling/service"
)

func newPortalSvc(fake service.Repository, appt *schedsvc.Service) *service.Service {
	return service.NewService(fake, appt, queuesvc.NewService(&stubQueueRepo{}), paymentsvc.NewService(&stubPaymentRepo{}, nil, "EGP"))
}

var _ paymentsvc.Repository = (*stubPaymentRepo)(nil)

type stubPaymentRepo struct {
	appt *paymentsvc.Appointment
	paid *paymentsvc.Payment
}

func (s *stubPaymentRepo) GetAppointmentForPayment(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*paymentsvc.Appointment, error) {
	return s.appt, nil
}
func (s *stubPaymentRepo) InsertPending(_ context.Context, _ paymentsvc.PayInput, _ string, _ string) (*paymentsvc.Payment, error) {
	p := &paymentsvc.Payment{ID: uuid.New(), AppointmentID: s.appt.ID, Status: paymentsvc.StatusPending}
	s.paid = p
	return p, nil
}
func (s *stubPaymentRepo) GetForAppointment(_ context.Context, _ uuid.UUID) (*paymentsvc.Payment, error) {
	return nil, nil
}
func (s *stubPaymentRepo) GetByID(_ context.Context, id uuid.UUID) (*paymentsvc.Payment, error) {
	if s.paid != nil && s.paid.ID == id {
		return s.paid, nil
	}
	return nil, nil
}
func (s *stubPaymentRepo) MarkPaid(_ context.Context, _ uuid.UUID) (*paymentsvc.Payment, error) {
	if s.paid != nil {
		s.paid.Status = paymentsvc.StatusPaid
		now := time.Now()
		s.paid.PaidAt = &now
	}
	return s.paid, nil
}
func (s *stubPaymentRepo) MarkRefunded(_ context.Context, id uuid.UUID) (*paymentsvc.Payment, error) {
	if s.paid != nil && s.paid.ID == id {
		s.paid.Status = paymentsvc.StatusRefunded
	}
	return s.paid, nil
}

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
	svc := newPortalSvc(fake, aptSvc)

	got, replayed, err := svc.Book(context.Background(), userID, service.BookInput{
		ClinicID: clinicID, DoctorID: doctorID, StartTime: start, DurationMinutes: 30,
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	if replayed {
		t.Errorf("expected a fresh booking, got replay=true")
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
	svc := newPortalSvc(fake, aptSvc)

	_, _, err := svc.Book(context.Background(), uuid.New(), service.BookInput{
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
	svc := newPortalSvc(fake, aptSvc)

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
	svc := newPortalSvc(fake, aptSvc)

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
	svc := newPortalSvc(fake, aptSvc)

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

func TestPay_ChargesInsClinicAndReturnsPayment(t *testing.T) {
	userID, clinicID, apptID := uuid.New(), uuid.New(), uuid.New()
	fake := &fakePatientRepo{clinic: &service.ClinicRef{ID: clinicID, Name: "Acme", Slug: "acme"}}
	stub := &stubPaymentRepo{appt: &paymentsvc.Appointment{ID: apptID, Status: "scheduled"}}
	svc := service.NewService(fake, schedsvc.NewService(&fakeApptRepo{}, nil, nil, time.Minute), queuesvc.NewService(&stubQueueRepo{}), paymentsvc.NewService(stub, nil, "EGP"))

	payment, clinic, err := svc.Pay(context.Background(), userID, clinicID, apptID, paymentsvc.MethodCard)
	if err != nil {
		t.Fatalf("Pay: %v", err)
	}
	if payment.Status != paymentsvc.StatusPaid {
		t.Errorf("expected paid, got %s", payment.Status)
	}
	if payment.AppointmentID != apptID {
		t.Errorf("payment for wrong appointment: %v", payment.AppointmentID)
	}
	if clinic.ID != clinicID || clinic.Name != "Acme" {
		t.Errorf("unexpected clinic link: %+v", clinic)
	}
}

func TestPay_UnknownClinicFails(t *testing.T) {
	svc := service.NewService(&fakePatientRepo{clinicErr: apperr.NotFound("clinic not found")},
		schedsvc.NewService(&fakeApptRepo{}, nil, nil, time.Minute),
		queuesvc.NewService(&stubQueueRepo{}),
		paymentsvc.NewService(&stubPaymentRepo{}, nil, "EGP"))

	_, _, err := svc.Pay(context.Background(), uuid.New(), uuid.New(), uuid.New(), paymentsvc.MethodCard)
	if ae := apperr.From(err); ae.Kind != apperr.KindNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestMe_RejectsDeactivatedAccount(t *testing.T) {
	fake := &fakePatientRepo{user: &service.User{ID: uuid.New(), IsActive: false}}
	svc := newPortalSvc(fake, schedsvc.NewService(&fakeApptRepo{}, nil, nil, time.Minute))

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
	myQueues  []service.QueueEntry

	ensureSlug    string
	ensureUserID  uuid.UUID
	myQueueUserID uuid.UUID
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

func (f *fakePatientRepo) ListMyQueues(_ context.Context, userID uuid.UUID) ([]service.QueueEntry, error) {
	f.myQueueUserID = userID
	return f.myQueues, nil
}

var _ queuesvc.Repository = (*stubQueueRepo)(nil)

type stubQueueRepo struct {
	created *queuesvc.Entry
}

func (s *stubQueueRepo) CreateEntry(_ context.Context, profileID uuid.UUID, _ *uuid.UUID, _ int32) (*queuesvc.Entry, error) {
	s.created = &queuesvc.Entry{ID: uuid.New(), ProfileID: profileID, Status: "waiting", Priority: 0}
	return s.created, nil
}
func (s *stubQueueRepo) GetEntry(context.Context, uuid.UUID) (*queuesvc.Entry, error) {
	return s.created, nil
}
func (s *stubQueueRepo) ListActive(context.Context, queuesvc.ListQuery) ([]queuesvc.Entry, int64, error) {
	return nil, 0, nil
}
func (s *stubQueueRepo) ListForProfile(context.Context, uuid.UUID) ([]queuesvc.Entry, error) {
	return nil, nil
}
func (s *stubQueueRepo) Position(context.Context, int32, time.Time) (int64, error) {
	return 1, nil
}
func (s *stubQueueRepo) Transition(context.Context, uuid.UUID, string) (*queuesvc.Entry, error) {
	return s.created, nil
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

func TestJoinQueue_ProvisionsProfileAndChecksIn(t *testing.T) {
	userID, clinicID, patientID := uuid.New(), uuid.New(), uuid.New()
	fake := &fakePatientRepo{
		clinic:    &service.ClinicRef{ID: clinicID, Name: "Acme", Slug: "acme"},
		profileID: patientID,
	}
	stub := &stubQueueRepo{}
	svc := service.NewService(fake, schedsvc.NewService(&fakeApptRepo{}, nil, nil, time.Minute), queuesvc.NewService(stub), paymentsvc.NewService(&stubPaymentRepo{}, nil, "EGP"))

	entry, err := svc.JoinQueue(context.Background(), userID, clinicID)
	if err != nil {
		t.Fatalf("JoinQueue: %v", err)
	}
	if fake.ensureSlug != "acme" || fake.ensureUserID != userID {
		t.Errorf("EnsurePatientProfile called with slug=%q user=%v", fake.ensureSlug, fake.ensureUserID)
	}
	if stub.created == nil || stub.created.ProfileID != patientID {
		t.Fatalf("check-in not passed through: %+v", stub.created)
	}
	if entry.ClinicID != clinicID || entry.ClinicName != "Acme" {
		t.Errorf("entry lacks clinic link: %+v", entry)
	}
	if entry.Status != "waiting" {
		t.Errorf("expected waiting, got %q", entry.Status)
	}
}

func TestMyQueue_ReturnsEntriesWithClinicLink(t *testing.T) {
	userID := uuid.New()
	fake := &fakePatientRepo{
		myQueues: []service.QueueEntry{
			{ID: uuid.New(), Status: "waiting", Position: 2, ClinicID: uuid.New(), ClinicName: "Acme"},
		},
	}
	svc := newPortalSvc(fake, schedsvc.NewService(&fakeApptRepo{}, nil, nil, time.Minute))

	items, err := svc.MyQueue(context.Background(), userID)
	if err != nil {
		t.Fatalf("MyQueue: %v", err)
	}
	if len(items) != 1 || items[0].Position != 2 || items[0].ClinicName != "Acme" {
		t.Fatalf("unexpected entries: %+v", items)
	}
	if fake.myQueueUserID != userID {
		t.Errorf("repo called with user %v, want %v", fake.myQueueUserID, userID)
	}
}
