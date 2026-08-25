package appointment

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

type fakeIdentity struct {
	patientID uuid.UUID
	doctorID  uuid.UUID
}

func (f fakeIdentity) PatientIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return f.patientID, nil
}
func (f fakeIdentity) DoctorIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return f.doctorID, nil
}

type fakeRepo struct {
	Repository
	appt *Appointment
}

func (f fakeRepo) GetByID(ctx context.Context, id uuid.UUID) (*Appointment, error) {
	return f.appt, nil
}

func testAppt(patientID, doctorID uuid.UUID) *Appointment {
	return &Appointment{
		ID:        uuid.New(),
		PatientID: patientID,
		DoctorID:  doctorID,
		Status:    StatusScheduled,
		StartTime: time.Now().Add(24 * time.Hour),
		EndTime:   time.Now().Add(25 * time.Hour),
	}
}

func acFor(roles ...string) AccessContext {
	id := uuid.New()
	return AccessContext{UserID: id, Roles: roles, ActorID: &id}
}

// SEC-02: patients can only touch their own appointments.
func TestGetScoped_PatientCannotReadOthers(t *testing.T) {
	ownPatient := uuid.New()
	svc := NewServiceWithIdentity(fakeRepo{appt: testAppt(ownPatient, uuid.New())}, nil, nil,
		fakeIdentity{patientID: ownPatient}, time.Minute)

	other := testAppt(uuid.New(), uuid.New())
	repo := fakeRepo{appt: other}
	svc2 := NewServiceWithIdentity(repo, nil, nil, fakeIdentity{patientID: ownPatient}, time.Minute)

	_, err := svc2.GetScoped(context.Background(), other.ID, acFor("patient"))
	if err == nil || apperr.HTTPStatus(apperr.From(err).Kind) != 403 {
		t.Fatalf("expected 403 for cross-patient read, got %v", err)
	}

	// Own appointment is readable.
	if _, err := svc.GetScoped(context.Background(), uuid.New(), acFor("patient")); err != nil {
		t.Fatalf("expected own read to succeed, got %v", err)
	}
}

func TestGetScoped_DoctorScopedToOwnAppointments(t *testing.T) {
	ownDoctor := uuid.New()
	other := testAppt(uuid.New(), uuid.New())
	svc := NewServiceWithIdentity(fakeRepo{appt: other}, nil, nil,
		fakeIdentity{doctorID: ownDoctor}, time.Minute)

	if _, err := svc.GetScoped(context.Background(), other.ID, acFor("doctor")); err == nil {
		t.Fatal("expected 403 when doctor reads another doctor's appointment")
	}
}

func TestGetScoped_StaffAndAdminSeeEverything(t *testing.T) {
	other := testAppt(uuid.New(), uuid.New())
	for _, role := range []string{"staff", "admin"} {
		svc := NewServiceWithIdentity(fakeRepo{appt: other}, nil, nil, fakeIdentity{}, time.Minute)
		if _, err := svc.GetScoped(context.Background(), other.ID, acFor(role)); err != nil {
			t.Fatalf("%s should access any appointment, got %v", role, err)
		}
	}
}

func TestGetScoped_UnlinkedUserDenied(t *testing.T) {
	other := testAppt(uuid.New(), uuid.New())
	svc := NewServiceWithIdentity(fakeRepo{appt: other}, nil, nil, fakeIdentity{}, time.Minute)
	if _, err := svc.GetScoped(context.Background(), other.ID, acFor("patient")); err == nil {
		t.Fatal("expected deny for patient with no linked profile")
	}
}

func TestBookScoped_PatientForcedToOwnPatientID(t *testing.T) {
	ownPatient := uuid.New()
	var captured BookTxParams
	repo := captureRepo{onBook: func(p BookTxParams) { captured = p }}
	svc := NewServiceWithIdentity(repo, nil, nil, fakeIdentity{patientID: ownPatient}, time.Minute)

	in := BookInput{
		PatientID:       uuid.NewString(), // attacker-supplied
		DoctorID:        uuid.NewString(),
		StartTime:       time.Now().Add(48 * time.Hour),
		DurationMinutes: 30,
	}
	ac := acFor("patient")
	if _, err := svc.BookScoped(context.Background(), in, ac); err != nil {
		t.Fatalf("booking failed: %v", err)
	}
	if captured.PatientID != ownPatient {
		t.Fatalf("patient_id not overridden: got %s want %s", captured.PatientID, ownPatient)
	}
}

type captureRepo struct {
	Repository
	onBook func(BookTxParams)
}

func (c captureRepo) BookTx(ctx context.Context, p BookTxParams) (BookingResult, error) {
	c.onBook(p)
	return BookingResult{Appointment: &Appointment{ID: uuid.New()}}, nil
}
