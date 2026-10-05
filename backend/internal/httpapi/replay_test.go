package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
)

func TestKeyReusedIs409(t *testing.T) {
	r := newRig()
	r.fake.err = domain.ErrIdempotencyKeyReused
	rec := r.do(http.MethodPost, "/v1/payments", reqOpt{user: str("user_a"), key: str(goodKey), body: `{"amount":50000}`})
	r.wantError(t, rec, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED")
	if h := rec.Header().Get("Idempotent-Replayed"); h != "" {
		t.Errorf("Idempotent-Replayed = %q on 409", h)
	}
}

func TestReplayedPaymentIs200WithHeader(t *testing.T) {
	r := newRig()
	r.fake.replayed = true
	rec := r.do(http.MethodPost, "/v1/payments", reqOpt{user: str("user_a"), key: str(goodKey), body: `{"amount":50000}`})
	if rec.Code != http.StatusOK || rec.Header().Get("Idempotent-Replayed") != "true" {
		t.Errorf("status = %d, header = %q, want 200 and true", rec.Code, rec.Header().Get("Idempotent-Replayed"))
	}
}

func TestFreshPaymentIs201WithoutHeader(t *testing.T) {
	r := newRig()
	rec := r.do(http.MethodPost, "/v1/payments", reqOpt{user: str("user_a"), key: str(goodKey), body: `{"amount":50000}`})
	if rec.Code != http.StatusCreated || rec.Header().Get("Idempotent-Replayed") != "" {
		t.Errorf("status = %d, header = %q, want 201 and none", rec.Code, rec.Header().Get("Idempotent-Replayed"))
	}
}

func TestRedemptionErrorsAreMapped(t *testing.T) {
	for _, c := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"insufficient balance", domain.ErrInsufficientBalance, http.StatusUnprocessableEntity, "INSUFFICIENT_BALANCE"},
		{"paused", domain.ErrRedemptionPaused, http.StatusConflict, "REDEMPTION_PAUSED"},
		{"key reused", domain.ErrIdempotencyKeyReused, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig()
			r.fake.err = c.err
			rec := r.do(http.MethodPost, "/v1/redemptions", reqOpt{user: str("user_a"), key: str(goodKey), body: `{"amount":5000}`})
			r.wantError(t, rec, c.status, c.code)
			if h := rec.Header().Get("Idempotent-Replayed"); h != "" {
				t.Errorf("Idempotent-Replayed = %q on an error", h)
			}
		})
	}
}

func TestRedemptionReplayedIs200WithHeaderAndFreshIs201(t *testing.T) {
	for _, c := range []struct {
		name     string
		replayed bool
		status   int
		header   string
	}{
		{"replayed", true, http.StatusOK, "true"},
		{"fresh", false, http.StatusCreated, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig()
			r.fake.replayed = c.replayed
			rec := r.do(http.MethodPost, "/v1/redemptions", reqOpt{user: str("user_a"), key: str(goodKey), body: `{"amount":5000}`})
			if rec.Code != c.status || rec.Header().Get("Idempotent-Replayed") != c.header {
				t.Errorf("status = %d, header = %q, want %d and %q", rec.Code, rec.Header().Get("Idempotent-Replayed"), c.status, c.header)
			}
			if !strings.Contains(rec.Body.String(), `"balance_after"`) || !strings.Contains(rec.Body.String(), `"redemption"`) {
				t.Errorf("body = %s, want redemption and balance_after", rec.Body)
			}
		})
	}
}
