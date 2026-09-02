package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/PandaX185/clinic-management/internal/auth/service"
	"github.com/PandaX185/clinic-management/internal/platform/apperr"
	"github.com/PandaX185/clinic-management/internal/platform/httpctx"
)

type Handler struct {
	svc *service.Service
}

func NewHandler(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/auth")
	{
		g.POST("/register", h.Register)
		g.POST("/login", h.Login)
		g.POST("/refresh", h.Refresh)
		g.POST("/logout", h.Logout)
	}
}

func (h *Handler) RegisterProtectedRoutes(g *gin.RouterGroup) {
	g.GET("/me", h.Me)
	g.GET("/tenants", h.ListTenants)
}

// Register
//
// @Summary Register a new patient account
// @Description Register a new patient account. Public endpoint — registration only creates patient accounts.
// @Tags auth
// @Accept json
// @Produce json
// @Param input body service.RegisterInput true "Registration details"
// @Success 201 {object} userResponse
// @Router /auth/register [post]
func (h *Handler) Register(c *gin.Context) {
	var in service.RegisterInput
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

// Login
//
// @Summary Login with phone and password
// @Description Authenticates a user by E.164 phone number and password, returning JWT access and refresh tokens.
// @Tags auth
// @Accept json
// @Produce json
// @Param input body service.LoginInput true "Login credentials"
// @Success 200 {object} service.TokenPair
// @Router /auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var in service.LoginInput
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

// Logout revokes the given refresh token.
//
// @Summary Logout
// @Description Revokes the given refresh token. The user must provide a valid refresh token.
// @Tags auth
// @Accept json
// @Produce json
// @Param input body object true "Refresh token" example:{"refresh_token":"<token>"}
// @Success 204 "No Content"
// @Router /auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.Invalid("refresh_token is required"))
		return
	}
	if err := h.svc.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListTenants returns all tenants that the current user has a profile in.
//
// @Summary List user's tenants
// @Description Returns all tenants associated with the current user, with their role in each.
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {array} service.UserTenant
// @Router /auth/tenants [get]
func (h *Handler) ListTenants(c *gin.Context) {
	userID, err := httpctx.UserID(c)
	if err != nil {
		c.Error(apperr.Unauthorized("missing identity"))
		return
	}
	tenants, err := h.svc.ListTenants(c.Request.Context(), userID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, tenants)
}

// Refresh
//
// @Summary Refresh access token
// @Description Exchanges a valid refresh token for a new access token pair.
// @Tags auth
// @Accept json
// @Produce json
// @Param input body object true "Refresh token" example:{"refresh_token":"<token>"}
// @Success 200 {object} service.TokenPair
// @Router /auth/refresh [post]
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

// Me
//
// @Summary Get current user
// @Description Returns the authenticated user's profile information.
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} userResponse
// @Router /auth/me [get]
func (h *Handler) Me(c *gin.Context) {
	uid, err := httpctx.UserID(c)
	if err != nil {
		c.Error(err)
		return
	}
	user, err := h.svc.Me(c.Request.Context(), uid)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toUserResponse(user))
}
