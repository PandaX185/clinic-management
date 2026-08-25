package doctor

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

type doctorResponse struct {
	ID             string  `json:"id"`
	Specialization string  `json:"specialization"`
	LicenseNumber  string  `json:"license_number"`
	Bio            *string `json:"bio,omitempty"`
	FullName       string  `json:"full_name"`
	Email          string  `json:"email"`
	IsActive       bool    `json:"is_active"`
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, staffGate gin.HandlerFunc) {
	g := rg.Group("/doctors")
	{
		g.GET("", h.List)
		g.GET("/:id", h.Get)
		g.GET("/:id/availability", h.Availability)
	}

	authed := rg.Group("/doctors")
	authed.Use(staffGate)
	{
		authed.POST("", h.Create)
		authed.POST("/:id/schedules", h.AddSchedule)
		authed.GET("/:id/schedules", h.GetSchedules)
		authed.DELETE("/schedules/:scheduleId", h.RemoveSchedule)
		authed.POST("/:id/exceptions", h.AddException)
	}
}

func (h *Handler) Create(c *gin.Context) {
	var in struct {
		Email          string  `json:"email" binding:"required,email"`
		Password       string  `json:"password" binding:"required,min=8,max=72"`
		FullName       string  `json:"full_name" binding:"required,max=255"`
		Specialization string  `json:"specialization" binding:"required,max=255"`
		LicenseNumber  string  `json:"license_number" binding:"required,max=100"`
		Bio            *string `json:"bio"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	d, err := h.svc.CreateDoctor(c.Request.Context(), in.Email, in.Password, in.FullName, in.Specialization, in.LicenseNumber, in.Bio)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, toDoctorResponse(d))
}

func (h *Handler) Get(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	d, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toDoctorResponse(d))
}

func (h *Handler) List(c *gin.Context) {
	activeOnly := c.Query("active") == "true"
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	doctors, err := h.svc.List(c.Request.Context(), activeOnly, c.Query("specialization"), limit, offset)
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]doctorResponse, 0, len(doctors))
	for i := range doctors {
		out = append(out, *toDoctorResponse(&doctors[i]))
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *Handler) AddSchedule(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	var in AddScheduleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	sch, err := h.svc.AddSchedule(c.Request.Context(), id, in)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, sch)
}

func (h *Handler) GetSchedules(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	schedules, err := h.svc.GetSchedules(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": schedules})
}

func (h *Handler) RemoveSchedule(c *gin.Context) {
	scheduleID, err := parseUUID(c, "scheduleId")
	if err != nil {
		c.Error(err)
		return
	}
	if err := h.svc.RemoveSchedule(c.Request.Context(), scheduleID); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) AddException(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	var in struct {
		Date          string     `json:"date" binding:"required"`
		IsUnavailable bool       `json:"is_unavailable"`
		StartTime     *time.Time `json:"start_time"`
		EndTime       *time.Time `json:"end_time"`
		Reason        string     `json:"reason"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	date, perr := time.Parse("2006-01-02", in.Date)
	if perr != nil {
		c.Error(apperr.Invalid("date must be YYYY-MM-DD"))
		return
	}
	if !in.IsUnavailable && (in.StartTime == nil || in.EndTime == nil) {
		c.Error(apperr.Invalid("custom hours require start_time and end_time"))
		return
	}
	if err := h.svc.AddException(c.Request.Context(), id, date, in.IsUnavailable, in.StartTime, in.EndTime, in.Reason); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusCreated)
}

func (h *Handler) Availability(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	fromStr := c.DefaultQuery("from", time.Now().Format("2006-01-02"))
	toStr := c.DefaultQuery("to", time.Now().AddDate(0, 0, 7).Format("2006-01-02"))

	from, ferr := time.ParseInLocation("2006-01-02", fromStr, time.Local)
	to, terr := time.ParseInLocation("2006-01-02", toStr, time.Local)
	if ferr != nil || terr != nil {
		c.Error(apperr.Invalid("from/to must be YYYY-MM-DD"))
		return
	}

	slots, err := h.svc.Availability(c.Request.Context(), id, from, to, time.Local)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, AvailabilityResponse{
		DoctorID: id.String(),
		Date:     from.Format("2006-01-02"),
		Slots:    slots,
	})
}

func parseUUID(c *gin.Context, param string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Param(param))
	if err != nil {
		return uuid.Nil, apperr.Invalid("invalid id format")
	}
	return id, nil
}

func toDoctorResponse(d *Doctor) *doctorResponse {
	return &doctorResponse{
		ID:             d.ID.String(),
		Specialization: d.Specialization,
		LicenseNumber:  d.LicenseNumber,
		Bio:            d.Bio,
		FullName:       d.FullName,
		Email:          d.Email,
		IsActive:       d.IsActive,
	}
}
