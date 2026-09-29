// Package handler adapts HTTP requests to the application use cases.
package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"global_360/internal/application/port"
	"global_360/internal/application/service"
	"global_360/internal/domain/shared"
)

// Handlers groups every HTTP handler.
type Handlers struct {
	Auth      *service.AuthService
	Users     *service.UserService
	Companies *service.CompanyService
	Roles     *service.RoleService
	Catalog   *service.CatalogService
	Audit     *service.AuditService
	DB        port.Pinger

	// Phase 2: marketplace ("commerce").
	Categories      *service.CategoryService
	Marketplace     *service.MarketplaceService
	Distributors    *service.DistributorProfileService
	ContactRequests *service.ContactRequestService
}

// fail pushes err to the error middleware.
func fail(c *gin.Context, err error) { _ = c.Error(err) }

// bind decodes the JSON body; decoding or validation errors become 400.
func bind(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		fail(c, shared.Validation("%s", bindMessage(err)))
		return false
	}
	return true
}

func bindMessage(err error) string {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		msgs := make([]string, 0, len(ve))
		for _, fe := range ve {
			field := toSnake(fe.Field())
			switch fe.Tag() {
			case "required":
				msgs = append(msgs, field+" is required")
			case "email":
				msgs = append(msgs, field+" must be a valid email")
			default:
				msgs = append(msgs, fmt.Sprintf("%s is invalid (%s)", field, fe.Tag()))
			}
		}
		return strings.Join(msgs, "; ")
	}
	if strings.Contains(err.Error(), "EOF") {
		return "request body is required"
	}
	return "invalid JSON body"
}

func toSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

func queryPtr(c *gin.Context, key string) *string {
	if v, ok := c.GetQuery(key); ok && strings.TrimSpace(v) != "" {
		v = strings.TrimSpace(v)
		return &v
	}
	return nil
}

func queryBool(c *gin.Context, key string) (bool, error) {
	v := c.Query(key)
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, shared.Validation("%s must be a boolean", key)
	}
	return b, nil
}

func pagination(c *gin.Context) (shared.Pagination, error) {
	page, err := queryInt(c, "page")
	if err != nil {
		return shared.Pagination{}, err
	}
	size, err := queryInt(c, "page_size")
	if err != nil {
		return shared.Pagination{}, err
	}
	return shared.Pagination{Page: page, PageSize: size}, nil
}

func queryInt(c *gin.Context, key string) (int, error) {
	v := c.Query(key)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, shared.Validation("%s must be an integer", key)
	}
	return n, nil
}

func queryTime(c *gin.Context, key string) (*time.Time, error) {
	v := c.Query(key)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, shared.Validation("%s must be an RFC 3339 timestamp", key)
	}
	t = t.UTC()
	return &t, nil
}

// Health is the liveness probe.
func (h *Handlers) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Ready checks the database connection.
func (h *Handlers) Ready(c *gin.Context) {
	if h.DB == nil {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return
	}
	if err := h.DB.Ping(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "database": "down"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "database": "up"})
}
