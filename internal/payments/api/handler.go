// Package api is the HTTP transport for payment operations. Refunds are
// clinic-admin actions and ride the tenant-scoped protected group: the clinic
// is chosen by X-Tenant-ID and the caller must hold the admin role there.
// Patient payments live on the portal surface instead (they are the patient's
// own action, not a clinic member action).
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	idapi "github.com/PandaX185/lahza/internal/identity/api"
	paymentsvc "github.com/PandaX185/lahza/internal/payments/service"
	"github.com/PandaX185/lahza/internal/platform/apperr"
	"github.com/PandaX185/lahza/internal/platform/httpctx"
)

// HeaderClinicID is the header that selects the clinic; kept in sync with
// the server package's constant, and compared against the path clinic id so
// a mismatched route is rejected rather than silently refunding elsewhere.
const HeaderClinicID = "X-Tenant-ID"

// Handler serves payment endpoints.
type Handler struct {
	svc *paymentsvc.Service
}

func NewHandler(svc *paymentsvc.Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts tenant-scoped payment management endpoints. All are
// admin-only.
func (h *Handler) RegisterRoutes(protected *gin.RouterGroup) {
	g := protected.Group("/clinics/:id")
	{
		ag := g.Group("/appointments/:apptId/payments/:paymentId")
		ag.POST("/refund", idapi.RequireRoles("admin"), h.Refund)
	}
}

// @Summary Refund a payment
// @Description Reverses a paid payment in the active clinic. Only a payment in paid status can be refunded.
// @Tags payments
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id (clinic the payment lives in)"
// @Param id path string true "Clinic id"
// @Param apptId path string true "Appointment id the payment belongs to"
// @Param paymentId path string true "Payment id to refund"
// @Success 200 {object} paymentResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Failure 409 {object} apperr.ErrorResponse
// @Router /clinics/{id}/appointments/{apptId}/payments/{paymentId}/refund [post]
func (h *Handler) Refund(c *gin.Context) {
	clinicID, err := httpctx.ParseUUIDParam(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	apptID, err := httpctx.ParseUUIDParam(c, "apptId")
	if err != nil {
		c.Error(err)
		return
	}
	paymentID, err := httpctx.ParseUUIDParam(c, "paymentId")
	if err != nil {
		c.Error(err)
		return
	}
	headerID, err := httpctx.ParseUUID(c.GetHeader(HeaderClinicID))
	if err != nil || headerID != clinicID {
		c.Error(apperr.Forbidden("X-Tenant-ID must match the clinic in the path"))
		return
	}
	payment, err := h.svc.Refund(c.Request.Context(), paymentID, apptID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toPaymentResponse(payment))
}
