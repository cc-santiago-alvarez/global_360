package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"global_360/internal/application/dto"
	"global_360/internal/domain/access"
	"global_360/internal/domain/audit"
)

type createRoleRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description *string  `json:"description"`
	Scope       string   `json:"scope" binding:"required"`
	Permissions []string `json:"permissions"`
}

type updateRoleRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Active      *bool   `json:"active"`
}

type rolePermissionsRequest struct {
	Permissions []string `json:"permissions" binding:"required"`
}

func (h *Handlers) ListPermissions(c *gin.Context) {
	out, err := h.Roles.ListPermissions(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *Handlers) ListRoles(c *gin.Context) {
	out, err := h.Roles.List(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *Handlers) GetRole(c *gin.Context) {
	out, err := h.Roles.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) CreateRole(c *gin.Context) {
	var req createRoleRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.Roles.Create(c.Request.Context(), dto.CreateRole{
		Name: req.Name, Description: req.Description, Scope: access.Scope(req.Scope), Permissions: req.Permissions,
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (h *Handlers) UpdateRole(c *gin.Context) {
	var req updateRoleRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.Roles.Update(c.Request.Context(), c.Param("id"), dto.UpdateRole(req))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) ReplaceRolePermissions(c *gin.Context) {
	var req rolePermissionsRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.Roles.ReplacePermissions(c.Request.Context(), c.Param("id"), req.Permissions)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) ListAuditLogs(c *gin.Context) {
	page, err := pagination(c)
	if err != nil {
		fail(c, err)
		return
	}
	from, err := queryTime(c, "from")
	if err != nil {
		fail(c, err)
		return
	}
	to, err := queryTime(c, "to")
	if err != nil {
		fail(c, err)
		return
	}
	q := dto.ListAuditLogs{
		Entity: queryPtr(c, "entity"), EntityID: queryPtr(c, "entity_id"), UserID: queryPtr(c, "user_id"),
		From: from, To: to, Page: page,
	}
	if v := queryPtr(c, "action"); v != nil {
		a := audit.Action(*v)
		q.Action = &a
	}
	out, err := h.Audit.List(c.Request.Context(), q)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
