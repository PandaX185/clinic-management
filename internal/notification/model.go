package notification

import (
	"github.com/google/uuid"
)

type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelSMS   Channel = "sms"
)

type Message struct {
	ID            uuid.UUID `json:"id"`
	AppointmentID uuid.UUID `json:"appointment_id"`
	Channel       Channel   `json:"channel"`
	Recipient     string    `json:"recipient"`
	Subject       string    `json:"subject"`
	Body          string    `json:"body"`
}

// Status values mirror the notifications table constraint.
const (
	StatusPending    = "pending"
	StatusSent       = "sent"
	StatusFailed     = "failed"
	StatusDeadLetter = "dead_letter"
)

const MaxAttempts = 5
