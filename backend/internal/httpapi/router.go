// Package httpapi holds the HTTP router and handlers.
package httpapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Deps are the collaborators the handlers need.
type Deps struct {
	PingPostgres func(context.Context) error
	PingRedis    func(context.Context) error
}

// NewRouter builds the router with every route under /v1.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Get("/healthz", getHealth(d))
	})
	return r
}
