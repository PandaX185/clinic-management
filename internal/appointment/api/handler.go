// Package api is the HTTP transport layer for the appointments feature. It
// maps gin requests to the appointment service and owns response DTOs and the
// idempotency replay handling. It depends on appointment/service and
// auth/api; nothing below it depends back on this package.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/appointment/service"
	authapi "github.com/PandaX185/clinic-management/internal/auth/api"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/httpctx"
)

const idempotencyHeader = "Idempotency-Key"

type Handler struct {
	svc *service.Service
}

func NewHandler(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/appointments")
	{
		g.POST("", h.Book)
		g.GET("", h.List)
		g.GET("/:id", h.Get)
		g.POST("/:id/cancel", h.Cancel)
		g.POST("/:id/reschedule", h.Reschedule)
		// Staff/clinic actions: a patient may not confirm, complete, or mark
		// no-show — those transition an appointment's clinical state and must
		// be performed by clinic staff or an admin.
		g.POST("/:id/confirm", authapi.RequireRoles("admin", "staff"), h.Confirm)
		g.POST("/:id/complete", authapi.RequireRoles("admin", "staff"), h.Complete)
		g.POST("/:id/no-show", authapi.RequireRoles("admin", "staff"), h.MarkNoShow)
	}
}

// Book handles FR-APT-01/06/07: creation that is concurrency-safe and
// idempotent when a client provides the Idempotency-Key header.
//
// @Summary Book an appointment
// @Description Creates an appointment in the active tenant (X-Tenant-ID). Concurrency-safe against overlapping schedules; providing the Idempotency-Key header makes the request repeatable — a replayed request returns the stored response with an Idempotent-Replay: true header.
// @Tags appointments
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param Idempotency-Key header string false "Client-generated idempotency key"
// @Param input body service.BookInput true "Appointment details"
// @Success 201 {object} appointmentResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 409 {object} apperr.ErrorResponse
// @Failure 500 {object} apperr.ErrorResponse
// @Router /appointments [post]
func (h *Handler) Book(c *gin.Context) {
	var in service.BookInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	in.IdempotencyKey = c.GetHeader(idempotencyHeader)

	result, err := h.svc.BookScoped(c.Request.Context(), in, accessContextOf(c))
	if err != nil {
		c.Error(err)
		return
	}

	if result.Replayed {
		c.Writer.Header().Set("Idempotent-Replay", "true")
		c.Data(result.StoredStatus, "application/json", result.StoredBody)
		return
	}
	body, _ := json.Marshal(toResponse(result.Appointment))
	c.Data(http.StatusCreated, "application/json", body)
}

// @Summary Get appointment
// @Description Returns a single appointment by id within the active tenant.
// @Tags appointments
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param id path string true "Appointment id"
// @Success 200 {object} appointmentResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /appointments/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.Error(err)
		return
	}
	a, err := h.svc.GetScoped(c.Request.Context(), id, accessContextOf(c))
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toResponse(a))
}

// @Summary List appointments
// @Description Lists appointments in the active tenant with optional filters and pagination.
// @Tags appointments
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param patient_id query string false "Filter by patient id"
// @Param doctor_id query string false "Filter by doctor id"
// @Param status query string false "Filter by status" Enums(scheduled, confirmed, completed, cancelled, no_show)
// @Param from query string false "Start of time window (RFC3339)"
// @Param to query string false "End of time window (RFC3339)"
// @Param limit query int false "Page size" default(20) minimum(1) maximum(100)
// @Param offset query int false "Page offset" minimum(0)
// @Success 200 {object} appointmentsListResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Router /appointments [get]
func (h *Handler) List(c *gin.Context) {
	var q service.ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.Error(apperr.Invalid("invalid query parameters"))
		return
	}
	items, total, err := h.svc.ListScoped(c.Request.Context(), q, accessContextOf(c))
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]appointmentResponse, 0, len(items))
	for i := range items {
		out = append(out, *toResponse(&items[i]))
	}
	c.JSON(http.StatusOK, gin.H{"items": out, "total": total, "limit": q.Limit, "offset": q.Offset})
}

// @Summary Cancel appointment
// @Description Cancels an appointment in the active tenant with a required reason.
// @Tags appointments
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param id path string true "Appointment id"
// @Param input body service.CancelInput true "Cancellation reason"
// @Success 200 {object} appointmentResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /appointments/{id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.Error(err)
		return
	}
	var in service.CancelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("reason is required"))
		return
	}
	a, err := h.svc.CancelScoped(c.Request.Context(), id, in.Reason, accessContextOf(c))
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toResponse(a))
}

// @Summary Reschedule appointment
// @Description Moves an appointment to a new start time and/or duration in the active tenant.
// @Tags appointments
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param id path string true "Appointment id"
// @Param input body service.RescheduleInput true "New timing"
// @Success 200 {object} appointmentResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /appointments/{id}/reschedule [post]
func (h *Handler) Reschedule(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.Error(err)
		return
	}
	var in service.RescheduleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	a, err := h.svc.RescheduleScoped(c.Request.Context(), id, in, accessContextOf(c))
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toResponse(a))
}

// @Summary Confirm appointment
// @Description Marks an appointment as confirmed. Requires the admin or staff role in the active tenant; patients may not perform this action.
// @Tags appointments
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param id path string true "Appointment id"
// @Success 200 {object} appointmentResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /appointments/{id}/confirm [post]
func (h *Handler) Confirm(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.Error(err)
		return
	}
	a, err := h.svc.ConfirmScoped(c.Request.Context(), id, accessContextOf(c))
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toResponse(a))
}

// @Summary Complete appointment
// @Description Marks an appointment as completed. Requires the admin or staff role in the active tenant; patients may not perform this action.
// @Tags appointments
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param id path string true "Appointment id"
// @Success 200 {object} appointmentResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /appointments/{id}/complete [post]
func (h *Handler) Complete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.Error(err)
		return
	}
	a, err := h.svc.CompleteScoped(c.Request.Context(), id, accessContextOf(c))
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toResponse(a))
}

// @Summary Mark appointment as no-show
// @Description Marks an appointment as a no-show. Requires the admin or staff role in the active tenant; patients may not perform this action.
// @Tags appointments
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param id path string true "Appointment id"
// @Success 200 {object} appointmentResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /appointments/{id}/no-show [post]
func (h *Handler) MarkNoShow(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.Error(err)
		return
	}
	a, err := h.svc.MarkNoShowScoped(c.Request.Context(), id, accessContextOf(c))
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toResponse(a))
}

func parseID(c *gin.Context) (uuid.UUID, error) {
	return httpctx.ParseUUIDParam(c, "id")
}

// accessContextOf builds the service-level AccessContext from middleware-set
// identity claims.
func accessContextOf(c *gin.Context) service.AccessContext {
	ac := service.AccessContext{Roles: httpctx.Roles(c)}
	uid, err := httpctx.UserID(c)
	if err == nil {
		ac.UserID = uid
		ac.ActorID = &uid
	}
	return ac
}
