package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"global_360/internal/application/dto"
	"global_360/internal/domain/commerce"
)

type createCategoryRequest struct {
	Code        string  `json:"code" binding:"required"`
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description"`
}

type updateCategoryRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Active      *bool   `json:"active"`
}

type profileRequest struct {
	DisplayName        string   `json:"display_name" binding:"required"`
	Summary            string   `json:"summary"`
	Description        string   `json:"description"`
	LogoURL            *string  `json:"logo_url"`
	WebsiteURL         *string  `json:"website_url"`
	CategoryIDs        []string `json:"category_ids"`
	CoverageCountryIDs []string `json:"coverage_country_ids"`
	ContactEmail       *string  `json:"contact_email"`
	ContactPhone       *string  `json:"contact_phone"`
	WhatsApp           *string  `json:"whatsapp"`
}

type reasonRequest struct {
	Reason string `json:"reason" binding:"required"`
}

type serviceRequest struct {
	CategoryID           *string `json:"category_id"`
	Name                 *string `json:"name"`
	Description          *string `json:"description"`
	OriginCountryID      *string `json:"origin_country_id"`
	DestinationCountryID *string `json:"destination_country_id"`
	Active               *bool   `json:"active"`
}

type contactRequestRequest struct {
	ServiceID    *string `json:"service_id"`
	Message      string  `json:"message" binding:"required"`
	ContactName  *string `json:"contact_name"`
	ContactEmail *string `json:"contact_email"`
	ContactPhone *string `json:"contact_phone"`
}

type handleContactRequestRequest struct {
	Status *string `json:"status"`
	Notes  *string `json:"notes"`
}

// --- categories ---

func (h *Handlers) ListCategories(c *gin.Context) {
	includeInactive, err := queryBool(c, "include_inactive")
	if err != nil {
		fail(c, err)
		return
	}
	out, err := h.Categories.List(c.Request.Context(), includeInactive)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *Handlers) CreateCategory(c *gin.Context) {
	var req createCategoryRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.Categories.Create(c.Request.Context(), dto.CreateCategory(req))
	respond(c, http.StatusCreated, out, err)
}

func (h *Handlers) UpdateCategory(c *gin.Context) {
	var req updateCategoryRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.Categories.Update(c.Request.Context(), c.Param("id"), dto.UpdateCategory(req))
	respond(c, http.StatusOK, out, err)
}

// --- public marketplace ---

func (h *Handlers) ListDistributors(c *gin.Context) {
	page, err := pagination(c)
	if err != nil {
		fail(c, err)
		return
	}
	out, err := h.Marketplace.ListDistributors(c.Request.Context(), dto.ListDistributors{
		Query: queryPtr(c, "q"), CategoryID: queryPtr(c, "category_id"), CountryID: queryPtr(c, "country_id"), Page: page,
	})
	respond(c, http.StatusOK, out, err)
}

func (h *Handlers) GetDistributor(c *gin.Context) {
	out, err := h.Marketplace.GetDistributor(c.Request.Context(), c.Param("companyId"))
	respond(c, http.StatusOK, out, err)
}

// --- contact requests ---

func (h *Handlers) CreateContactRequest(c *gin.Context) {
	var req contactRequestRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.ContactRequests.Create(c.Request.Context(), c.Param("companyId"), dto.CreateContactRequest(req))
	respond(c, http.StatusCreated, out, err)
}

func (h *Handlers) ListSentContactRequests(c *gin.Context) {
	q, ok := contactRequestQuery(c)
	if !ok {
		return
	}
	out, err := h.ContactRequests.ListSent(c.Request.Context(), q)
	respond(c, http.StatusOK, out, err)
}

func (h *Handlers) ListReceivedContactRequests(c *gin.Context) {
	q, ok := contactRequestQuery(c)
	if !ok {
		return
	}
	out, err := h.ContactRequests.ListReceived(c.Request.Context(), c.Param("companyId"), q)
	respond(c, http.StatusOK, out, err)
}

