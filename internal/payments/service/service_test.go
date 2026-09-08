package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	paymentsvc "github.com/PandaX185/lahza/internal/payments/service"
	"github.com/PandaX185/lahza/internal/platform/apperr"
	schedsvc "github.com/PandaX185/lahza/internal/scheduling/service"
)

func TestPay_CollectsPriceAndMarksPaid(t *testing.T) {
	userID, apptID := uuid.New(), uuid.New()
	appt := &paymentsvc.Appointment{ID: apptID, PatientID: userID, Status: "scheduled", Price: "500.00", Currency: "EGP"}
	stub := &stubRepo{appt: appt}
	pub := &stubPublisher{}
	svc := paymentsvc.NewService(stub, pub, "EGP")

	got, err := svc.Pay(context.Background(), userID, paymentsvc.PayInput{AppointmentID: apptID, Method: paymentsvc.MethodCard})
	if err != nil {
		t.Fatalf("Pay: %v", err)
	}
	if got.Status != paymentsvc.StatusPaid {
		t.Errorf("expected paid, got %s", got.Status)
	}
	if got.Amount != "500.00" {
		t.Errorf("expected amount 500.00, got %s", got.Amount)
	}
	if stub.insertedAmount != "500.00" {
		t.Errorf("InsertPending amount = %q", stub.insertedAmount)
	}
	if len(pub.events) != 1 || pub.events[0].Type != "appointment.paid" {
		t.Fatalf("expected one appointment.paid event, got %+v", pub.events)
	}
}

func TestPay_InvalidMethodRejected(t *testing.T) {
	svc := paymentsvc.NewService(&stubRepo{}, nil, "EGP")
	_, err := svc.Pay(context.Background(), uuid.New(), paymentsvc.PayInput{
		AppointmentID: uuid.New(),
		Method:        paymentsvc.Method("bitcoin"),
	})
	if ae := apperr.From(err); ae.Kind != apperr.KindInvalid {
		t.Fatalf("expected Invalid, got %v", err)
	}
}

func TestPay_UnpayableAppointmentRejected(t *testing.T) {
	userID, apptID := uuid.New(), uuid.New()
	stub := &stubRepo{appt: &paymentsvc.Appointment{ID: apptID, PatientID: userID, Status: "cancelled"}}
	svc := paymentsvc.NewService(stub, nil, "EGP")
	_, err := svc.Pay(context.Background(), userID, paymentsvc.PayInput{AppointmentID: apptID, Method: paymentsvc.MethodCash})
	if ae := apperr.From(err); ae.Kind != apperr.KindConflict {
		t.Fatalf("expected Conflict, got %v", err)
	}
	if stub.inserted != nil {
		t.Errorf("insert must not run for unpayable appointment")
	}
}

func TestPay_OwnershipCheckThreadsUserID(t *testing.T) {
	userID, apptID := uuid.New(), uuid.New()
	stub := &stubRepo{ownedBy: userID, appt: &paymentsvc.Appointment{ID: apptID, PatientID: userID, Status: "scheduled", Price: "10.00"}}
	svc := paymentsvc.NewService(stub, nil, "EGP")
	_, err := svc.Pay(context.Background(), userID, paymentsvc.PayInput{AppointmentID: apptID, Method: paymentsvc.MethodCard})
	if err != nil {
		t.Fatalf("Pay: %v", err)
	}
	if stub.apptUser != userID {
		t.Errorf("GetAppointmentForPayment called with user %v, want %v", stub.apptUser, userID)
	}
}

