package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
)

// Reader is what GET /campaign and GET /me/cashback need.
type Reader interface {
	Campaign(ctx context.Context) (domain.CampaignView, error)
	Cashback(ctx context.Context, user domain.UserID) (domain.CashbackView, error)
}

// getCampaign validates the user (every route but health needs one), then
// serves the campaign view.
func (d Deps) getCampaign(w http.ResponseWriter, r *http.Request) {
	user, err := domain.ParseUserID(r.Header.Values("X-User-ID"))
	if err != nil {
		d.writeFailure(w, r, err)
		return
	}
	setLogUser(r.Context(), string(user))
	view, err := d.Reads.Campaign(r.Context())
	if err != nil {
		d.writeFailure(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (d Deps) getCashback(w http.ResponseWriter, r *http.Request) {
	user, err := domain.ParseUserID(r.Header.Values("X-User-ID"))
	if err != nil {
		d.writeFailure(w, r, err)
		return
	}
	setLogUser(r.Context(), string(user))
	view, err := d.Reads.Cashback(r.Context(), user)
	if err != nil {
		d.writeFailure(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // the status is sent; a failed write has nowhere to be reported
}
