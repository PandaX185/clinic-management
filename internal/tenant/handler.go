package tenant

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	auth "github.com/PandaX185/clinic-management/internal/auth"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

type tenantResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	IsActive bool   `json:"is_active"`
}

type tenantsListResponse struct {
	Items []tenantResponse `json:"items"`
}

type createTenantInput struct {
	Name string `json:"name" binding:"required,max=255"`
	Slug string `json:"slug" binding:"required,max=63"`
}

type bindStaffInput struct {
	UserID string `json:"user_id" binding:"required,uuid"`
	Role   string `json:"role"`
}

// RegisterRoutes mounts global (auth-only) tenant endpoints. These do NOT
// require X-Tenant-ID — browsing clinics is always allowed.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/tenants", h.List)
	rg.GET("/tenants/mine", h.ListMine)
}

// List returns all active clinics (patients browse everything).
//
// @Summary List clinics
// @Description Returns all active clinics. Patients may browse the full registry.
// @Tags tenants
// @Produce json
// @Security BearerAuth
// @Success 200 {object} tenantsListResponse
// @Failure 500 {object} apperr.ErrorResponse
// @Router /tenants [get]
func (h *Handler) List(c *gin.Context) {
	items, err := h.svc.List(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": toResponses(items)})
}

// ListMine returns the clinics relevant to the caller: staff bindings if
// they have any, otherwise every active clinic.
//
// @Summary List my clinics
// @Description Returns the clinics the current user has a profile in; falls back to all active clinics when the user has no staff bindings.
// @Tags tenants
// @Produce json
// @Security BearerAuth
// @Success 200 {object} tenantsListResponse
// @Failure 401 {object} apperr.ErrorResponse
// @Failure 500 {object} apperr.ErrorResponse
// @Router /tenants/mine [get]
func (h *Handler) ListMine(c *gin.Context) {
	userID, err := parseUserID(c)
	if err != nil {
		c.Error(err)
		return
	}
	items, err := h.svc.ListForUser(c.Request.Context(), userID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": toResponses(items)})
}

// Create provisions a new clinic (admin-only; mounted by main.go on an
// admin-guarded group).
//
// @Summary Create clinic
// @Description Provisions a new clinic and its tenant schema. Gated on the global super-admin flag (users.is_admin); no X-Tenant-ID is involved.
// @Tags tenants
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param input body createTenantInput true "Clinic details"
// @Success 201 {object} tenantResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 500 {object} apperr.ErrorResponse
// @Router /tenants [post]
func (h *Handler) Create(c *gin.Context) {
	var in createTenantInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	creatorID, err := parseUserID(c)
	if err != nil {
		c.Error(err)
		return
	}
	t, err := h.svc.Create(c.Request.Context(), creatorID, in.Name, in.Slug)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, toResponse(t))
}

// BindStaff attaches a user to a clinic as staff/doctor (admin-only).
//
// @Summary Bind staff to clinic
// @Description Associates a user with a clinic and assigns them a role. Requires the admin role in the clinic identified by X-Tenant-ID.
// @Tags tenants
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id (clinic to assign into)"
// @Param id path string true "Tenant id"
// @Param input body bindStaffInput true "Staff binding"
// @Success 200 {object} bindStaffResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 500 {object} apperr.ErrorResponse
// @Router /tenants/{id}/staff [post]
func (h *Handler) BindStaff(c *gin.Context) {
	var in bindStaffInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	tid, err := parseUUID(c.Param("id"))
	if err != nil {
		c.Error(err)
		return
	}
	uid, err := parseUUID(in.UserID)
	if err != nil {
		c.Error(err)
		return
	}
	if err := h.svc.BindStaff(c.Request.Context(), uid, tid, in.Role); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"bound": true})
}

type bindStaffResponse struct {
	Bound bool `json:"bound"`
}

func toResponse(t *Tenant) tenantResponse {
	return tenantResponse{
		ID:       t.ID.String(),
		Name:     t.Name,
		Slug:     t.Slug,
		IsActive: t.IsActive,
	}
}

func toResponses(items []Tenant) []tenantResponse {
	out := make([]tenantResponse, 0, len(items))
	for i := range items {
		out = append(out, toResponse(&items[i]))
	}
	return out
}

func parseUserID(c *gin.Context) (uuid.UUID, error) {
	v, ok := c.Get(auth.CtxUserID)
	if !ok {
		return uuid.Nil, apperr.Unauthorized("unauthenticated")
	}
	return parseUUID(v.(string))
}

func parseUUID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, apperr.Invalid("invalid id format")
	}
	return id, nil
}