func TestPay_IdempotentWhenAlreadyPaid(t *testing.T) {
	userID, apptID := uuid.New(), uuid.New()
	paidPayment := &paymentsvc.Payment{ID: uuid.New(), AppointmentID: apptID, Status: paymentsvc.StatusPaid, Amount: "100.00"}
	stub := &stubRepo{
		appt:   &paymentsvc.Appointment{ID: apptID, PatientID: userID, Status: "scheduled", Price: "100.00"},
		paying: paidPayment,
	}
	pub := &stubPublisher{}
	svc := paymentsvc.NewService(stub, pub, "EGP")

	got, err := svc.Pay(context.Background(), userID, paymentsvc.PayInput{AppointmentID: apptID, Method: paymentsvc.MethodCard})
	if err != nil {
		t.Fatalf("Pay: %v", err)
	}
	if got != paidPayment {
		t.Errorf("expected the existing paid payment back")
	}
	if len(pub.events) != 0 {
		t.Errorf("idempotent replay must not emit another paid event")
	}
}

func TestPay_RaceLoserDoesNotEmitEvent(t *testing.T) {
	userID, apptID := uuid.New(), uuid.New()
	// Simulate losing the pending->paid race: another request wins the
	// transition (MarkPaid returns nil) and the payment is already paid.
	stub := &stubRepo{
		appt:         &paymentsvc.Appointment{ID: apptID, PatientID: userID, Status: "scheduled", Price: "50.00"},
		one:          &paymentsvc.Payment{ID: uuid.New(), AppointmentID: apptID, Status: paymentsvc.StatusPaid, Amount: "50.00"},
		loseMarkPaid: true,
	}
	pub := &stubPublisher{}
	svc := paymentsvc.NewService(stub, pub, "EGP")

	got, err := svc.Pay(context.Background(), userID, paymentsvc.PayInput{AppointmentID: apptID, Method: paymentsvc.MethodCard})
	if err != nil {
		t.Fatalf("Pay: %v", err)
	}
	if got.Status != paymentsvc.StatusPaid {
		t.Errorf("expected paid, got %s", got.Status)
	}
	if len(pub.events) != 0 {
		t.Errorf("race loser must not emit appointment.paid, got %+v", pub.events)
	}
}

func TestPay_RejectsRefundedPayment(t *testing.T) {
	userID, apptID := uuid.New(), uuid.New()
	stub := &stubRepo{
		appt:  &paymentsvc.Appointment{ID: apptID, PatientID: userID, Status: "scheduled", Price: "100.00"},
		first: &paymentsvc.Payment{ID: uuid.New(), AppointmentID: apptID, Status: paymentsvc.StatusRefunded},
	}
	svc := paymentsvc.NewService(stub, nil, "EGP")
	_, err := svc.Pay(context.Background(), userID, paymentsvc.PayInput{AppointmentID: apptID, Method: paymentsvc.MethodCard})
	if ae := apperr.From(err); ae.Kind != apperr.KindConflict {
		t.Fatalf("expected Conflict, got %v", err)
	}
}

func TestRefund_ReversesPaidPayment(t *testing.T) {
	paymentID, apptID := uuid.New(), uuid.New()
	now := time.Now()
	paid := &paymentsvc.Payment{ID: paymentID, AppointmentID: apptID, Status: paymentsvc.StatusPaid, PaidAt: &now}
	stub := &stubRepo{one: paid}
	svc := paymentsvc.NewService(stub, nil, "EGP")

	got, err := svc.Refund(context.Background(), paymentID, apptID)
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if got.Status != paymentsvc.StatusRefunded {
		t.Errorf("expected refunded, got %s", got.Status)
	}
}

func TestRefund_RejectsUnownedMismatchedAppointment(t *testing.T) {
	paymentID, apptID, otherAppt := uuid.New(), uuid.New(), uuid.New()
	paid := &paymentsvc.Payment{ID: paymentID, AppointmentID: otherAppt, Status: paymentsvc.StatusPaid}
	stub := &stubRepo{one: paid}
	svc := paymentsvc.NewService(stub, nil, "EGP")

	_, err := svc.Refund(context.Background(), paymentID, apptID)
	if ae := apperr.From(err); ae.Kind != apperr.KindNotFound {
		t.Fatalf("expected NotFound for mismatched appointment, got %v", err)
	}
}

