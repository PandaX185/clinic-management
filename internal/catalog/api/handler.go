package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PandaX185/clinic-management/internal/catalog/service"
	idapi "github.com/PandaX185/clinic-management/internal/identity/api"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/httpctx"
)

type Handler struct {
	svc *service.Service
}

func NewHandler(svc *service.Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts tenant-scoped directory endpoints (X-Tenant-ID
// required; resolved per clinic). Profile creation and type mutations are
// admin-only; directory reads are allowed for any member of the clinic.
func (h *Handler) RegisterRoutes(protected *gin.RouterGroup) {
	g := protected.Group("/profiles")
	{
		g.GET("", h.ListProfiles)
		g.POST("", idapi.RequireRoles("admin"), h.CreateProfile)
	}
	protected.GET("/doctors", h.ListDoctors)

	dg := protected.Group("/doctors/:id")
	{
		dg.GET("/schedule", h.GetSchedule)
		dg.PUT("/schedule", idapi.RequireRoles("admin"), h.SetSchedule)
		dg.POST("/schedule/exceptions", idapi.RequireRoles("admin"), h.AddException)
		dg.DELETE("/schedule/exceptions/:exception_id", idapi.RequireRoles("admin"), h.RemoveException)
	}

	tg := protected.Group("/appointment-types")
	{
		tg.GET("", h.ListTypes)
		tg.POST("", idapi.RequireRoles("admin"), h.CreateType)
		tg.PUT("/:id", idapi.RequireRoles("admin"), h.UpdateType)
	}
}

// @Summary List clinic profiles
// @Description Returns every person registered in the active clinic (X-Tenant-ID), with their roles.
// @Tags catalog
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Success 200 {object} profilesListResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 500 {object} apperr.ErrorResponse
// @Router /profiles [get]
func (h *Handler) ListProfiles(c *gin.Context) {
	items, err := h.svc.ListProfiles(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, profilesListResponse{Items: toProfileResponses(items)})
}

// @Summary Register a profile
// @Description Registers an existing user as a member of the active clinic and assigns them a role. Admin only.
// @Tags catalog
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param input body createProfileInput true "Profile details"
// @Success 201 {object} profileResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Failure 500 {object} apperr.ErrorResponse
// @Router /profiles [post]
func (h *Handler) CreateProfile(c *gin.Context) {
	var in createProfileInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	uid, err := httpctx.ParseUUID(in.UserID)
	if err != nil {
		c.Error(apperr.Invalid("invalid user_id"))
		return
	}
	p, err := h.svc.CreateProfile(c.Request.Context(), uid, in.DisplayName, in.Role)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, toProfileResponse(p))
}

// @Summary List doctors
// @Description Returns all profiles holding the doctor role in the active clinic.
// @Tags catalog
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Success 200 {object} profilesListResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 500 {object} apperr.ErrorResponse
// @Router /doctors [get]
func (h *Handler) ListDoctors(c *gin.Context) {
	items, err := h.svc.ListDoctors(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, profilesListResponse{Items: toProfileResponses(items)})
}

// @Summary List appointment types
// @Description Returns the bookable services defined in the active clinic.
// @Tags catalog
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Success 200 {object} typesListResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Router /appointment-types [get]
func (h *Handler) ListTypes(c *gin.Context) {
	items, err := h.svc.ListAppointmentTypes(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, typesListResponse{Items: toTypeResponses(items)})
}

// @Summary Create appointment type
// @Description Defines a new bookable service in the active clinic. Admin only.
// @Tags catalog
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param input body typeInput true "Appointment type"
// @Success 201 {object} typeResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Router /appointment-types [post]
func (h *Handler) CreateType(c *gin.Context) {
	var in typeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	t, err := h.svc.CreateAppointmentType(c.Request.Context(), service.AppointmentType{
		Name:            in.Name,
		DurationMinutes: in.DurationMinutes,
		Price:           in.Price,
		Color:           in.Color,
		Icon:            in.Icon,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, toTypeResponse(t))
}

// @Summary Update appointment type
// @Description Updates an existing bookable service in the active clinic. Admin only.
// @Tags catalog
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param id path string true "Appointment type id"
// @Param input body typeInput true "Appointment type"
// @Success 200 {object} typeResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /appointment-types/{id} [put]
func (h *Handler) UpdateType(c *gin.Context) {
	id, err := httpctx.ParseUUIDParam(c, "id")
	if err != nil {
		c.Error(apperr.Invalid("invalid id"))
		return
	}
	var in typeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	t, err := h.svc.UpdateAppointmentType(c.Request.Context(), id, service.AppointmentType{
		Name:            in.Name,
		DurationMinutes: in.DurationMinutes,
		Price:           in.Price,
		Color:           in.Color,
		Icon:            in.Icon,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toTypeResponse(t))
}

// @Summary Get doctor schedule
// @Description Returns the doctor's recurring weekly windows and one-off date exceptions in the active clinic.
// @Tags catalog
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param id path string true "Doctor profile id"
// @Success 200 {object} scheduleResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 500 {object} apperr.ErrorResponse
// @Router /doctors/{id}/schedule [get]
func (h *Handler) GetSchedule(c *gin.Context) {
	doctorID, err := httpctx.ParseUUIDParam(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	s, err := h.svc.GetDoctorSchedule(c.Request.Context(), doctorID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toScheduleResponse(doctorID, s))
}

// @Summary Replace weekly schedule
// @Description Replaces the doctor's recurring weekly windows in the active clinic. Admin only.
// @Tags catalog
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param id path string true "Doctor profile id"
// @Param input body scheduleInput true "Weekly windows (times as HH:MM)"
// @Success 204 "Schedule updated"
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /doctors/{id}/schedule [put]
func (h *Handler) SetSchedule(c *gin.Context) {
	doctorID, err := httpctx.ParseUUIDParam(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	var in scheduleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	hours, err := weeklyHours(in.Weekly)
	if err != nil {
		c.Error(err)
		return
	}
	if err := h.svc.SetWeeklySchedule(c.Request.Context(), doctorID, hours); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

// @Summary Add schedule exception
// @Description Registers a one-off date override for the doctor: leave, holiday, unavailable (blocked hours) or extra_hours (adds a window). Admin only.
// @Tags catalog
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param id path string true "Doctor profile id"
// @Param input body exceptionInput true "Date override"
// @Success 201 {object} scheduleExceptionResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /doctors/{id}/schedule/exceptions [post]
func (h *Handler) AddException(c *gin.Context) {
	doctorID, err := httpctx.ParseUUIDParam(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	var in exceptionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	date, err := time.Parse("2006-01-02", in.Date)
	if err != nil {
		c.Error(apperr.Invalid("invalid date"))
		return
	}
	ex, err := h.svc.CreateScheduleException(c.Request.Context(), doctorID, service.ScheduleException{
		Date:     date,
		Type:     in.Type,
		StartMin: minutesOf(in.StartTime),
		EndMin:   minutesOf(in.EndTime),
		Reason:   in.Reason,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, scheduleExceptionResponse{
		ID:        ex.ID.String(),
		Date:      ex.Date.Format("2006-01-02"),
		Type:      ex.Type,
		StartTime: hhmm(ex.StartMin),
		EndTime:   hhmm(ex.EndMin),
		Reason:    ex.Reason,
	})
}

// @Summary Remove schedule exception
// @Description Deletes a one-off date override. Admin only.
// @Tags catalog
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id"
// @Param id path string true "Doctor profile id"
// @Param exception_id path string true "Schedule exception id"
// @Success 204 "Exception removed"
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 500 {object} apperr.ErrorResponse
// @Router /doctors/{id}/schedule/exceptions/{exception_id} [delete]
func (h *Handler) RemoveException(c *gin.Context) {
	exceptionID, err := httpctx.ParseUUIDParam(c, "exception_id")
	if err != nil {
		c.Error(err)
		return
	}
	if err := h.svc.DeleteScheduleException(c.Request.Context(), exceptionID); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}
