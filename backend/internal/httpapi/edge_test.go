package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
)

const goodKey = "123e4567-e89b-12d3-a456-426614174000"

type fakeMoney struct {
	calls    []domain.MoneyCommand
	histCall []histArgs
	err      error
	panicVal any
	replayed bool
}

type histArgs struct {
	user  domain.UserID
	limit int
}

func (f *fakeMoney) Pay(_ context.Context, c domain.MoneyCommand) (domain.PaymentResult, bool, error) {
	return domain.PaymentResult{}, f.replayed, f.record(c)
}
func (f *fakeMoney) Redeem(_ context.Context, c domain.MoneyCommand) (domain.RedemptionResult, bool, error) {
	return domain.RedemptionResult{}, f.replayed, f.record(c)
}
func (f *fakeMoney) record(c domain.MoneyCommand) error {
	f.calls = append(f.calls, c)
	if f.panicVal != nil {
		panic(f.panicVal)
	}
	return f.err
}
func (f *fakeMoney) History(_ context.Context, u domain.UserID, limit int) (domain.HistoryView, error) {
	f.histCall = append(f.histCall, histArgs{u, limit})
	return domain.HistoryView{}, f.err
}

type rig struct {
	h    http.Handler
	fake *fakeMoney
	logs *bytes.Buffer
}

func newRig() rig {
	f := &fakeMoney{}
	logs := &bytes.Buffer{}
	h := NewRouter(Deps{
		PingPostgres: ok,
		PingRedis:    ok,
		Log:          slog.New(slog.NewJSONHandler(logs, nil)),
		Payments:     f,
		Redemptions:  f,
		History:      f,
	})
	return rig{h, f, logs}
}

type reqOpt struct {
	user, key *string
	body      string
	reqID     string
}

func str(s string) *string { return &s }

func (r rig) do(method, path string, o reqOpt) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(o.body))
	if o.user != nil {
		req.Header.Set("X-User-ID", *o.user)
	}
	if o.key != nil {
		req.Header.Set("Idempotency-Key", *o.key)
	}
	if o.reqID != "" {
		req.Header.Set("X-Request-ID", o.reqID)
	}
	rec := httptest.NewRecorder()
	r.h.ServeHTTP(rec, req)
	return rec
}

type envelope struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("body is not an envelope: %q: %v", rec.Body.String(), err)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if e.Error.RequestID == "" || e.Error.RequestID != rec.Header().Get("X-Request-ID") {
		t.Errorf("request_id %q, header %q", e.Error.RequestID, rec.Header().Get("X-Request-ID"))
	}
	return e
}

func (r rig) wantError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, status, rec.Body)
	}
	if e := decodeEnvelope(t, rec); e.Error.Code != code {
		t.Fatalf("code = %q, want %q", e.Error.Code, code)
	}
}

var postPaths = []string{"/v1/payments", "/v1/redemptions"}

// AC-18: user, then key, then body.
func TestCheckOrder(t *testing.T) {
	tests := []struct {
		name   string
		o      reqOpt
		status int
		code   string
	}{
		{"all bad", reqOpt{user: str("User_A"), key: str("abc"), body: "x"}, 400, "INVALID_USER"},
		{"no user, bad key, bad body", reqOpt{key: str("abc"), body: "x"}, 400, "MISSING_USER"},
		{"good user, missing key, bad body", reqOpt{user: str("user_a"), body: "x"}, 400, "MISSING_IDEMPOTENCY_KEY"},
		{"good user, bad key, bad body", reqOpt{user: str("user_a"), key: str("abc"), body: "x"}, 400, "INVALID_IDEMPOTENCY_KEY"},
		{"good headers, bad body", reqOpt{user: str("user_a"), key: str(goodKey), body: "x"}, 400, "MALFORMED_REQUEST"},
		{"good headers, bad amount", reqOpt{user: str("user_a"), key: str(goodKey), body: `{"amount":0}`}, 422, "INVALID_AMOUNT"},
	}
	for _, path := range postPaths {
		for _, tc := range tests {
			t.Run(path+" "+tc.name, func(t *testing.T) {
				r := newRig()
				r.wantError(t, r.do("POST", path, tc.o), tc.status, tc.code)
				if len(r.fake.calls) != 0 {
					t.Fatalf("service called %d times on a 4xx", len(r.fake.calls))
				}
			})
		}
	}
}