func (h *Handlers) HandleContactRequest(c *gin.Context) {
	var req handleContactRequestRequest
	if !bind(c, &req) {
		return
	}
	cmd := dto.HandleContactRequest{Notes: req.Notes}
	if req.Status != nil {
		s := commerce.ContactRequestStatus(*req.Status)
		cmd.Status = &s
	}
	out, err := h.ContactRequests.Handle(c.Request.Context(), c.Param("companyId"), c.Param("requestId"), cmd)
	respond(c, http.StatusOK, out, err)
}

func contactRequestQuery(c *gin.Context) (dto.ListContactRequests, bool) {
	page, err := pagination(c)
	if err != nil {
		fail(c, err)
		return dto.ListContactRequests{}, false
	}
	q := dto.ListContactRequests{Page: page}
	if v := queryPtr(c, "status"); v != nil {
		s := commerce.ContactRequestStatus(*v)
		q.Status = &s
	}
	return q, true
}

// --- distributor profile management ---

func (h *Handlers) GetDistributorProfile(c *gin.Context) {
	out, err := h.Distributors.GetProfile(c.Request.Context(), c.Param("companyId"))
	respond(c, http.StatusOK, out, err)
}

// UpsertDistributorProfile answers 201 when the profile is created and 200 when it is updated.
func (h *Handlers) UpsertDistributorProfile(c *gin.Context) {
	var req profileRequest
	if !bind(c, &req) {
		return
	}
	out, created, err := h.Distributors.UpsertProfile(c.Request.Context(), c.Param("companyId"), dto.DistributorProfileInput(req))
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	respond(c, status, out, err)
}

func (h *Handlers) SubmitDistributorProfile(c *gin.Context) {
	h.profileAction(c, h.Distributors.Submit)
}

func (h *Handlers) ApproveDistributorProfile(c *gin.Context) {
	h.profileAction(c, h.Distributors.Approve)
}

func (h *Handlers) ReinstateDistributorProfile(c *gin.Context) {
	h.profileAction(c, h.Distributors.Reinstate)
}

func (h *Handlers) RejectDistributorProfile(c *gin.Context) {
	h.profileReasonAction(c, h.Distributors.Reject)
}

func (h *Handlers) SuspendDistributorProfile(c *gin.Context) {
	h.profileReasonAction(c, h.Distributors.Suspend)
}

func (h *Handlers) profileAction(c *gin.Context, fn func(context.Context, string) (dto.DistributorProfile, error)) {
	out, err := fn(c.Request.Context(), c.Param("companyId"))
	respond(c, http.StatusOK, out, err)
}

func (h *Handlers) profileReasonAction(c *gin.Context, fn func(context.Context, string, string) (dto.DistributorProfile, error)) {
	var req reasonRequest
	if !bind(c, &req) {
		return
	}
	out, err := fn(c.Request.Context(), c.Param("companyId"), req.Reason)
	respond(c, http.StatusOK, out, err)
}

func (h *Handlers) ListDistributorProfiles(c *gin.Context) {
	page, err := pagination(c)
	if err != nil {
		fail(c, err)
		return
	}
	q := dto.ListDistributorProfiles{Page: page}
	if v := queryPtr(c, "status"); v != nil {
		s := commerce.ProfileStatus(*v)
		q.Status = &s
	}
	out, err := h.Distributors.ListProfiles(c.Request.Context(), q)
	respond(c, http.StatusOK, out, err)
}

// --- distributor services ---

func (h *Handlers) ListDistributorServices(c *gin.Context) {
	out, err := h.Distributors.ListServices(c.Request.Context(), c.Param("companyId"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *Handlers) CreateDistributorService(c *gin.Context) {
	var req serviceRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.Distributors.CreateService(c.Request.Context(), c.Param("companyId"), dto.DistributorServiceInput(req))
	respond(c, http.StatusCreated, out, err)
}

func (h *Handlers) UpdateDistributorService(c *gin.Context) {
	var req serviceRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.Distributors.UpdateService(c.Request.Context(), c.Param("companyId"), c.Param("serviceId"), dto.DistributorServiceInput(req))
	respond(c, http.StatusOK, out, err)
}

// respond writes out with status, or pushes err to the error middleware.
func respond(c *gin.Context, status int, out any, err error) {
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(status, out)
}
