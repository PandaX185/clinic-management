package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/PandaX185/lahza/internal/platform/metrics"
	schedsvc "github.com/PandaX185/lahza/internal/scheduling/service"
)

// Logger is the minimal logging surface used by the notifier stub.
// *slog.Logger satisfies it directly.
type Logger interface {
	Info(msg string, args ...any)
}

// Stub delivers notifications to the structured log and bumps a counter. It is
// the placeholder for a real SMS/email/push provider (or a persisted inbox).
type Stub struct {
	Log Logger
	M   *metrics.Metrics
}

func (s *Stub) Deliver(ctx context.Context, n Notification) error {
	s.Log.Info("notification delivered",
		"notification_id", n.ID,
		"event", n.EventType,
		"subject", n.Subject,
		"appointment_id", n.AppointmentID,
		"patient_id", n.PatientID,
		"doctor_id", n.DoctorID,
		"starts_at", n.StartsAt.Format(time.RFC3339),
		"ends_at", n.EndsAt.Format(time.RFC3339),
		"status", n.Status,
	)
	if s.M != nil {
		s.M.NotificationsDeliveredTotal.Inc()
	}
	return nil
}

// BuildNotification renders a user-facing notification from an appointment
// event. The event type drives the subject; the body carries the window and
// the current status so recipients always see where their booking stands.
func BuildNotification(e schedsvc.Event) (Notification, error) {
	subject, err := subjectFor(e.Type)
	if err != nil {
		return Notification{}, err
	}
	appt := e.Appointment
	return Notification{
		ID:            uuid.New(),
		EventType:     e.Type,
		Subject:       subject,
		Body:          fmt.Sprintf("Appointment %s for %s..%s is now %s.", appt.ID, appt.StartTime.Format(time.RFC3339), appt.EndTime.Format(time.RFC3339), appt.Status),
		AppointmentID: appt.ID,
		PatientID:     appt.PatientID,
		DoctorID:      appt.DoctorID,
		StartsAt:      appt.StartTime,
		EndsAt:        appt.EndTime,
		Status:        string(appt.Status),
		CreatedAt:     time.Now().UTC(),
	}, nil
}

func subjectFor(eventType string) (string, error) {
	switch eventType {
	case "appointment.booked":
		return "Appointment booked", nil
	case "appointment.confirmed":
		return "Appointment confirmed", nil
	case "appointment.cancelled":
		return "Appointment cancelled", nil
	case "appointment.rescheduled":
		return "Appointment rescheduled", nil
	case "appointment.completed":
		return "Appointment completed", nil
	case "appointment.no_show":
		return "Appointment marked as no-show", nil
	case "appointment.paid":
		return "Payment received", nil
	default:
		return "", fmt.Errorf("unknown appointment event %q", eventType)
	}
}