func TestRefund_RejectsPendingPayment(t *testing.T) {
	paymentID, apptID := uuid.New(), uuid.New()
	stub := &stubRepo{one: &paymentsvc.Payment{ID: paymentID, AppointmentID: apptID, Status: paymentsvc.StatusPending}}
	svc := paymentsvc.NewService(stub, nil, "EGP")

	_, err := svc.Refund(context.Background(), paymentID, apptID)
	if ae := apperr.From(err); ae.Kind != apperr.KindConflict {
		t.Fatalf("expected Conflict, got %v", err)
	}
}

func TestRefund_NotFound(t *testing.T) {
	svc := paymentsvc.NewService(&stubRepo{}, nil, "EGP")
	_, err := svc.Refund(context.Background(), uuid.New(), uuid.New())
	if ae := apperr.From(err); ae.Kind != apperr.KindNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

var _ paymentsvc.Repository = (*stubRepo)(nil)

type stubRepo struct {
	appt     *paymentsvc.Appointment
	ownedBy  uuid.UUID
	apptUser uuid.UUID

	first  *paymentsvc.Payment // returned by GetForAppointment on first call
	one    *paymentsvc.Payment // returned by GetByID
	paying *paymentsvc.Payment // result of InsertPending / MarkPaid

	loseMarkPaid bool // MarkPaid returns nil (simulates losing the race)

	inserted       *paymentsvc.Payment
	insertedAmount string
}

func (s *stubRepo) GetAppointmentForPayment(_ context.Context, userID, _ uuid.UUID) (*paymentsvc.Appointment, error) {
	s.apptUser = userID
	if s.ownedBy != uuid.Nil && userID != s.ownedBy {
		return nil, apperr.NotFound("appointment not found")
	}
	return s.appt, nil
}

func (s *stubRepo) InsertPending(_ context.Context, in paymentsvc.PayInput, amount, _ string) (*paymentsvc.Payment, error) {
	s.insertedAmount = amount
	if s.paying == nil {
		s.paying = &paymentsvc.Payment{ID: uuid.New(), AppointmentID: in.AppointmentID, Status: paymentsvc.StatusPending, Amount: amount}
	}
	s.inserted = s.paying
	return s.paying, nil
}

func (s *stubRepo) GetForAppointment(_ context.Context, _ uuid.UUID) (*paymentsvc.Payment, error) {
	if s.first != nil {
		p := s.first
		s.first = nil
		return p, nil
	}
	return s.paying, nil
}

func (s *stubRepo) GetByID(_ context.Context, _ uuid.UUID) (*paymentsvc.Payment, error) {
	if s.one != nil {
		return s.one, nil
	}
	return s.paying, nil
}

func (s *stubRepo) MarkPaid(_ context.Context, id uuid.UUID) (*paymentsvc.Payment, error) {
	if s.loseMarkPaid {
		return nil, nil
	}
	if s.paying == nil || s.paying.ID != id {
		return nil, nil
	}
	now := time.Now()
	s.paying.Status = paymentsvc.StatusPaid
	s.paying.PaidAt = &now
	return s.paying, nil
}

func (s *stubRepo) MarkRefunded(_ context.Context, id uuid.UUID) (*paymentsvc.Payment, error) {
	if s.paying != nil && s.paying.ID == id {
		s.paying.Status = paymentsvc.StatusRefunded
		return s.paying, nil
	}
	if s.one != nil && s.one.ID == id {
		s.one.Status = paymentsvc.StatusRefunded
		return s.one, nil
	}
	return nil, nil
}

var _ schedsvc.EventPublisher = (*stubPublisher)(nil)

type stubPublisher struct {
	events []schedsvc.Event
}

func (p *stubPublisher) PublishAppointmentEvent(_ context.Context, e schedsvc.Event) {
	p.events = append(p.events, e)
}