// paddedBody is a valid amount body padded with spaces to exactly n bytes.
func paddedBody(n int) string {
	const base = `{"amount":20000}`
	return base + strings.Repeat(" ", n-len(base))
}

// AC-17: a body of exactly 1 KiB is read; one byte more is malformed.
func TestBodyLimitBoundary(t *testing.T) {
	for _, path := range postPaths {
		t.Run(path, func(t *testing.T) {
			r := newRig()
			body := paddedBody(1024)
			if len(body) != 1024 {
				t.Fatalf("test body is %d bytes", len(body))
			}
			rec := r.do("POST", path, reqOpt{user: str("user_a"), key: str(goodKey), body: body})
			if rec.Code != http.StatusCreated || len(r.fake.calls) != 1 || r.fake.calls[0].Amount != 20000 {
				t.Fatalf("status %d calls %+v", rec.Code, r.fake.calls)
			}
		})
	}
}

// AC-17.
func TestAmountValues(t *testing.T) {
	tests := []struct {
		body   string
		status int
		code   string
	}{
		{`{"amount":0}`, 422, "INVALID_AMOUNT"},
		{`{"amount":-1}`, 422, "INVALID_AMOUNT"},
		{`{"amount":10000001}`, 422, "INVALID_AMOUNT"},
		{`{"amount":1.5}`, 422, "INVALID_AMOUNT"},
		{`{"amount":100000.0}`, 422, "INVALID_AMOUNT"},
		{`{"amount":1e5}`, 422, "INVALID_AMOUNT"},
		{`{"amount":"100000"}`, 400, "MALFORMED_REQUEST"},
		{`{"amount":null}`, 400, "MALFORMED_REQUEST"},
		{`{"amount":true}`, 400, "MALFORMED_REQUEST"},
		{`{}`, 400, "MALFORMED_REQUEST"},
		{`{"other":1}`, 400, "MALFORMED_REQUEST"},
		{`{"amount":1,"extra":1}`, 400, "MALFORMED_REQUEST"},
		{`{"Amount":1}`, 400, "MALFORMED_REQUEST"},
		{`{"amount":1,"amount":2}`, 400, "MALFORMED_REQUEST"},
		{`not json`, 400, "MALFORMED_REQUEST"},
		{``, 400, "MALFORMED_REQUEST"},
		{`{"amount":1}` + strings.Repeat(" ", 1024), 400, "MALFORMED_REQUEST"},
		{paddedBody(1025), 400, "MALFORMED_REQUEST"},
	}
	for _, path := range postPaths {
		for _, tc := range tests {
			t.Run(path+" "+tc.body[:min(len(tc.body), 30)], func(t *testing.T) {
				r := newRig()
				rec := r.do("POST", path, reqOpt{user: str("user_a"), key: str(goodKey), body: tc.body})
				r.wantError(t, rec, tc.status, tc.code)
				if len(r.fake.calls) != 0 {
					t.Fatal("service called on a 4xx")
				}
			})
		}
	}
}

