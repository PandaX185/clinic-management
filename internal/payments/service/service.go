// Package service holds the payment use cases: collecting payment for an
// appointment (patient-pay) and refunding a paid appointment (clinic admin).
// Every payment lives in the appointment's clinic schema, so all operations
// run against a tenant-scoped context supplied by the caller.
package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	schedsvc "github.com/PandaX185/clinic-management/internal/scheduling/service"
)

// Service implements the payment use cases behind a tenant-scoped context.
type Service struct {
	repo      Repository
	publisher schedsvc.EventPublisher
	currency  string
}

// NewService returns a payment service. publisher may be nil (NATS disabled);
// refund and pay still work, only the notification is skipped.
func NewService(repo Repository, publisher schedsvc.EventPublisher, currency string) *Service {
	return &Service{repo: repo, publisher: publisher, currency: currency}
}

// payableStatuses are appointment states a patient may pay for. Cancelled and
// no-show visits cannot be collected.
var payableStatuses = map[string]bool{
	"scheduled":   true,
	"confirmed":   true,
	"checked_in":  true,
	"in_progress": true,
	"completed":   true,
}

// validMethods are accepted instruments.
var validMethods = map[Method]bool{
	MethodCash:    true,
	MethodCard:    true,
	MethodEWallet: true,
}

// Pay collects the patient's payment for an owned appointment. It is
// idempotent: a repeated request for an already-paid appointment returns the
// existing paid payment without charging again.
func (s *Service) Pay(ctx context.Context, userID uuid.UUID, in PayInput) (*Payment, error) {
	if !validMethods[in.Method] {
		return nil, apperr.Invalid("method must be one of cash, card, e_wallet")
	}

	appt, err := s.repo.GetAppointmentForPayment(ctx, userID, in.AppointmentID)
	if err != nil {
		return nil, err
	}
	if !payableStatuses[appt.Status] {
		return nil, apperr.Conflict("cannot pay for an appointment in status " + appt.Status)
	}

	payment, err := s.repo.GetForAppointment(ctx, in.AppointmentID)
	if err != nil {
		return nil, err
	}
	if payment == nil {
		payment, err = s.repo.InsertPending(ctx, in, appt.Price, s.currency)
		if err != nil {
			return nil, err
		}
		if payment == nil {
			// Lost the insert race: another request created it. Re-read.
			payment, err = s.repo.GetForAppointment(ctx, in.AppointmentID)
			if err != nil {
				return nil, err
			}
		}
	}
	if payment == nil {
		return nil, apperr.Conflict("payment could not be recorded")
	}

	switch payment.Status {
	case StatusPaid:
		return payment, nil // idempotent replay
	case StatusRefunded:
		return nil, apperr.Conflict("this payment has already been refunded")
	}

	paid, err := s.repo.MarkPaid(ctx, payment.ID)
	if err != nil {
		return nil, err
	}
	if paid == nil {
		// Lost the update race; return whatever is now stored.
		paid, err = s.repo.GetByID(ctx, payment.ID)
		if err != nil {
			return nil, err
		}
	}
	if paid == nil {
		return nil, apperr.Conflict("payment could not be finalized")
	}
	if paid.Status == StatusRefunded {
		return nil, apperr.Conflict("this payment has already been refunded")
	}

	if s.publisher != nil {
		s.publisher.PublishAppointmentEvent(ctx, schedsvc.Event{
			Type: "appointment.paid",
			Appointment: schedsvc.Appointment{
				ID:        appt.ID,
				PatientID: appt.PatientID,
				DoctorID:  appt.DoctorID,
				StartTime: appt.StartTime,
				EndTime:   appt.EndTime,
				Status:    schedsvc.Status(appt.Status),
			},
		})
	}
	return paid, nil
}

// Refund reverses a paid payment. Only paid payments can be refunded; a
// refunded payment cannot be paid again (Pay rejects StatusRefunded). The
// expected appointment is cross-checked so a mismatched route (payment that
// does not belong to the path's appointment) is rejected rather than acting
// on an unexpected record.
func (s *Service) Refund(ctx context.Context, paymentID, expectedAppointmentID uuid.UUID) (*Payment, error) {
	payment, err := s.repo.GetByID(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	if payment == nil {
		return nil, apperr.NotFound("payment not found")
	}
	if payment.AppointmentID != expectedAppointmentID {
		return nil, apperr.NotFound("payment not found")
	}
	if payment.Status != StatusPaid {
		return nil, apperr.Conflict("only a paid payment can be refunded")
	}
	refunded, err := s.repo.MarkRefunded(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	if refunded == nil {
		// Lost the race; report the current state.
		refunded, err = s.repo.GetByID(ctx, paymentID)
		if err != nil {
			return nil, err
		}
	}
	if refunded == nil {
		return nil, apperr.Conflict("payment could not be refunded")
	}
	return refunded, nil
}
