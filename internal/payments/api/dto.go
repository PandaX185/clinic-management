package api

import (
	"time"

	paymentsvc "github.com/PandaX185/clinic-management/internal/payments/service"
)

type paymentResponse struct {
	ID            string     `json:"id"`
	AppointmentID string     `json:"appointment_id"`
	Amount        string     `json:"amount"`
	Currency      string     `json:"currency"`
	Method        string     `json:"method"`
	Status        string     `json:"status"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
	Reference     *string    `json:"reference,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func toPaymentResponse(p *paymentsvc.Payment) paymentResponse {
	return paymentResponse{
		ID:            p.ID.String(),
		AppointmentID: p.AppointmentID.String(),
		Amount:        p.Amount,
		Currency:      p.Currency,
		Method:        string(p.Method),
		Status:        string(p.Status),
		PaidAt:        p.PaidAt,
		Reference:     p.Reference,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}
