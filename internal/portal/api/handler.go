package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/httpctx"
	"github.com/PandaX185/clinic-management/internal/portal/service"
)

// Handler serves the patient portal endpoints. Every handler resolves the
// caller's identity from the JWT and operates strictly on the caller's own
// data; no X-Tenant-ID is required because the clinic is always explicit in
// the request or resolved per appointment.
type Handler struct {
	svc *service.Service
}

func NewHandler(svc *service.Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes is mounted on the global (JWT-protected, tenant-less) group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/portal")
	{
		g.GET("/me", h.Me)
		g.PUT("/me", h.UpdateMe)
		g.GET("/appointments", h.ListAppointments)
		g.GET("/appointments/:id", h.GetAppointment)
		g.POST("/appointments", h.Book)
		g.POST("/appointments/:id/cancel", h.Cancel)
		g.POST("/appointments/:id/reschedule", h.Reschedule)
	}
}

// @Summary Get my profile
// @Description Returns the patient's global identity.
// @Tags portal
// @Produce json
// @Security BearerAuth
// @Success 200 {object} userResponse
// @Failure 401 {object} apperr.ErrorResponse
// @Router /portal/me [get]
func (h *Handler) Me(c *gin.Context) {
	userID, err := httpctx.UserID(c)
	if err != nil {
		c.Error(err)
		return
	}
	user, err := h.svc.Me(c.Request.Context(), userID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toUserResponse(user))
}

// @Summary Update my profile
// @Description Updates the patient's display name and phone.
// @Tags portal
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param input body updateUserInput true "Identity fields"
// @Success 200 {object} userResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 409 {object} apperr.ErrorResponse
// @Router /portal/me [put]
func (h *Handler) UpdateMe(c *gin.Context) {
	userID, err := httpctx.UserID(c)
	if err != nil {
		c.Error(err)
		return
	}
	var in updateUserInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	if err := h.svc.UpdateMe(c.Request.Context(), userID, service.UpdateUserInput{
		FullName: in.FullName,
		Phone:    in.Phone,
	}); err != nil {
		c.Error(err)
		return
	}
	user, err := h.svc.Me(c.Request.Context(), userID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toUserResponse(user))
}

// @Summary List my appointments
// @Description Returns the patient's appointments across every clinic they attend.
// @Tags portal
// @Produce json
// @Security BearerAuth
// @Success 200 {object} appointmentsListResponse
// @Failure 401 {object} apperr.ErrorResponse
// @Router /portal/appointments [get]
func (h *Handler) ListAppointments(c *gin.Context) {
	userID, err := httpctx.UserID(c)
	if err != nil {
		c.Error(err)
		return
	}
	items, err := h.svc.ListAppointments(c.Request.Context(), userID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, appointmentsListResponse{Items: toAppointmentResponses(items)})
}

// @Summary Get my appointment
// @Description Returns one of the patient's appointments. The clinic it belongs to is passed explicitly.
// @Tags portal
// @Produce json
// @Security BearerAuth
// @Param id path string true "Appointment id"
// @Param clinic_id query string true "Clinic the appointment belongs to"
// @Success 200 {object} appointmentResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /portal/appointments/{id} [get]
func (h *Handler) GetAppointment(c *gin.Context) {
	userID, err := httpctx.UserID(c)
	if err != nil {
		c.Error(err)
		return
	}
	apptID, err := httpctx.ParseUUIDParam(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	clinicID, err := httpctx.ParseUUID(c.Query("clinic_id"))
	if err != nil {
		c.Error(apperr.Invalid("clinic_id is required"))
		return
	}
	appt, err := h.svc.GetAppointment(c.Request.Context(), userID, apptID, clinicID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toAppointmentResponse(appt))
}

// @Summary Book an appointment
// @Description Books a new appointment for the patient at the given clinic. The patient profile is provisioned on first booking.
// @Tags portal
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param input body bookInput true "Booking details"
// @Param Idempotency-Key header string false "Client-generated idempotency key to make the booking repeatable"
// @Success 201 {object} appointmentResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Failure 409 {object} apperr.ErrorResponse
// @Router /portal/appointments [post]
func (h *Handler) Book(c *gin.Context) {
	userID, err := httpctx.UserID(c)
	if err != nil {
		c.Error(err)
		return
	}
	var in bookInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	clinicID, err := httpctx.ParseUUID(in.ClinicID)
	if err != nil {
		c.Error(err)
		return
	}
	doctorID, err := httpctx.ParseUUID(in.DoctorID)
	if err != nil {
		c.Error(err)
		return
	}
	if in.DurationMinutes <= 0 {
		in.DurationMinutes = 30
	}
	appt, err := h.svc.Book(c.Request.Context(), userID, service.BookInput{
		ClinicID:        clinicID,
		DoctorID:        doctorID,
		StartTime:       in.StartTime,
		DurationMinutes: in.DurationMinutes,
		Notes:           in.Notes,
		IdempotencyKey:  c.GetHeader("Idempotency-Key"),
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, toAppointmentResponse(appt))
}

// @Summary Cancel my appointment
// @Description Cancels one of the patient's appointments in the given clinic with a required reason.
// @Tags portal
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Appointment id"
// @Param input body cancelInput true "Clinic and cancellation reason"
// @Success 200 {object} appointmentResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /portal/appointments/{id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) {
	userID, err := httpctx.UserID(c)
	if err != nil {
		c.Error(err)
		return
	}
	apptID, err := httpctx.ParseUUIDParam(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	var in cancelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("clinic_id and reason are required"))
		return
	}
	clinicID, err := httpctx.ParseUUID(in.ClinicID)
	if err != nil {
		c.Error(apperr.Invalid("clinic_id is required"))
		return
	}
	appt, err := h.svc.Cancel(c.Request.Context(), userID, clinicID, apptID, in.Reason)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toAppointmentResponse(appt))
}

// @Summary Reschedule my appointment
// @Description Moves one of the patient's appointments to a new time in the same clinic.
// @Tags portal
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Appointment id"
// @Param input body rescheduleInput true "Clinic and new timing"
// @Success 200 {object} appointmentResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /portal/appointments/{id}/reschedule [post]
func (h *Handler) Reschedule(c *gin.Context) {
	userID, err := httpctx.UserID(c)
	if err != nil {
		c.Error(err)
		return
	}
	apptID, err := httpctx.ParseUUIDParam(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	var in rescheduleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("clinic_id and start_time are required"))
		return
	}
	clinicID, err := httpctx.ParseUUID(in.ClinicID)
	if err != nil {
		c.Error(apperr.Invalid("clinic_id is required"))
		return
	}
	if in.DurationMinutes <= 0 {
		in.DurationMinutes = 30
	}
	appt, err := h.svc.Reschedule(c.Request.Context(), userID, clinicID, apptID, service.RescheduleInput{
		StartTime:       in.StartTime,
		DurationMinutes: in.DurationMinutes,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toAppointmentResponse(appt))
}
