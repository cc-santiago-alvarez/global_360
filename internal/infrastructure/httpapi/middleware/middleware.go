// Package middleware contains the Gin middleware of the HTTP adapter.
package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"global_360/internal/application/actor"
	"global_360/internal/domain/shared"
)

const (
	HeaderRequestID = "X-Request-ID"
	ctxRequestID    = "request_id"
)

// ErrorBody is the single error format of the API.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// RequestID propagates or generates a request id.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if id == "" || len(id) > 64 {
			id = uuid.NewString()
		}
		c.Set(ctxRequestID, id)
		c.Header(HeaderRequestID, id)
		c.Next()
	}
}

// AccessLog writes one structured line per request.
func AccessLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		status := c.Writer.Status()
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		}
		log.LogAttrs(c.Request.Context(), level, "http request",
			slog.String("request_id", c.GetString(ctxRequestID)),
			slog.String("method", c.Request.Method),
			slog.String("path", c.FullPath()),
			slog.Int("status", status),
			slog.Duration("latency", time.Since(start)),
			slog.String("ip", c.ClientIP()),
			slog.String("user_id", actor.FromContext(c.Request.Context()).UserID),
		)
	}
}

// Recovery turns panics into a 500 response.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		log.Error("panic recovered", "request_id", c.GetString(ctxRequestID), "panic", recovered)
		abort(c, http.StatusInternalServerError, "internal_error", "internal server error")
	})
}

// RequestContext stores request metadata (ip, user agent) in the actor used by use cases.
func RequestContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		a := actor.Actor{IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}
		c.Request = c.Request.WithContext(actor.WithActor(c.Request.Context(), a))
		c.Next()
	}
}

// ErrorHandler renders the last error pushed with c.Error, mapping domain error kinds.
func ErrorHandler(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}
		err := c.Errors.Last().Err
		status, code := classify(err)
		msg := err.Error()
		if status == http.StatusInternalServerError {
			log.Error("request failed", "request_id", c.GetString(ctxRequestID), "error", err)
			msg = "internal server error"
		}
		abort(c, status, code, msg)
	}
}

func classify(err error) (int, string) {
	switch {
	case errors.Is(err, shared.ErrValidation):
		return http.StatusBadRequest, shared.ErrValidation.Error()
	case errors.Is(err, shared.ErrUnauthorized):
		return http.StatusUnauthorized, shared.ErrUnauthorized.Error()
	case errors.Is(err, shared.ErrForbidden):
		return http.StatusForbidden, shared.ErrForbidden.Error()
	case errors.Is(err, shared.ErrNotFound):
		return http.StatusNotFound, shared.ErrNotFound.Error()
	case errors.Is(err, shared.ErrConflict):
		return http.StatusConflict, shared.ErrConflict.Error()
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "timeout"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}

func abort(c *gin.Context, status int, code, msg string) {
	c.AbortWithStatusJSON(status, ErrorBody{Error: ErrorDetail{Code: code, Message: msg, RequestID: c.GetString(ctxRequestID)}})
}

// NoRoute answers unknown paths with the standard error body.
func NoRoute(c *gin.Context) {
	abort(c, http.StatusNotFound, shared.ErrNotFound.Error(), "route not found")
}

// NoMethod answers known paths called with an unsupported method.
func NoMethod(c *gin.Context) {
	abort(c, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
}

// Authenticator resolves the caller from an access token.
type Authenticator interface {
	Authenticate(ctx context.Context, accessToken string) (actor.Actor, error)
}

// Authenticate requires a valid "Authorization: Bearer <token>" header.
func Authenticate(auth Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		authenticate(c, auth, true)
	}
}

// OptionalAuthenticate lets anonymous requests through but still validates a token
// when one is sent: an invalid token is rejected rather than silently ignored.
func OptionalAuthenticate(auth Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		authenticate(c, auth, c.GetHeader("Authorization") != "")
	}
}

func authenticate(c *gin.Context, auth Authenticator, required bool) {
	if !required {
		c.Next()
		return
	}
	token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		_ = c.Error(shared.Unauthorized("missing bearer token"))
		c.Abort()
		return
	}
	a, err := auth.Authenticate(c.Request.Context(), strings.TrimSpace(token))
	if err != nil {
		_ = c.Error(err)
		c.Abort()
		return
	}
	c.Request = c.Request.WithContext(actor.WithActor(c.Request.Context(), a))
	c.Next()
}

// RequirePermission rejects callers that do not hold code anywhere (globally or in
// some company). Use cases perform the fine grained, company scoped check.
func RequirePermission(code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !actor.FromContext(c.Request.Context()).Grants.HasAny(code) {
			_ = c.Error(shared.Forbidden("missing permission %s", code))
			c.Abort()
			return
		}
		c.Next()
	}
}
