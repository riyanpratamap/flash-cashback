package httpapi

import (
	"net/http"
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
