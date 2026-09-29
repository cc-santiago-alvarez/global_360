package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"global_360/internal/application/dto"
	"global_360/internal/domain/company"
)

type createCompanyRequest struct {
	LegalName         string  `json:"legal_name" binding:"required"`
	TradeName         *string `json:"trade_name"`
	DocumentType      string  `json:"document_type" binding:"required"`
	DocumentNumber    string  `json:"document_number" binding:"required"`
	CountryID         string  `json:"country_id" binding:"required"`
	BillingCurrencyID string  `json:"billing_currency_id" binding:"required"`
	CompanyType       string  `json:"company_type" binding:"required"`
	ContactEmail      *string `json:"contact_email"`
	ContactPhone      *string `json:"contact_phone"`
}

type updateCompanyRequest struct {
	LegalName         *string `json:"legal_name"`
	TradeName         *string `json:"trade_name"`
	BillingCurrencyID *string `json:"billing_currency_id"`
	CompanyType       *string `json:"company_type"`
	ContactEmail      *string `json:"contact_email"`
	ContactPhone      *string `json:"contact_phone"`
}

type statusRequest struct {
	Status string `json:"status" binding:"required"`
}

func (h *Handlers) CreateCompany(c *gin.Context) {
	var req createCompanyRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.Companies.Create(c.Request.Context(), dto.CreateCompany{
		LegalName: req.LegalName, TradeName: req.TradeName, DocumentType: req.DocumentType, DocumentNumber: req.DocumentNumber,
		CountryID: req.CountryID, BillingCurrencyID: req.BillingCurrencyID, CompanyType: company.Type(req.CompanyType),
		ContactEmail: req.ContactEmail, ContactPhone: req.ContactPhone,
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (h *Handlers) GetCompany(c *gin.Context) {
	out, err := h.Companies.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) ListCompanies(c *gin.Context) {
	page, err := pagination(c)
	if err != nil {
		fail(c, err)
		return
	}
	q := dto.ListCompanies{Page: page}
	if v := queryPtr(c, "status"); v != nil {
		s := company.Status(*v)
		q.Status = &s
	}
	if v := queryPtr(c, "company_type"); v != nil {
		t := company.Type(*v)
		q.CompanyType = &t
	}
	out, err := h.Companies.List(c.Request.Context(), q)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) UpdateCompany(c *gin.Context) {
	var req updateCompanyRequest
	if !bind(c, &req) {
		return
	}
	cmd := dto.UpdateCompany{
		LegalName: req.LegalName, TradeName: req.TradeName, BillingCurrencyID: req.BillingCurrencyID,
		ContactEmail: req.ContactEmail, ContactPhone: req.ContactPhone,
	}
	if req.CompanyType != nil {
		t := company.Type(*req.CompanyType)
		cmd.CompanyType = &t
	}
	out, err := h.Companies.Update(c.Request.Context(), c.Param("id"), cmd)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) ChangeCompanyStatus(c *gin.Context) {
	var req statusRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.Companies.ChangeStatus(c.Request.Context(), c.Param("id"), company.Status(req.Status))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
