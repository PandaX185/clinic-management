package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

const (
	CtxUserID = "auth_user_id"
	CtxRoles  = "auth_roles"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/auth")
	{
		g.POST("/register", h.Register)
		g.POST("/login", h.Login)
		g.POST("/refresh", h.Refresh)
	}
}

func (h *Handler) RegisterProtectedRoutes(g *gin.RouterGroup) {
	g.GET("/auth/me", h.Me)
}

func (h *Handler) Register(c *gin.Context) {
	var in RegisterInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	user, err := h.svc.Register(c.Request.Context(), in)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, toUserResponse(user))
}

func (h *Handler) Login(c *gin.Context) {
	var in LoginInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	pair, err := h.svc.Login(c.Request.Context(), in)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, pair)
}

func (h *Handler) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.Invalid("refresh_token is required"))
		return
	}
	pair, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, pair)
}

func (h *Handler) Me(c *gin.Context) {
	id, _ := c.Get(CtxUserID)
	userID, ok := id.(string)
	if !ok {
		c.Error(apperr.Unauthorized("missing identity"))
		return
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		c.Error(apperr.Unauthorized("invalid identity"))
		return
	}
	user, err := h.svc.Me(c.Request.Context(), uid)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toUserResponse(user))
}

type userResponse struct {
	ID    string   `json:"id"`
	Email string   `json:"email"`
	Name  string   `json:"full_name"`
	Phone *string  `json:"phone,omitempty"`
	Roles []string `json:"roles"`
}

func toUserResponse(u *User) userResponse {
	roles := make([]string, len(u.Roles))
	for i, r := range u.Roles {
		roles[i] = string(r)
	}
	return userResponse{
		ID:    u.ID.String(),
		Email: u.Email,
		Name:  u.FullName,
		Phone: u.Phone,
		Roles: roles,
	}
}
