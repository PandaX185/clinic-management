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

// RegisterRoutes mounts global (auth-only) tenant endpoints. These do NOT
// require X-Tenant-ID — browsing clinics is always allowed.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/tenants", h.List)
	rg.GET("/tenants/mine", h.ListMine)
}

// List returns all active clinics (patients browse everything).
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
func (h *Handler) Create(c *gin.Context) {
	var in struct {
		Name string `json:"name" binding:"required,max=255"`
		Slug string `json:"slug" binding:"required,max=63"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	t, err := h.svc.Create(c.Request.Context(), in.Name, in.Slug)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, toResponse(t))
}

// BindStaff attaches a user to a clinic as staff/doctor (admin-only).
func (h *Handler) BindStaff(c *gin.Context) {
	var in struct {
		UserID string `json:"user_id" binding:"required,uuid"`
	}
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
	if err := h.svc.BindStaff(c.Request.Context(), uid, tid); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"bound": true})
}

func toResponse(t *Tenant) gin.H {
	return gin.H{"id": t.ID, "name": t.Name, "slug": t.Slug, "is_active": t.IsActive}
}

func toResponses(items []Tenant) []gin.H {
	out := make([]gin.H, 0, len(items))
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
