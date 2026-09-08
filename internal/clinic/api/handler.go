package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/PandaX185/lahza/internal/clinic/service"
	"github.com/PandaX185/lahza/internal/platform/apperr"
	"github.com/PandaX185/lahza/internal/platform/httpctx"
)

type Handler struct {
	svc *service.Service
}

func NewHandler(svc *service.Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts global (auth-only) clinic endpoints. These do NOT
// require X-Tenant-ID — browsing clinics is always allowed.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/clinics", h.List)
	rg.GET("/clinics/mine", h.ListMine)
}

// List returns all active clinics (patients browse everything).
//
// @Summary List clinics
// @Description Returns all active clinics. Patients may browse the full registry.
// @Tags clinics
// @Produce json
// @Security BearerAuth
// @Success 200 {object} clinicsListResponse
// @Failure 500 {object} apperr.ErrorResponse
// @Router /clinics [get]
func (h *Handler) List(c *gin.Context) {
	items, err := h.svc.List(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, clinicsListResponse{Items: toResponses(items)})
}

// ListMine returns the clinics the caller is a member of.
//
// @Summary List my clinics
// @Description Returns the clinics the current user has a membership in; an empty list when the user has no clinics.
// @Tags clinics
// @Produce json
// @Security BearerAuth
// @Success 200 {object} clinicsListResponse
// @Failure 401 {object} apperr.ErrorResponse
// @Failure 500 {object} apperr.ErrorResponse
// @Router /clinics/mine [get]
func (h *Handler) ListMine(c *gin.Context) {
	userID, err := httpctx.UserID(c)
	if err != nil {
		c.Error(err)
		return
	}
	items, err := h.svc.ListForUser(c.Request.Context(), userID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, clinicsListResponse{Items: toResponses(items)})
}

// Create provisions a new clinic (admin-only; mounted by main.go on an
// admin-guarded group).
//
// @Summary Create clinic
// @Description Provisions a new clinic and its tenant schema. Gated on the global super-admin flag (users.is_admin); no X-Tenant-ID is involved.
// @Tags clinics
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param input body createClinicInput true "Clinic details"
// @Success 201 {object} clinicResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 500 {object} apperr.ErrorResponse
// @Router /clinics [post]
func (h *Handler) Create(c *gin.Context) {
	var in createClinicInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	creatorID, err := httpctx.UserID(c)
	if err != nil {
		c.Error(err)
		return
	}
	c2, err := h.svc.Create(c.Request.Context(), creatorID, in.Name, in.Slug)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, toResponse(c2))
}

// BindStaff attaches a user to a clinic as staff/doctor (admin-only).
//
// @Summary Bind staff to clinic
// @Description Associates a user with a clinic and assigns them a role. Requires the admin role in the clinic identified by X-Tenant-ID.
// @Tags clinics
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant id (clinic to assign into)"
// @Param id path string true "Clinic id"
// @Param input body bindStaffInput true "Staff binding"
// @Success 200 {object} bindStaffResponse
// @Failure 400 {object} apperr.ErrorResponse
// @Failure 403 {object} apperr.ErrorResponse
// @Failure 500 {object} apperr.ErrorResponse
// @Router /clinics/{id}/staff [post]
func (h *Handler) BindStaff(c *gin.Context) {
	var in bindStaffInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	clinicID, err := httpctx.ParseUUIDParam(c, "id")
	if err != nil {
		c.Error(err)
		return
	}
	uid, err := httpctx.ParseUUID(in.UserID)
	if err != nil {
		c.Error(err)
		return
	}
	if err := h.svc.BindStaff(c.Request.Context(), uid, clinicID, in.Role); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, bindStaffResponse{Bound: true})
}
