package appointment

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	auth "github.com/PandaX185/clinic-management/internal/auth"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

const idempotencyHeader = "Idempotency-Key"

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

type appointmentResponse struct {
	ID                 string  `json:"id"`
	PatientID          string  `json:"patient_id"`
	DoctorID           string  `json:"doctor_id"`
	StartTime          string  `json:"start_time"`
	EndTime            string  `json:"end_time"`
	Status             string  `json:"status"`
	Notes              *string `json:"notes,omitempty"`
	CancellationReason *string `json:"cancellation_reason,omitempty"`
	Version            int32   `json:"version"`
	CreatedAt          string  `json:"created_at"`
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
		g.POST("/:id/confirm", auth.RequireRoles("admin", "staff"), h.Confirm)
		g.POST("/:id/complete", auth.RequireRoles("admin", "staff"), h.Complete)
		g.POST("/:id/no-show", auth.RequireRoles("admin", "staff"), h.MarkNoShow)
	}
}

// Book handles FR-APT-01/06/07: creation that is concurrency-safe and
// idempotent when a client provides the Idempotency-Key header.
func (h *Handler) Book(c *gin.Context) {
	var in BookInput
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

func (h *Handler) List(c *gin.Context) {
	var q ListQuery
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

func (h *Handler) Cancel(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.Error(err)
		return
	}
	var in CancelInput
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

func (h *Handler) Reschedule(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.Error(err)
		return
	}
	var in RescheduleInput
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
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return uuid.Nil, apperr.Invalid("invalid id format")
	}
	return id, nil
}

func actorID(c *gin.Context) *uuid.UUID {
	v, ok := c.Get("auth_user_id")
	if !ok {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}

// accessContextOf builds the service-level AccessContext from middleware-set
// identity claims.
func accessContextOf(c *gin.Context) AccessContext {
	ac := AccessContext{ActorID: actorID(c)}
	if ac.ActorID != nil {
		ac.UserID = *ac.ActorID
	}
	if raw, ok := c.Get("auth_roles"); ok {
		if roles, ok := raw.([]string); ok {
			ac.Roles = roles
		}
	}
	return ac
}

func toResponse(a *Appointment) *appointmentResponse {
	return &appointmentResponse{
		ID:                 a.ID.String(),
		PatientID:          a.PatientID.String(),
		DoctorID:           a.DoctorID.String(),
		StartTime:          a.StartTime.Format("2006-01-02T15:04:05Z07:00"),
		EndTime:            a.EndTime.Format("2006-01-02T15:04:05Z07:00"),
		Status:             string(a.Status),
		Notes:              a.Notes,
		CancellationReason: a.CancellationReason,
		Version:            a.Version,
		CreatedAt:          a.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
