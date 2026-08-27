package auth

import (
	"net/http"
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

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/auth")
	{
		g.POST("/register", h.Register)
		g.POST("/login", h.Login)
		g.POST("/refresh", h.Refresh)
	}
}

func (h *Handler) RegisterProtectedRoutes(g *gin.RouterGroup) {
	g.GET("/me", h.Me)
}

// Register
//
// @Summary Register a new patient account
// @Description Register a new patient account. Public endpoint — registration only creates patient accounts.
// @Tags auth
// @Accept json
// @Produce json
// @Param input body RegisterInput true "Registration details"
// @Success 201 {object} userResponse
// @Router /auth/register [post]
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

// Login
//
// @Summary Login with phone and password
// @Description Authenticates a user by E.164 phone number and password, returning JWT access and refresh tokens.
// @Tags auth
// @Accept json
// @Produce json
// @Param input body LoginInput true "Login credentials"
// @Success 200 {object} TokenPair
// @Router /auth/login [post]
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

// Refresh
//
// @Summary Refresh access token
// @Description Exchanges a valid refresh token for a new access token pair.
// @Tags auth
// @Accept json
// @Produce json
// @Param input body object true "Refresh token" example:{"refresh_token":"<token>"}
// @Success 200 {object} TokenPair
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
	uid, _ := c.Get("user_id")
	if uid == nil {
		c.Error(apperr.Unauthorized("missing identity"))
		return
	}
	userID, ok := uid.(string)
	if !ok {
		c.Error(apperr.Unauthorized("invalid identity"))
		return
	}
	uidParsed, err := uuid.Parse(userID)
	if err != nil {
		c.Error(apperr.Unauthorized("invalid identity"))
		return
	}
	user, err := h.svc.Me(c.Request.Context(), uidParsed)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toUserResponse(user))
}

func toUserResponse(u *User) userResponse {
	return userResponse{
		ID:        u.ID.String(),
		Phone:     u.Phone,
		Name:      u.FullName,
		IsActive:  u.IsActive,
		CreatedAt: u.CreatedAt.Format(time.RFC3339),
	}
}

type userResponse struct {
	ID        string `json:"id"`
	Phone     string `json:"phone"`
	Name      string `json:"full_name"`
	IsActive  bool   `json:"is_active"`
	CreatedAt string `json:"created_at"`
}