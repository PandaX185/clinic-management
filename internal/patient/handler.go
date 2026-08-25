package patient

import (
	"net/http"

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

type response struct {
	ID                    string  `json:"id"`
	FullName              string  `json:"full_name"`
	DateOfBirth           *string `json:"date_of_birth,omitempty"`
	Gender                *string `json:"gender,omitempty"`
	Phone                 *string `json:"phone,omitempty"`
	Address               *string `json:"address,omitempty"`
	EmergencyContactName  *string `json:"emergency_contact_name,omitempty"`
	EmergencyContactPhone *string `json:"emergency_contact_phone,omitempty"`
	MedicalNotes          *string `json:"medical_notes,omitempty"`
	CreatedAt             string  `json:"created_at"`
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, guard gin.HandlerFunc) {
	g := rg.Group("/patients")
	g.Use(guard)
	{
		g.POST("", h.Create)
		g.GET("", h.List)
		g.GET("/:id", h.Get)
		g.PATCH("/:id", h.Update)
		g.DELETE("/:id", h.Delete)
	}
}

func (h *Handler) Create(c *gin.Context) {
	var in CreateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	p, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, toResponse(p))
}

func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.Error(err)
		return
	}
	p, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toResponse(p))
}

func (h *Handler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.Error(err)
		return
	}
	var in UpdateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.Error(apperr.Invalid("invalid request body"))
		return
	}
	p, err := h.svc.Update(c.Request.Context(), id, in)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toResponse(p))
}

func (h *Handler) List(c *gin.Context) {
	var q ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.Error(apperr.Invalid("invalid query parameters"))
		return
	}
	patients, total, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]response, 0, len(patients))
	for i := range patients {
		out = append(out, *toResponse(&patients[i]))
	}
	c.JSON(http.StatusOK, gin.H{"items": out, "total": total, "limit": q.Limit, "offset": q.Offset})
}

func (h *Handler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.Error(err)
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

func parseID(c *gin.Context) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return uuid.Nil, apperr.Invalid("invalid id format")
	}
	return id, nil
}

func toResponse(p *Patient) *response {
	return &response{
		ID:                    p.ID.String(),
		FullName:              p.FullName,
		DateOfBirth:           formatTime(p.DateOfBirth),
		Gender:                p.Gender,
		Phone:                 p.Phone,
		Address:               p.Address,
		EmergencyContactName:  p.EmergencyContactName,
		EmergencyContactPhone: p.EmergencyContactPhone,
		MedicalNotes:          p.MedicalNotes,
		CreatedAt:             p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