// AC-18: user and key values.
func TestHeaderValues(t *testing.T) {
	body := `{"amount":20000}`
	tests := []struct {
		name string
		o    reqOpt
		code string
	}{
		{"missing user", reqOpt{key: str(goodKey), body: body}, "MISSING_USER"},
		{"upper-case user", reqOpt{user: str("User_A"), key: str(goodKey), body: body}, "INVALID_USER"},
		{"empty user", reqOpt{user: str(""), key: str(goodKey), body: body}, "INVALID_USER"},
		{"65-char user", reqOpt{user: str(strings.Repeat("a", 65)), key: str(goodKey), body: body}, "INVALID_USER"},
		{"user with space", reqOpt{user: str("a b"), key: str(goodKey), body: body}, "INVALID_USER"},
		{"missing key", reqOpt{user: str("user_a"), body: body}, "MISSING_IDEMPOTENCY_KEY"},
		{"abc key", reqOpt{user: str("user_a"), key: str("abc"), body: body}, "INVALID_IDEMPOTENCY_KEY"},
		{"braced key", reqOpt{user: str("user_a"), key: str("{" + goodKey + "}"), body: body}, "INVALID_IDEMPOTENCY_KEY"},
		{"urn key", reqOpt{user: str("user_a"), key: str("urn:uuid:" + goodKey), body: body}, "INVALID_IDEMPOTENCY_KEY"},
		{"dash-less key", reqOpt{user: str("user_a"), key: str(strings.ReplaceAll(goodKey, "-", "")), body: body}, "INVALID_IDEMPOTENCY_KEY"},
	}
	for _, path := range postPaths {
		for _, tc := range tests {
			t.Run(path+" "+tc.name, func(t *testing.T) {
				r := newRig()
				r.wantError(t, r.do("POST", path, tc.o), 400, tc.code)
				if len(r.fake.calls) != 0 {
					t.Fatal("service called on a 4xx")
				}
			})
		}
	}
}

func TestValidRequestReachesService(t *testing.T) {
	for _, path := range postPaths {
		t.Run(path, func(t *testing.T) {
			r := newRig()
			rec := r.do("POST", path, reqOpt{
				user: str("user_a"), key: str(strings.ToUpper(goodKey)), body: ` { "amount" : 100000 } `, reqID: "rid-9",
			})
			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d", rec.Code)
			}
			if len(r.fake.calls) != 1 {
				t.Fatalf("calls = %d", len(r.fake.calls))
			}
			want := domain.MoneyCommand{
				UserID: "user_a", Key: uuid.MustParse(goodKey), Amount: 100000, Hash: domain.RequestHash(100000),
				RequestID: "rid-9",
			}
			if r.fake.calls[0] != want {
				t.Fatalf("command = %+v, want %+v", r.fake.calls[0], want)
			}
		})
	}
}

func TestServiceErrorIsMappedWithFixedMessage(t *testing.T) {
	r := newRig()
	r.fake.err = errors.New(`ERROR: relation "ledger" SELECT secret_column`)
	rec := r.do("POST", "/v1/payments", reqOpt{user: str("user_a"), key: str(goodKey), body: `{"amount":20000}`})
	r.wantError(t, rec, 500, "INTERNAL_ERROR")
	if strings.Contains(rec.Body.String(), "secret_column") || strings.Contains(r.logs.String(), "secret_column") {
		t.Fatal("driver text leaked into body or log")
	}
}

func TestNotFoundAndMethodNotAllowed(t *testing.T) {
	r := newRig()
	r.wantError(t, r.do("GET", "/v1/nope", reqOpt{}), 404, "NOT_FOUND")
	r.wantError(t, r.do("DELETE", "/v1/healthz", reqOpt{}), 405, "METHOD_NOT_ALLOWED")
	r.wantError(t, r.do("GET", "/v1/payments", reqOpt{user: str("user_a")}), 405, "METHOD_NOT_ALLOWED")
}

func TestRoutesWithoutServiceAreNotRegistered(t *testing.T) {
	h := NewRouter(Deps{PingPostgres: ok, PingRedis: ok, Log: slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))})
	r := rig{h: h}
	for _, p := range []string{"/v1/payments", "/v1/redemptions"} {
		r.wantError(t, r.do("POST", p, reqOpt{user: str("user_a"), key: str(goodKey), body: `{"amount":20000}`}), 404, "NOT_FOUND")
	}
	r.wantError(t, r.do("GET", "/v1/me/history", reqOpt{user: str("user_a")}), 404, "NOT_FOUND")
}

