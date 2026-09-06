package notification

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	natsclient "github.com/PandaX185/clinic-management/internal/platform/nats"
	schedsvc "github.com/PandaX185/clinic-management/internal/scheduling/service"
)

type fakeNotifier struct {
	mu    sync.Mutex
	calls []Notification
	err   error
}

func (f *fakeNotifier) Deliver(_ context.Context, n Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, n)
	return f.err
}

func (f *fakeNotifier) got() []Notification {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Notification(nil), f.calls...)
}

func sampleEvent() schedsvc.Event {
	now := time.Date(2026, 9, 6, 10, 0, 0, 0, time.UTC)
	return schedsvc.Event{
		Type: "appointment.booked",
		Appointment: schedsvc.Appointment{
			ID:        uuid.New(),
			PatientID: uuid.New(),
			DoctorID:  uuid.New(),
			StartTime: now,
			EndTime:   now.Add(30 * time.Minute),
			Status:    schedsvc.StatusScheduled,
			Version:   1,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}

func TestHandleMessageDeliversNotification(t *testing.T) {
	e := sampleEvent()
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}

	nf := &fakeNotifier{}
	if err := HandleMessage(context.Background(), data, nf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := nf.got()
	if len(got) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(got))
	}
	n := got[0]
	if n.EventType != "appointment.booked" {
		t.Errorf("EventType = %q", n.EventType)
	}
	if n.Subject != "Appointment booked" {
		t.Errorf("Subject = %q", n.Subject)
	}
	if n.AppointmentID != e.Appointment.ID {
		t.Errorf("AppointmentID = %v, want %v", n.AppointmentID, e.Appointment.ID)
	}
	if n.PatientID != e.Appointment.PatientID || n.DoctorID != e.Appointment.DoctorID {
		t.Errorf("ids mismatch: %v / %v", n.PatientID, n.DoctorID)
	}
	if n.Status != "scheduled" {
		t.Errorf("Status = %q", n.Status)
	}
}

func TestHandleMessageMalformedPayload(t *testing.T) {
	nf := &fakeNotifier{}
	if err := HandleMessage(context.Background(), []byte("{not json"), nf); err == nil {
		t.Fatal("expected error for malformed payload")
	}
	if got := nf.got(); len(got) != 0 {
		t.Fatalf("expected no deliveries, got %d", len(got))
	}
}

func TestHandleMessageUnknownEventKind(t *testing.T) {
	data, _ := json.Marshal(schedsvc.Event{Type: "appointment.party", Appointment: sampleEvent().Appointment})
	nf := &fakeNotifier{}
	if err := HandleMessage(context.Background(), data, nf); err == nil {
		t.Fatal("expected error for unknown event kind")
	}
}

func TestHandleMessageDeliveryErrorPropagates(t *testing.T) {
	data, _ := json.Marshal(sampleEvent())
	nf := &fakeNotifier{err: context.DeadlineExceeded}
	if err := HandleMessage(context.Background(), data, nf); err == nil {
		t.Fatal("expected delivery error to propagate")
	}
}

type fakeBus struct {
	mu         sync.Mutex
	subject    string
	payload    []byte
	publishErr error
}

func (b *fakeBus) Publish(_ context.Context, subject string, payload []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subject = subject
	b.payload = payload
	return b.publishErr
}

func TestAppointmentEventPublisherPublishesToNotifySubject(t *testing.T) {
	bus := &fakeBus{}
	p := AppointmentEventPublisher{Bus: bus, Subject: natsclient.SubjectNotify}
	e := sampleEvent()
	p.PublishAppointmentEvent(context.Background(), e)

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if bus.subject != natsclient.SubjectNotify {
		t.Fatalf("subject = %q, want %q", bus.subject, natsclient.SubjectNotify)
	}
	var decoded schedsvc.Event
	if err := json.Unmarshal(bus.payload, &decoded); err != nil {
		t.Fatalf("payload did not round-trip: %v", err)
	}
	if decoded.Type != e.Type || decoded.Appointment.ID != e.Appointment.ID {
		t.Errorf("payload mismatch: %+v", decoded)
	}
}
