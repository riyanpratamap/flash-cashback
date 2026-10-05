// Package httpapi holds the HTTP router and handlers.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Deps are the collaborators the handlers need.
type Deps struct {
	PingPostgres func(context.Context) error
	PingRedis    func(context.Context) error
	Log          *slog.Logger

	// A route is registered only when its dependency is set.
	Payments    Payer
	Redemptions Redeemer
	History     HistoryReader
	Reads       Reader
}

// NewRouter builds the router with every route under /v1. A money route is
// registered only when its service is set, so it answers 404 until then.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(requestID, d.accessLog, d.recoverer)
	r.NotFound(notFound)
	r.MethodNotAllowed(methodNotAllowed)
	r.Route("/v1", func(r chi.Router) {
		r.Get("/healthz", getHealth(d))
		if d.Reads != nil {
			r.Get("/campaign", d.getCampaign)
			r.Get("/me/cashback", d.getCashback)
		}
		if d.Payments != nil {
			r.Post("/payments", d.postMoney(d.Payments.Pay))
		}
		if d.Redemptions != nil {
			r.Post("/redemptions", d.postMoney(d.Redemptions.Redeem))
		}
		if d.History != nil {
			r.Get("/me/history", d.getHistory)
		}
	})
	return r
}
