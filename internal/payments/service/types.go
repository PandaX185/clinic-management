// Package service holds the payment use cases: collecting payment for an
// appointment (patient pay) and refunding a paid appointment (admin). All
// state lives in the appointment's clinic schema, so every operation runs
// against a tenant-scoped context supplied by the caller.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Method is the payment instrument used to collect a payment.
type Method string

const (
	MethodCash    Method = "cash"
	MethodCard    Method = "card"
	MethodEWallet Method = "e_wallet"
)

// Status of a payment record.
type Status string

const (
	StatusPending  Status = "pending"
	StatusPaid     Status = "paid"
	StatusFailed   Status = "failed"
	StatusRefunded Status = "refunded"
)

// Payment is a clinic's payment record for one appointment. One payment per
// appointment (the schema enforces uniqueness on appointment_id).
type Payment struct {
	ID            uuid.UUID
	AppointmentID uuid.UUID
	Amount        string
	Currency      string
	Method        Method
	Status        Status
	PaidAt        *time.Time
	Reference     *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// PayInput describes a payment the patient wants to collect for an
// appointment. Amount is resolved from the appointment type price; the input
// carries only the instrument.
type PayInput struct {
	AppointmentID uuid.UUID
	Method        Method
	Reference     string
}

// Appointment is the subset of an appointment needed to authorize a payment
// (ownership, payability) and resolve its price.
type Appointment struct {
	ID        uuid.UUID
	PatientID uuid.UUID
	DoctorID  uuid.UUID
	TypeID    uuid.UUID
	StartTime time.Time
	EndTime   time.Time
	Status    string
	Price     string
	Currency  string
}

// Repository is the persistence port for payments. Every method runs inside a
// tenant schema pinned on the context by the caller.
type Repository interface {
	// GetAppointmentForPayment returns the appointment only when it belongs to
	// userID, along with its resolved price.
	GetAppointmentForPayment(ctx context.Context, userID, apptID uuid.UUID) (*Appointment, error)
	// InsertPending creates the pending payment for an appointment. A second
	// call for the same appointment returns (nil, nil) because the unique
	// constraint on appointment_id rejects the insert.
	InsertPending(ctx context.Context, in PayInput, amount, currency string) (*Payment, error)
	// GetForAppointment returns the payment for an appointment or (nil, nil)
	// when none exists.
	GetForAppointment(ctx context.Context, appointmentID uuid.UUID) (*Payment, error)
	// GetByID returns the payment by id or (nil, nil) when not found.
	GetByID(ctx context.Context, paymentID uuid.UUID) (*Payment, error)
	// MarkPaid transitions a pending payment to paid, stamping paid_at.
	// Returns (nil, nil) when the payment was not pending.
	MarkPaid(ctx context.Context, paymentID uuid.UUID) (*Payment, error)
	// MarkRefunded transitions a paid payment to refunded. Returns (nil, nil)
	// when the payment was not paid.
	MarkRefunded(ctx context.Context, paymentID uuid.UUID) (*Payment, error)
}
