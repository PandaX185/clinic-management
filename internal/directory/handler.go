package directory

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"

	auth "github.com/PandaX185/clinic-management/internal/auth"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

type profileResponse struct {
	ID          string   `json:"id"`
	UserID      string   `json:"user_id"`
	DisplayName string   `json:"display_name"`
	Status      string   `json:"status"`
	Roles       []string `json:"roles"`
	CreatedAt   string   `json:"created_at"`
}

type createProfileInput struct {
	UserID      string `json:"user_id" binding:"required,uuid"`
	DisplayName string `json:"display_name" binding:"required,max=255"`
	Role        string `json:"role"`
}

type profilesListResponse struct {
	Items []profileResponse `json:"items"`
}

type typeResponse struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	DurationMinutes int32  `json:"duration_minutes"`
	Price           string `json:"price"`
	Color           string `json:"color,omitempty"`
	Icon            string `json:"icon,omitempty"`
	CreatedAt       string `json:"created_at"`
}

type typesListResponse struct {
	Items []typeResponse `json:"items"`
}

type typeInput struct {
	Name            string `json:"name" binding:"required,max=100"`
	DurationMinutes int32  `json:"duration_minutes" binding:"required"`
	Price           string `json:"price"`
	Color           string `json:"color"`
	Icon            string `json:"icon"`
}

// RegisterRoutes mounts tenant-scoped directory endpoints (X-Tenant-ID
// required; resolved per clinic). Profile creation and type mutations are
// admin-only; directory reads are allowed for any member of the clinic.
func (h *Handler) RegisterRoutes(protected *gin.RouterGroup) {
	g := protected.Group("/profiles")
	{
		g.GET("", h.ListProfiles)
		g.POST("", auth.RequireRoles("admin"), h.CreateProfile)
	}
	protected.GET("/doctors", h.ListDoctors)

	tg := protected.Group("/appointment-types")
	{
		tg.GET("", h.ListTypes)
		tg.POST("", auth.RequireRoles("admin"), h.CreateType)
		tg.PUT("/:id", auth.RequireRoles("admin"), h.UpdateType)
	}
}

// @Summary List clinic profiles
// @Description Returns every person registered in the active clinic (X-Tenant-ID), with their roles.
// @Tags directory
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
	c.JSON(http.StatusOK, gin.H{"items": toProfileResponses(items)})
}

// @Summary Register a profile
// @Description Registers an existing user as a member of the active clinic and assigns them a role. Admin only.
// @Tags directory
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
	uid, err := uuid.Parse(in.UserID)
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
// @Tags directory
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
	c.JSON(http.StatusOK, gin.H{"items": toProfileResponses(items)})
}

// @Summary List appointment types
// @Description Returns the bookable services defined in the active clinic.
// @Tags directory
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
	c.JSON(http.StatusOK, gin.H{"items": toTypeResponses(items)})
}

// @Summary Create appointment type
// @Description Defines a new bookable service in the active clinic. Admin only.
// @Tags directory
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
	t, err := h.svc.CreateAppointmentType(c.Request.Context(), AppointmentType{
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
// @Tags directory
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
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Error(apperr.Invalid("invalid id"))
		return
	}
	var in typeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	t, err := h.svc.UpdateAppointmentType(c.Request.Context(), id, AppointmentType{
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

func toProfileResponse(p *Profile) profileResponse {
	roles := p.Roles
	if roles == nil {
		roles = []string{}
	}
	return profileResponse{
		ID:          p.ID.String(),
		UserID:      p.UserID.String(),
		DisplayName: p.DisplayName,
		Status:      p.Status,
		Roles:       roles,
		CreatedAt:   p.CreatedAt,
	}
}

func toProfileResponses(items []Profile) []profileResponse {
	out := make([]profileResponse, 0, len(items))
	for i := range items {
		out = append(out, toProfileResponse(&items[i]))
	}
	return out
}

func toTypeResponse(t *AppointmentType) typeResponse {
	return typeResponse{
		ID:              t.ID.String(),
		Name:            t.Name,
		DurationMinutes: t.DurationMinutes,
		Price:           t.Price,
		Color:           t.Color,
		Icon:            t.Icon,
		CreatedAt:       t.CreatedAt,
	}
}

func toTypeResponses(items []AppointmentType) []typeResponse {
	out := make([]typeResponse, 0, len(items))
	for i := range items {
		out = append(out, toTypeResponse(&items[i]))
	}
	return out
}