func TestPanicIsFixed500WithoutStack(t *testing.T) {
	r := newRig()
	r.fake.panicVal = "boom-secret-value"
	rec := r.do("POST", "/v1/payments", reqOpt{user: str("user_a"), key: str(goodKey), body: `{"amount":20000}`})
	r.wantError(t, rec, 500, "INTERNAL_ERROR")
	for name, text := range map[string]string{"body": rec.Body.String(), "log": r.logs.String()} {
		for _, bad := range []string{"goroutine", "panic(", "boom-secret-value", ".go:"} {
			if strings.Contains(text, bad) {
				t.Errorf("%s contains %q: %s", name, bad, text)
			}
		}
	}
	got := decodeEnvelope(t, rec).Error.Message
	if got != messages[codeInternal] {
		t.Errorf("panic message = %q, want fixed %q", got, messages[codeInternal])
	}
	e := newRig()
	e.fake.err = errors.New("driver text")
	svc := decodeEnvelope(t, e.do("POST", "/v1/payments", reqOpt{user: str("user_a"), key: str(goodKey), body: `{"amount":20000}`})).Error.Message
	if got != svc {
		t.Errorf("panic message %q differs from service-error message %q", got, svc)
	}
}

func TestRequestID(t *testing.T) {
	uuidShape := func(s string) bool { _, err := uuid.Parse(s); return err == nil }
	tests := []struct {
		name, sent string
		echoed     bool
	}{
		{"valid echoed", "abc-1", true},
		{"128 chars echoed", strings.Repeat("a", 128), true},
		{"absent generated", "", false},
		{"129 chars replaced", strings.Repeat("a", 129), false},
		{"bad characters replaced", "a b!", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig()
			rec := r.do("GET", "/v1/nope", reqOpt{reqID: tc.sent})
			got := rec.Header().Get("X-Request-ID")
			if tc.echoed && got != tc.sent {
				t.Fatalf("header = %q, want echo %q", got, tc.sent)
			}
			if !tc.echoed && !uuidShape(got) {
				t.Fatalf("header = %q, want a generated UUID", got)
			}
			if decodeEnvelope(t, rec).Error.RequestID != got {
				t.Fatal("request_id differs from header")
			}
		})
	}
}

func TestAccessLog(t *testing.T) {
	r := newRig()
	r.do("POST", "/v1/payments", reqOpt{user: str("user_a"), key: str(goodKey), body: `{"amount":20000}`, reqID: "rid-7"})
	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(r.logs.Bytes()), &line); err != nil {
		t.Fatalf("log is not one JSON line: %q: %v", r.logs.String(), err)
	}
	want := map[string]any{
		"request_id": "rid-7", "method": "POST", "route": "/v1/payments", "status": float64(201), "user_id": "user_a",
	}
	for k, v := range want {
		if line[k] != v {
			t.Errorf("log %s = %v, want %v", k, line[k], v)
		}
	}
	if _, ok := line["duration_ms"]; !ok {
		t.Error("log has no duration_ms")
	}
	if strings.Contains(r.logs.String(), "20000") {
		t.Error("log contains the body")
	}
}

func TestHistoryLimit(t *testing.T) {
	tests := []struct {
		query string
		limit int
		code  string
	}{
		{"", 20, ""},
		{"?limit=50", 50, ""},
		{"?limit=1", 1, ""},
		{"?limit=51", 0, "MALFORMED_REQUEST"},
		{"?limit=0", 0, "MALFORMED_REQUEST"},
		{"?limit=abc", 0, "MALFORMED_REQUEST"},
		{"?limit=+5", 0, "MALFORMED_REQUEST"},
		{"?limit=%zz", 0, "MALFORMED_REQUEST"},
		{"?limit=5;x=1", 0, "MALFORMED_REQUEST"},
		{"?limit=5&limit=%zz", 0, "MALFORMED_REQUEST"},
	}
	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			r := newRig()
			rec := r.do("GET", "/v1/me/history"+tc.query, reqOpt{user: str("user_a")})
			if tc.code != "" {
				r.wantError(t, rec, 400, tc.code)
				if len(r.fake.histCall) != 0 {
					t.Fatal("service called on a 4xx")
				}
				return
			}
			if rec.Code != 200 || len(r.fake.histCall) != 1 || r.fake.histCall[0] != (histArgs{"user_a", tc.limit}) {
				t.Fatalf("status %d calls %+v", rec.Code, r.fake.histCall)
			}
		})
	}
	r := newRig()
	r.wantError(t, r.do("GET", "/v1/me/history?limit=51", reqOpt{}), 400, "MISSING_USER")
}
