package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
)

// MoneyCommand is a validated POST /payments or POST /redemptions request.
type MoneyCommand struct {
	UserID domain.UserID
	Key    uuid.UUID
	Amount int64
	Hash   [32]byte
}

// Payer is what POST /payments needs. The signature widens when the service
// lands (P2.2).
type Payer interface {
	Pay(ctx context.Context, cmd MoneyCommand) error
}

// Redeemer is what POST /redemptions needs (P3.1).
type Redeemer interface {
	Redeem(ctx context.Context, cmd MoneyCommand) error
}

// HistoryReader is what GET /me/history needs (P3.3).
type HistoryReader interface {
	History(ctx context.Context, user domain.UserID, limit int) error
}

const maxBodyBytes = 1024

// postMoney validates user, key, then body (AC-18), builds the typed command
// and calls the service. Success rendering waits for the services (P2.2, P3.1):
// until then a call that returns nil answers 201 with no body.
func (d Deps) postMoney(call func(context.Context, MoneyCommand) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := domain.ParseUserID(r.Header.Values("X-User-ID"))
		if err != nil {
			d.writeFailure(w, r, err)
			return
		}
		setLogUser(r.Context(), string(user))
		key, err := domain.ParseIdempotencyKey(r.Header.Values("Idempotency-Key"))
		if err != nil {
			d.writeFailure(w, r, err)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		if err != nil {
			d.writeFailure(w, r, domain.ErrMalformedRequest)
			return
		}
		amount, err := domain.ParseAmountBody(body)
		if err != nil {
			d.writeFailure(w, r, err)
			return
		}
		cmd := MoneyCommand{UserID: user, Key: key, Amount: amount, Hash: domain.RequestHash(amount)}
		if err := call(r.Context(), cmd); err != nil {
			d.writeFailure(w, r, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

// getHistory validates user, then limit, then calls the reader; success is
// rendered in P3.3.
func (d Deps) getHistory(w http.ResponseWriter, r *http.Request) {
	user, err := domain.ParseUserID(r.Header.Values("X-User-ID"))
	if err != nil {
		d.writeFailure(w, r, err)
		return
	}
	setLogUser(r.Context(), string(user))
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		d.writeFailure(w, r, domain.ErrMalformedRequest)
		return
	}
	vals := query["limit"]
	if len(vals) > 1 {
		d.writeFailure(w, r, domain.ErrMalformedRequest)
		return
	}
	raw := ""
	if len(vals) == 1 {
		raw = vals[0]
	}
	limit, err := domain.ParseLimit(raw, len(vals) == 1)
	if err != nil {
		d.writeFailure(w, r, err)
		return
	}
	if err := d.History.History(r.Context(), user, limit); err != nil {
		d.writeFailure(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}
