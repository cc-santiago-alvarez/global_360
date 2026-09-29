// Package actor carries the authenticated caller and request metadata through
// the context, so use cases can authorize and audit without knowing about HTTP.
package actor

import (
	"context"

	"global_360/internal/domain/access"
)

// Actor is who performs an operation.
type Actor struct {
	UserID    string // empty for anonymous requests (e.g. login)
	CompanyID *string
	Grants    *access.Grants
	IP        string
	UserAgent string
}

func (a Actor) Authenticated() bool { return a.UserID != "" }

// UserIDPtr returns the user id as pointer, nil when anonymous.
func (a Actor) UserIDPtr() *string {
	if a.UserID == "" {
		return nil
	}
	id := a.UserID
	return &id
}

type ctxKey struct{}

func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, ctxKey{}, a)
}

// FromContext returns the actor or an anonymous one.
func FromContext(ctx context.Context) Actor {
	a, _ := ctx.Value(ctxKey{}).(Actor)
	return a
}
