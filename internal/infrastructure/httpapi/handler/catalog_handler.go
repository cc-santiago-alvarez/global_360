package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"global_360/internal/application/dto"
)

type createCountryRequest struct {
	ISOCode string `json:"iso_code" binding:"required"`
	Name    string `json:"name" binding:"required"`
}

type updateCountryRequest struct {
	Name   *string `json:"name"`
	Active *bool   `json:"active"`
}

type createCurrencyRequest struct {
	ISOCode  string `json:"iso_code" binding:"required"`
	Name     string `json:"name" binding:"required"`
	Symbol   string `json:"symbol" binding:"required"`
	Decimals *int   `json:"decimals"`
}

type updateCurrencyRequest struct {
	Name     *string `json:"name"`
	Symbol   *string `json:"symbol"`
	Decimals *int    `json:"decimals"`
	Active   *bool   `json:"active"`
}

func (h *Handlers) ListCountries(c *gin.Context) {
	onlyActive, err := queryBool(c, "active")
	if err != nil {
		fail(c, err)
		return
	}
	out, err := h.Catalog.ListCountries(c.Request.Context(), onlyActive)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *Handlers) CreateCountry(c *gin.Context) {
	var req createCountryRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.Catalog.CreateCountry(c.Request.Context(), dto.CreateCountry{ISOCode: req.ISOCode, Name: req.Name})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (h *Handlers) UpdateCountry(c *gin.Context) {
	var req updateCountryRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.Catalog.UpdateCountry(c.Request.Context(), c.Param("id"), dto.UpdateCountry{Name: req.Name, Active: req.Active})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) ListCurrencies(c *gin.Context) {
	onlyActive, err := queryBool(c, "active")
	if err != nil {
		fail(c, err)
		return
	}
	out, err := h.Catalog.ListCurrencies(c.Request.Context(), onlyActive)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *Handlers) CreateCurrency(c *gin.Context) {
	var req createCurrencyRequest
	if !bind(c, &req) {
		return
	}
	decimals := 2
	if req.Decimals != nil {
		decimals = *req.Decimals
	}
	out, err := h.Catalog.CreateCurrency(c.Request.Context(), dto.CreateCurrency{
		ISOCode: req.ISOCode, Name: req.Name, Symbol: req.Symbol, Decimals: decimals,
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (h *Handlers) UpdateCurrency(c *gin.Context) {
	var req updateCurrencyRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.Catalog.UpdateCurrency(c.Request.Context(), c.Param("id"), dto.UpdateCurrency{
		Name: req.Name, Symbol: req.Symbol, Decimals: req.Decimals, Active: req.Active,
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
