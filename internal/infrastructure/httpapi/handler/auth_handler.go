package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type loginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *Handlers) Login(c *gin.Context) {
	var req loginRequest
	if !bind(c, &req) {
		return
	}
	pair, err := h.Auth.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, pair)
}

func (h *Handlers) Refresh(c *gin.Context) {
	var req refreshRequest
	if !bind(c, &req) {
		return
	}
	pair, err := h.Auth.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, pair)
}

func (h *Handlers) Logout(c *gin.Context) {
	var req refreshRequest
	if !bind(c, &req) {
		return
	}
	if err := h.Auth.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handlers) Me(c *gin.Context) {
	me, err := h.Auth.Me(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, me)
}
