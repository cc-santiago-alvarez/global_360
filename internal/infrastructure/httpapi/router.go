// Package httpapi is the HTTP (Gin) driving adapter.
package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"global_360/internal/domain/access"
	"global_360/internal/infrastructure/httpapi/handler"
	mw "global_360/internal/infrastructure/httpapi/middleware"
)

// NewRouter wires middleware and routes.
func NewRouter(h *handler.Handlers, log *slog.Logger) *gin.Engine {
	r := gin.New()
	r.HandleMethodNotAllowed = true
	r.Use(mw.RequestID(), mw.Recovery(log), mw.AccessLog(log), mw.RequestContext(), mw.ErrorHandler(log))

	r.GET("/health", h.Health)
	r.GET("/ready", h.Ready)

	api := r.Group("/api/v1")
	api.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "pong"}) })

	auth := api.Group("/auth")
	auth.POST("/login", h.Login)
	auth.POST("/refresh", h.Refresh)

	p := api.Group("", mw.Authenticate(h.Auth))
	p.POST("/auth/logout", h.Logout)
	p.GET("/auth/me", h.Me)

	perm := mw.RequirePermission

	p.GET("/countries", perm(access.PermCatalogRead), h.ListCountries)
	p.POST("/countries", perm(access.PermCatalogManage), h.CreateCountry)
	p.PATCH("/countries/:id", perm(access.PermCatalogManage), h.UpdateCountry)
	p.GET("/currencies", perm(access.PermCatalogRead), h.ListCurrencies)
	p.POST("/currencies", perm(access.PermCatalogManage), h.CreateCurrency)
	p.PATCH("/currencies/:id", perm(access.PermCatalogManage), h.UpdateCurrency)

	p.GET("/companies", perm(access.PermCompanyRead), h.ListCompanies)
	p.POST("/companies", perm(access.PermCompanyManage), h.CreateCompany)
	p.GET("/companies/:id", perm(access.PermCompanyRead), h.GetCompany)
	p.PATCH("/companies/:id", perm(access.PermCompanyManage), h.UpdateCompany)
	p.PATCH("/companies/:id/status", perm(access.PermCompanyManage), h.ChangeCompanyStatus)

	// Self-service operations (read own user, change own password) are authorized in the use case.
	p.GET("/users", perm(access.PermUserRead), h.ListUsers)
	p.POST("/users", perm(access.PermUserManage), h.CreateUser)
	p.GET("/users/:id", h.GetUser)
	p.PATCH("/users/:id", h.UpdateUser)
	p.PATCH("/users/:id/status", perm(access.PermUserManage), h.ChangeUserStatus)
	p.PUT("/users/:id/password", h.ChangeUserPassword)
	p.GET("/users/:id/roles", h.ListUserRoles)
	p.POST("/users/:id/roles", perm(access.PermUserManage), h.AssignUserRole)
	p.DELETE("/users/:id/roles/:assignmentId", perm(access.PermUserManage), h.RevokeUserRole)

	p.GET("/permissions", perm(access.PermRoleRead), h.ListPermissions)
	p.GET("/roles", perm(access.PermRoleRead), h.ListRoles)
	p.POST("/roles", perm(access.PermRoleManage), h.CreateRole)
	p.GET("/roles/:id", perm(access.PermRoleRead), h.GetRole)
	p.PATCH("/roles/:id", perm(access.PermRoleManage), h.UpdateRole)
	p.PUT("/roles/:id/permissions", perm(access.PermRoleManage), h.ReplaceRolePermissions)

	p.GET("/audit-logs", perm(access.PermAuditRead), h.ListAuditLogs)

	registerCommerce(api, h)

	r.NoRoute(mw.NoRoute)
	r.NoMethod(mw.NoMethod)
	return r
}

// registerCommerce mounts the marketplace ("commerce" section). Browsing is public; a
// valid token only adds the distributor contact data. Use cases do the company scoped checks.
func registerCommerce(api *gin.RouterGroup, h *handler.Handlers) {
	perm := mw.RequirePermission

	public := api.Group("/commerce", mw.OptionalAuthenticate(h.Auth))
	public.GET("/categories", h.ListCategories)
	public.GET("/distributors", h.ListDistributors)
	public.GET("/distributors/:companyId", h.GetDistributor)

	cm := api.Group("/commerce", mw.Authenticate(h.Auth))
	cm.POST("/categories", perm(access.PermCommerceManage), h.CreateCategory)
	cm.PATCH("/categories/:id", perm(access.PermCommerceManage), h.UpdateCategory)

	// Clients: contact distributors and follow the requests they sent.
	cm.POST("/distributors/:companyId/contact-requests", perm(access.PermCommerceContact), h.CreateContactRequest)
	cm.GET("/contact-requests", perm(access.PermCommerceContact), h.ListSentContactRequests)

	// Distributors (or Global 360 staff): profile, services and received leads.
	cm.GET("/distributors/:companyId/profile", h.GetDistributorProfile)
	cm.PUT("/distributors/:companyId/profile", h.UpsertDistributorProfile)
	cm.POST("/distributors/:companyId/profile/submit", h.SubmitDistributorProfile)
	cm.GET("/distributors/:companyId/services", h.ListDistributorServices)
	cm.POST("/distributors/:companyId/services", h.CreateDistributorService)
	cm.PATCH("/distributors/:companyId/services/:serviceId", h.UpdateDistributorService)
	cm.GET("/distributors/:companyId/contact-requests", h.ListReceivedContactRequests)
	cm.PATCH("/distributors/:companyId/contact-requests/:requestId", h.HandleContactRequest)

	// Global 360 review queue.
	cm.GET("/profiles", perm(access.PermCommerceManage), h.ListDistributorProfiles)
	cm.POST("/distributors/:companyId/profile/approve", perm(access.PermCommerceManage), h.ApproveDistributorProfile)
	cm.POST("/distributors/:companyId/profile/reject", perm(access.PermCommerceManage), h.RejectDistributorProfile)
	cm.POST("/distributors/:companyId/profile/suspend", perm(access.PermCommerceManage), h.SuspendDistributorProfile)
	cm.POST("/distributors/:companyId/profile/reinstate", perm(access.PermCommerceManage), h.ReinstateDistributorProfile)
}

// NewServer builds the HTTP server with sane timeouts.
func NewServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}
