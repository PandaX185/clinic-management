package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PandaX185/clinic-management/internal/booking/service"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/httpctx"
)

// Handler serves the public clinic discovery endpoints. These routes carry
// no authentication or tenant context: discovery is part of the marketing
// surface a patient reaches before logging in.
type Handler struct {
	svc *service.Service
}

func NewHandler(svc *service.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/clinics")
	{
		g.GET("", h.ListClinics)
		g.GET("/:id", h.GetClinic)
		g.GET("/:id/doctors", h.ListDoctors)
		g.GET("/:id/slots", h.GetAvailableSlots)
	}
	rg.GET("/doctors/:id", h.GetDoctor)
}

// @Summary List clinics
// @Description Returns active clinics for public discovery, paginated.
// @Tags booking
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param size query int false "Page size" default(20) minimum(1) maximum(100)
// @Success 200 {object} clinicsListResponse
// @Failure 500 {object} apperr.ErrorResponse
// @Router /booking/clinics [get]
func (h *Handler) ListClinics(c *gin.Context) {
	page, size, err := pageParams(c)
	if err != nil {
		c.Error(err)
		return
	}
	items, total, err := h.svc.ListClinics(c.Request.Context(), page, size)
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]clinicListItem, 0, len(items))
	for _, item := range items {
		out = append(out, toClinicListItem(item))
	}
	c.JSON(http.StatusOK, clinicsListResponse{
		Items:      out,
		Pagination: pagination{Page: page, Size: size, Total: total},
	})
}

// @Summary Get clinic
// @Description Returns a clinic's public profile with its services and doctors.
// @Tags booking
// @Produce json
// @Param id path string true "Clinic id"
// @Success 200 {object} clinicDetailResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /booking/clinics/{id} [get]
func (h *Handler) GetClinic(c *gin.Context) {
	id, err := httpctx.ParseUUIDParam(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	clinic, err := h.svc.GetClinicDetail(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toClinicDetail(*clinic))
}

// @Summary List clinic doctors
// @Description Returns the doctors practising at a clinic, paginated.
// @Tags booking
// @Produce json
// @Param id path string true "Clinic id"
// @Param page query int false "Page number" default(1)
// @Param size query int false "Page size" default(20) minimum(1) maximum(100)
// @Success 200 {object} doctorsListResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /booking/clinics/{id}/doctors [get]
func (h *Handler) ListDoctors(c *gin.Context) {
	clinicID, err := httpctx.ParseUUIDParam(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	page, size, err := pageParams(c)
	if err != nil {
		c.Error(err)
		return
	}
	items, total, err := h.svc.ListDoctors(c.Request.Context(), clinicID, page, size)
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]doctorResponse, 0, len(items))
	for _, item := range items {
		out = append(out, toDoctorResponse(item))
	}
	c.JSON(http.StatusOK, doctorsListResponse{
		Items:      out,
		Pagination: pagination{Page: page, Size: size, Total: total},
	})
}

// @Summary Get doctor
// @Description Returns a doctor regardless of which clinic they practise at.
// @Tags booking
// @Produce json
// @Param id path string true "Doctor profile id"
// @Success 200 {object} doctorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /booking/doctors/{id} [get]
func (h *Handler) GetDoctor(c *gin.Context) {
	id, err := httpctx.ParseUUIDParam(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	doctor, err := h.svc.GetDoctor(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toDoctorResponse(*doctor))
}

// @Summary Get available slots
// @Description Returns open booking windows for a doctor at a clinic on a date.
// @Tags booking
// @Produce json
// @Param id path string true "Clinic id"
// @Param doctor_id query string true "Doctor profile id"
// @Param date query string true "Date in YYYY-MM-DD"
// @Param appointment_type_id query string false "Appointment type id (uses clinic default when absent)"
// @Success 200 {object} slotsResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 404 {object} apperr.ErrorResponse
// @Router /booking/clinics/{id}/slots [get]
func (h *Handler) GetAvailableSlots(c *gin.Context) {
	clinicID, err := httpctx.ParseUUIDParam(c, "id")
	if err != nil {
		c.Error(err)
		return
	}

	doctorID, err := httpctx.ParseUUID(c.Query("doctor_id"))
	if err != nil {
		c.Error(apperr.Invalid("doctor_id is required"))
		return
	}

	date, err := time.Parse("2006-01-02", c.Query("date"))
	if err != nil {
		c.Error(apperr.Invalid("date must be in YYYY-MM-DD format"))
		return
	}

	q := service.SlotQuery{DoctorID: doctorID, Date: date}
	if raw := c.Query("appointment_type_id"); raw != "" {
		if q.AppointmentTypeID, err = httpctx.ParseUUID(raw); err != nil {
			c.Error(apperr.Invalid("invalid appointment_type_id"))
			return
		}
	}

	slots, err := h.svc.GetAvailableSlots(c.Request.Context(), clinicID, q)
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]slotResponse, 0, len(slots))
	for _, slot := range slots {
		out = append(out, toSlotResponse(slot))
	}
	c.JSON(http.StatusOK, slotsResponse{Items: out})
}

func pageParams(c *gin.Context) (int, int, error) {
	var pg struct {
		Page int `form:"page"`
		Size int `form:"size"`
	}
	if err := c.ShouldBindQuery(&pg); err != nil {
		return 0, 0, apperr.Invalid("invalid query parameters")
	}
	if pg.Page < 1 {
		pg.Page = 1
	}
	if pg.Size < 1 || pg.Size > 100 {
		pg.Size = 20
	}
	return pg.Page, pg.Size, nil
}
