package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
)

// Payer is what POST /payments needs.
type Payer interface {
	Pay(ctx context.Context, cmd domain.MoneyCommand) (res domain.PaymentResult, replayed bool, err error)
}

// Redeemer is what POST /redemptions needs (P3.1).
type Redeemer interface {
	Redeem(ctx context.Context, cmd domain.MoneyCommand) (res domain.RedemptionResult, replayed bool, err error)
}

// HistoryReader is what GET /me/history needs (P3.3).
type HistoryReader interface {
	History(ctx context.Context, user domain.UserID, limit int) error
}

const maxBodyBytes = 1024

// parseMoney validates user, key, then body (AC-18) and builds the typed
// command. On failure it has already written the response.
func (d Deps) parseMoney(w http.ResponseWriter, r *http.Request) (domain.MoneyCommand, bool) {
	user, err := domain.ParseUserID(r.Header.Values("X-User-ID"))
	if err != nil {
		d.writeFailure(w, r, err)
		return domain.MoneyCommand{}, false
	}
	setLogUser(r.Context(), string(user))
	key, err := domain.ParseIdempotencyKey(r.Header.Values("Idempotency-Key"))
	if err != nil {
		d.writeFailure(w, r, err)
		return domain.MoneyCommand{}, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		d.writeFailure(w, r, domain.ErrMalformedRequest)
		return domain.MoneyCommand{}, false
	}
	amount, err := domain.ParseAmountBody(body)
	if err != nil {
		d.writeFailure(w, r, err)
		return domain.MoneyCommand{}, false
	}
	return domain.MoneyCommand{
		UserID: user, Key: key, Amount: amount, Hash: domain.RequestHash(amount),
		RequestID: requestIDFrom(r.Context()),
	}, true
}

// postPayment answers 201 with the contract body, or 200 with
// Idempotent-Replayed: true when the key already had a payment.
func (d Deps) postPayment(w http.ResponseWriter, r *http.Request) {
	cmd, ok := d.parseMoney(w, r)
	if !ok {
		return
	}
	res, replayed, err := d.Payments.Pay(r.Context(), cmd)
	if err != nil {
		d.writeFailure(w, r, err)
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
		writeJSON(w, http.StatusOK, res)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

// postRedemption answers 201 with the contract body, or 200 with
// Idempotent-Replayed: true when the key already had a redemption.
func (d Deps) postRedemption(w http.ResponseWriter, r *http.Request) {
	cmd, ok := d.parseMoney(w, r)
	if !ok {
		return
	}
	res, replayed, err := d.Redemptions.Redeem(r.Context(), cmd)
	if err != nil {
		d.writeFailure(w, r, err)
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
		writeJSON(w, http.StatusOK, res)
		return
	}
	writeJSON(w, http.StatusCreated, res)
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
