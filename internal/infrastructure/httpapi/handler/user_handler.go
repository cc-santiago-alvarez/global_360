package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"global_360/internal/application/dto"
	"global_360/internal/domain/identity"
)

type personRequest struct {
	DocumentType      string  `json:"document_type" binding:"required"`
	DocumentNumber    string  `json:"document_number" binding:"required"`
	DocumentCountryID string  `json:"document_country_id" binding:"required"`
	FirstNames        string  `json:"first_names" binding:"required"`
	LastNames         string  `json:"last_names" binding:"required"`
	Email             string  `json:"email" binding:"required"`
	Phone             *string `json:"phone"`
}

type createUserRequest struct {
	Email     string        `json:"email" binding:"required"`
	Password  string        `json:"password" binding:"required"`
	CompanyID *string       `json:"company_id"`
	Person    personRequest `json:"person" binding:"required"`
}

type updateUserRequest struct {
	Email       *string `json:"email"`
	FirstNames  *string `json:"first_names"`
	LastNames   *string `json:"last_names"`
	PersonEmail *string `json:"person_email"`
	Phone       *string `json:"phone"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password" binding:"required"`
}

type assignRoleRequest struct {
	RoleID    string  `json:"role_id" binding:"required"`
	CompanyID *string `json:"company_id"`
}

func (h *Handlers) CreateUser(c *gin.Context) {
	var req createUserRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.Users.Create(c.Request.Context(), dto.CreateUser{
		Email: req.Email, Password: req.Password, CompanyID: emptyToNil(req.CompanyID),
		Person: dto.PersonInput{
			DocumentType: req.Person.DocumentType, DocumentNumber: req.Person.DocumentNumber,
			DocumentCountryID: req.Person.DocumentCountryID, FirstNames: req.Person.FirstNames,
			LastNames: req.Person.LastNames, Email: req.Person.Email, Phone: req.Person.Phone,
		},
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (h *Handlers) GetUser(c *gin.Context) {
	out, err := h.Users.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) ListUsers(c *gin.Context) {
	page, err := pagination(c)
	if err != nil {
		fail(c, err)
		return
	}
	q := dto.ListUsers{Page: page, CompanyID: queryPtr(c, "company_id")}
	if v := queryPtr(c, "status"); v != nil {
		s := identity.UserStatus(*v)
		q.Status = &s
	}
	out, err := h.Users.List(c.Request.Context(), q)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) UpdateUser(c *gin.Context) {
	var req updateUserRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.Users.Update(c.Request.Context(), c.Param("id"), dto.UpdateUser(req))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) ChangeUserStatus(c *gin.Context) {
	var req statusRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.Users.ChangeStatus(c.Request.Context(), c.Param("id"), identity.UserStatus(req.Status))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) ChangeUserPassword(c *gin.Context) {
	var req changePasswordRequest
	if !bind(c, &req) {
		return
	}
	err := h.Users.ChangePassword(c.Request.Context(), c.Param("id"), dto.ChangePassword{
		CurrentPassword: req.CurrentPassword, NewPassword: req.NewPassword,
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handlers) ListUserRoles(c *gin.Context) {
	out, err := h.Users.ListRoles(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *Handlers) AssignUserRole(c *gin.Context) {
	var req assignRoleRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.Users.AssignRole(c.Request.Context(), c.Param("id"), dto.AssignRole{
		RoleID: req.RoleID, CompanyID: emptyToNil(req.CompanyID),
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (h *Handlers) RevokeUserRole(c *gin.Context) {
	if err := h.Users.RevokeRole(c.Request.Context(), c.Param("id"), c.Param("assignmentId")); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func emptyToNil(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}
