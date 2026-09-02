package service

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
	svc := NewServiceWithIdentity(fakeRepo{appt: testAppt(ownPatient, uuid.New())}, nil,
		fakeIdentity{patientID: ownPatient}, time.Minute)

	other := testAppt(uuid.New(), uuid.New())
	repo := fakeRepo{appt: other}
	svc2 := NewServiceWithIdentity(repo, nil, fakeIdentity{patientID: ownPatient}, time.Minute)

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
	svc := NewServiceWithIdentity(fakeRepo{appt: other}, nil,
		fakeIdentity{doctorID: ownDoctor}, time.Minute)

	if _, err := svc.GetScoped(context.Background(), other.ID, acFor("doctor")); err == nil {
		t.Fatal("expected 403 when doctor reads another doctor's appointment")
	}
}

func TestGetScoped_StaffAndAdminSeeEverything(t *testing.T) {
	other := testAppt(uuid.New(), uuid.New())
	for _, role := range []string{"staff", "admin"} {
		svc := NewServiceWithIdentity(fakeRepo{appt: other}, nil, fakeIdentity{}, time.Minute)
		if _, err := svc.GetScoped(context.Background(), other.ID, acFor(role)); err != nil {
			t.Fatalf("%s should access any appointment, got %v", role, err)
		}
	}
}

func TestGetScoped_UnlinkedUserDenied(t *testing.T) {
	other := testAppt(uuid.New(), uuid.New())
	svc := NewServiceWithIdentity(fakeRepo{appt: other}, nil, fakeIdentity{}, time.Minute)
	if _, err := svc.GetScoped(context.Background(), other.ID, acFor("patient")); err == nil {
		t.Fatal("expected deny for patient with no linked profile")
	}
}

func TestBookScoped_PatientForcedToOwnPatientID(t *testing.T) {
	ownPatient := uuid.New()
	var captured BookTxParams
	repo := captureRepo{onBook: func(p BookTxParams) { captured = p }}
	svc := NewServiceWithIdentity(repo, nil, fakeIdentity{patientID: ownPatient}, time.Minute)

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

// Doctor access regression (SEC-02): a doctor-only caller must be scoped to
// their own doctor profile id in list queries, exactly as a patient is scoped
// to their patient profile id for booking. The DoctorIDForUser resolver now
// returns the caller's profile id.
func TestListScoped_DoctorScopedToOwnDoctorID(t *testing.T) {
	ownDoctor := uuid.New()
	var captured ListQuery
	repo := captureListRepo{onList: func(q ListQuery) { captured = q }}
	svc := NewServiceWithIdentity(repo, nil, fakeIdentity{doctorID: ownDoctor}, time.Minute)

	items, total, err := svc.ListScoped(context.Background(), ListQuery{}, acFor("doctor"))
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if captured.DoctorID != ownDoctor.String() {
		t.Fatalf("doctor_id not scoped to own profile: got %s want %s", captured.DoctorID, ownDoctor)
	}
	if captured.PatientID != "" {
		t.Fatalf("patient_id should be cleared for doctor scope, got %q", captured.PatientID)
	}
	if len(items) != 0 || total != 0 {
		t.Fatalf("expected empty result list from capture repo, got %d items/%d total", len(items), total)
	}
}

// A user holding both doctor and patient roles may access appointments they
// are linked to on either side (previously the mixed-role path was denied).
func TestGetScoped_MixedDoctorPatientRoles(t *testing.T) {
	ownPatient := uuid.New()
	ownDoctor := uuid.New()
	appt := testAppt(ownPatient, ownDoctor)
	svc := NewServiceWithIdentity(fakeRepo{appt: appt}, nil, fakeIdentity{patientID: ownPatient, doctorID: ownDoctor}, time.Minute)

	if _, err := svc.GetScoped(context.Background(), appt.ID, acFor("patient", "doctor")); err != nil {
		t.Fatalf("mixed-role user should access appointment they link to, got %v", err)
	}
}

type captureListRepo struct {
	Repository
	onList func(ListQuery)
}

func (c captureListRepo) List(ctx context.Context, q ListQuery) ([]Appointment, int64, error) {
	c.onList(q)
	return []Appointment{}, 0, nil
}

type captureRepo struct {
	Repository
	onBook func(BookTxParams)
}

func (c captureRepo) BookTx(ctx context.Context, p BookTxParams) (BookingResult, error) {
	c.onBook(p)
	return BookingResult{Appointment: &Appointment{ID: uuid.New()}}, nil
}
