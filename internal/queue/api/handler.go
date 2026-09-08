// Package api is the HTTP transport for the clinic queue. Staff and admins
// manage the line (check-in, call, start, complete, skip, cancel); patient
// check-ins live on the patient portal surface.
package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	idapi "github.com/PandaX185/lahza/internal/identity/api"
	"github.com/PandaX185/lahza/internal/platform/apperr"
	"github.com/PandaX185/lahza/internal/platform/httpctx"
	queuesvc "github.com/PandaX185/lahza/internal/queue/service"
)

type Handler struct {
	svc *queuesvc.Service
}

func NewHandler(svc *queuesvc.Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts tenant-scoped queue management endpoints. All queue
// actions are staff/admin only.
func (h *Handler) RegisterRoutes(protected *gin.RouterGroup) {
	g := protected.Group("/queue")
	{
		g.GET("", idapi.RequireRoles("admin", "staff"), h.ListActive)
		g.POST("", idapi.RequireRoles("admin", "staff"), h.CheckIn)
		g.PATCH("/:id", idapi.RequireRoles("admin", "staff"), h.Transition)
	}
}

// @Summary List the active queue
// @Description Returns the clinic's current line (waiting/called/in_progress) with derived positions, ordered by priority then check-in time.
// @Tags queue
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param from query string false "Only entries checked in on/after this date (YYYY-MM-DD)"
// @Param limit query int false "Page size" default(20) minimum(1) maximum(100)
// @Param offset query int false "Page offset" minimum(0)
// @Success 200 {object} queueListResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Router /queue [get]
func (h *Handler) ListActive(c *gin.Context) {
	q := queuesvc.ListQuery{Limit: 20}
	if raw := c.Query("from"); raw != "" {
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			c.Error(apperr.Invalid("from must be in YYYY-MM-DD format"))
			return
		}
		q.From = &t
	}
	if raw := c.Query("limit"); raw != "" {
		if n, err := parseInt32(raw); err == nil {
			q.Limit = n
		} else {
			c.Error(apperr.Invalid("invalid limit"))
			return
		}
	}
	if raw := c.Query("offset"); raw != "" {
		if n, err := parseInt32(raw); err == nil {
			q.Offset = n
		} else {
			c.Error(apperr.Invalid("invalid offset"))
			return
		}
	}
	items, total, err := h.svc.ListActive(c.Request.Context(), q)
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]entryResponse, 0, len(items))
	for i := range items {
		out = append(out, toEntryResponse(&items[i]))
	}
	c.JSON(http.StatusOK, queueListResponse{Items: out, Total: total, Limit: q.Limit, Offset: q.Offset})
}

// @Summary Check a patient in
// @Description Adds a patient to the clinic queue. A patient may also check in from the portal; this endpoint is for staff over-the-counter walk-ins.
// @Tags queue
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param input body checkInInput true "Patient check-in"
// @Success 201 {object} entryResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /queue [post]
func (h *Handler) CheckIn(c *gin.Context) {
	var in checkInInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	profileID, err := httpctx.ParseUUID(in.ProfileID)
	if err != nil {
		c.Error(apperr.Invalid("invalid profile_id"))
		return
	}
	var appointmentID *uuid.UUID
	if in.AppointmentID != "" {
		id, err := httpctx.ParseUUID(in.AppointmentID)
		if err != nil {
			c.Error(apperr.Invalid("invalid appointment_id"))
			return
		}
		appointmentID = &id
	}
	entry, err := h.svc.CheckIn(c.Request.Context(), profileID, appointmentID, in.Priority)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, toEntryResponse(entry))
}

// @Summary Update a queue entry
// @Description Moves an entry through the queue lifecycle: waiting -> called -> in_progress -> completed; waiting/called may also be skipped or cancelled.
// @Tags queue
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param id path string true "Queue entry id"
// @Param input body statusInput true "Target status"
// @Success 200 {object} entryResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Failure 409 {object} apperr.ErrorResponse
// @Router /queue/{id} [patch]
func (h *Handler) Transition(c *gin.Context) {
	id, err := httpctx.ParseUUIDParam(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	var in statusInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("status is required"))
		return
	}
	entry, err := h.svc.Transition(c.Request.Context(), id, in.Status)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toEntryResponse(entry))
}
