//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/httpapi"
	"github.com/riyanpratamap/flash-cashback/backend/internal/service"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// payRouter serves the reads and POST /payments through the real router.
func payRouter() http.Handler { return payRouterLog(io.Discard) }

// payRouterLog is payRouter writing its logs to w.
func payRouterLog(w io.Writer) http.Handler {
	log := slog.New(slog.NewJSONHandler(w, nil))
	tx := store.TxRunner{Pool: pool, LockTimeoutMS: 2000, StatementTimeoutMS: 5000}
	return httpapi.NewRouter(httpapi.Deps{
		PingPostgres: func(context.Context) error { return nil },
		PingRedis:    func(context.Context) error { return nil },
		Log:          log,
		Reads:        service.NewReads(pool),
		Payments:     service.NewPayments(tx, log),
	})
}

type payReply struct {
	status   int
	raw      string
	res      domain.PaymentResult
	replayed string // the Idempotent-Replayed header
}

// postPayment sends one POST /payments with the given headers and body.
func postPayment(t *testing.T, headers map[string]string, body string) payReply {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments", strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	payRouter().ServeHTTP(rec, req)
	out := payReply{status: rec.Code, raw: rec.Body.String(), replayed: rec.Header().Get("Idempotent-Replayed")}
	if rec.Code == http.StatusCreated || rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out.res); err != nil {
			t.Fatalf("payment body: %v: %s", err, rec.Body)
		}
	}
	return out
}

// pay sends a payment of amount for user with a new key.
func pay(t *testing.T, user string, amount int64) payReply {
	t.Helper()
	return postPayment(t, map[string]string{
		"X-User-ID": user, "Idempotency-Key": uuid.NewString(),
	}, `{"amount":`+itoa(amount)+`}`)
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func mustSetNow(t *testing.T, rfc3339 string) {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		t.Fatal(err)
	}
	setNow(t, ts)
}

func queryInt(t *testing.T, sql string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	return n
}

func TestPayFirstPaymentAC01(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:32:00Z")

	got := pay(t, "user_a", 100000)
	if got.status != http.StatusCreated {
		t.Fatalf("status = %d: %s", got.status, got.raw)
	}
	want := domain.PaymentResult{
		Payment: domain.PaymentView{
			ID: 1, Reference: "PAY-20261003-000001", Amount: 100000, Status: "SUCCEEDED",
			CreatedAt: "2026-10-03T14:32:00+07:00",
		},
		Cashback: domain.AwardView{Awarded: 5000, Reason: domain.ReasonAwarded},
	}
	if got.res != want {
		t.Errorf("result = %+v, want %+v", got.res, want)
	}

	_, m := getBody(t, "/v1/me/cashback", "user_a")
	cb := m["today"].(map[string]any)
	if m["balance"] != 5000.0 || cb["earned"] != 5000.0 || cb["remaining"] != 45000.0 {
		t.Errorf("cashback = %v, want balance 5000 earned 5000 remaining 45000", m)
	}
	if n := queryInt(t, `SELECT count(*) FROM ledger_entries WHERE kind = 'AWARD' AND amount = 5000
		AND payment_id = 1 AND balance_after = 5000 AND user_id = 'user_a'`); n != 1 {
		t.Errorf("matching AWARD ledger entries = %d, want 1", n)
	}
	if n := queryInt(t, `SELECT spent FROM campaigns`); n != 5000 {
		t.Errorf("spent = %d, want 5000", n)
	}
}

// assertBooks checks the invariants that hold after any run of payments:
// per user the ledger sums to the balance, spent equals the awards and stays
// within the budget, earned stays within the cap, and no balance is negative.
func assertBooks(t *testing.T) {
	t.Helper()
	checks := map[string]string{
		"ledger sum = balance": `SELECT count(*) FROM cashback_balances b
			WHERE b.balance <> COALESCE((SELECT sum(amount) FROM ledger_entries l WHERE l.user_id = b.user_id), 0)`,
		"spent = sum(awards)": `SELECT abs((SELECT spent FROM campaigns) -
			COALESCE((SELECT sum(cashback_awarded) FROM payments), 0))`,
		"spent <= budget": `SELECT count(*) FROM campaigns WHERE spent > budget`,
		"earned <= cap":   `SELECT count(*) FROM user_daily_earnings WHERE earned > daily_cap`,
		"balance >= 0":    `SELECT count(*) FROM cashback_balances WHERE balance < 0`,
		"earned = day's awards": `SELECT count(*) FROM user_daily_earnings u WHERE u.earned <>
			COALESCE((SELECT sum(cashback_awarded) FROM payments p WHERE p.user_id = u.user_id AND p.campaign_day = u.day), 0)`,
	}
	for name, sql := range checks {
		if n := queryInt(t, sql); n != 0 {
			t.Errorf("invariant %q broken: %d", name, n)
		}
	}
}

func campaignStatus(t *testing.T) string {
	t.Helper()
	_, m := getBody(t, "/v1/campaign", "user_new")
	return m["status"].(string)
}

func TestPayAwardTable(t *testing.T) {
	type tc struct {
		name    string
		budget  int64
		prior   []int64 // payments by user_a that build earned and spent
		pause   bool
		amount  int64
		award   int64
		reason  domain.Reason
		status  string
		earned  int64 // user_a's earned today after the payment
		spentAt int64 // spent after the payment
	}
	const big = 10_000_000
	cases := []tc{
		{"AC-02 19999 below minimum", big, nil, false, 19999, 0, domain.ReasonBelowMinimum, "ACTIVE", 0, 0},
		{"AC-03 20000", big, nil, false, 20000, 1000, domain.ReasonAwarded, "ACTIVE", 1000, 1000},
		{"AC-04 20001 rounds down", big, nil, false, 20001, 1000, domain.ReasonAwarded, "ACTIVE", 1000, 1000},
		{"AC-04 39999", big, nil, false, 39999, 1999, domain.ReasonAwarded, "ACTIVE", 1999, 1999},
		{"AC-04 10000000 hits the cap", big, nil, false, 10_000_000, 50000, domain.ReasonPartialDailyCap, "ACTIVE", 50000, 50000},
		{"AC-05 earned 45000", big, []int64{900000}, false, 100000, 5000, domain.ReasonAwarded, "ACTIVE", 50000, 50000},
		{"AC-06 earned 47000", big, []int64{940000}, false, 100000, 3000, domain.ReasonPartialDailyCap, "ACTIVE", 50000, 50000},
		{"AC-07 earned 50000", big, []int64{1_000_000}, false, 100000, 0, domain.ReasonDailyCapReached, "ACTIVE", 50000, 50000},
		{"AC-07 earned 50000, 19999", big, []int64{1_000_000}, false, 19999, 0, domain.ReasonBelowMinimum, "ACTIVE", 50000, 50000},
		{"AC-08 last 5000 of the budget", 5000, nil, false, 100000, 5000, domain.ReasonAwarded, "ENDED", 5000, 5000},
		{"AC-09 budget left 2000", 2000, nil, false, 100000, 2000, domain.ReasonPartialBudget, "ENDED", 2000, 2000},
		{"AC-10 cap 3000 budget 2000", 49000, []int64{940000}, false, 100000, 2000, domain.ReasonPartialBudget, "ENDED", 49000, 49000},
		{"AC-10 tie cap 2000 budget 2000", 50000, []int64{960000}, false, 100000, 2000, domain.ReasonPartialBudget, "ENDED", 50000, 50000},
		{"AC-10 cap 2000 budget 3000", 51000, []int64{960000}, false, 100000, 2000, domain.ReasonPartialDailyCap, "ACTIVE", 50000, 50000},
		{"AC-11 spent equals budget", 5000, []int64{100000}, false, 100000, 0, domain.ReasonCampaignEnded, "ENDED", 5000, 5000},
		{"AC-11 ended, 19999", 5000, []int64{100000}, false, 19999, 0, domain.ReasonBelowMinimum, "ENDED", 5000, 5000},
		{"AC-12 paused", big, nil, true, 100000, 0, domain.ReasonCampaignPaused, "PAUSED", 0, 0},
		{"AC-12 paused, earned 50000", big, []int64{1_000_000}, true, 100000, 0, domain.ReasonCampaignPaused, "PAUSED", 50000, 50000},
		{"AC-12 paused and ended", 5000, []int64{100000}, true, 100000, 0, domain.ReasonCampaignEnded, "ENDED", 5000, 5000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reset(t, c.budget)
			mustSetNow(t, "2026-10-03T07:00:00Z")
			for _, a := range c.prior {
				if r := pay(t, "user_a", a); r.status != http.StatusCreated {
					t.Fatalf("prior payment %d = %d: %s", a, r.status, r.raw)
				}
			}
			if c.pause {
				if _, err := pool.Exec(context.Background(), `UPDATE campaigns SET awards_paused = true`); err != nil {
					t.Fatal(err)
				}
			}
			ledgerBefore := queryInt(t, `SELECT count(*) FROM ledger_entries`)
			got := pay(t, "user_a", c.amount)
			if got.status != http.StatusCreated {
				t.Fatalf("status = %d: %s", got.status, got.raw)
			}
			if got.res.Cashback.Awarded != c.award || got.res.Cashback.Reason != c.reason {
				t.Errorf("cashback = %+v, want %d %s", got.res.Cashback, c.award, c.reason)
			}
			if got.res.Payment.Amount != c.amount || got.res.Payment.Status != "SUCCEEDED" {
				t.Errorf("payment = %+v", got.res.Payment)
			}
			if st := campaignStatus(t); st != c.status {
				t.Errorf("campaign status = %s, want %s", st, c.status)
			}
			_, m := getBody(t, "/v1/me/cashback", "user_a")
			today := m["today"].(map[string]any)
			if today["earned"] != float64(c.earned) || today["remaining"] != float64(50000-c.earned) {
				t.Errorf("today = %v, want earned %d", today, c.earned)
			}
			if n := queryInt(t, `SELECT spent FROM campaigns`); n != c.spentAt {
				t.Errorf("spent = %d, want %d", n, c.spentAt)
			}
			wantLedger := ledgerBefore
			if c.award > 0 {
				wantLedger++
			}
			if n := queryInt(t, `SELECT count(*) FROM ledger_entries`); n != wantLedger {
				t.Errorf("ledger rows = %d, want %d", n, wantLedger)
			}
			if n := queryInt(t, `SELECT count(*) FROM payments WHERE id = $1 AND cashback_awarded = $2`,
				got.res.Payment.ID, c.award); n != 1 {
				t.Errorf("payment row with the award missing")
			}
			assertBooks(t)
		})
	}
}

func TestPayUserWithNoRowsAndOthersUntouched(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	if r := pay(t, "user_a", 100000); r.status != http.StatusCreated {
		t.Fatal(r.raw)
	}
	// AC-48 reads: user_new sees none of user_a's state.
	_, a := getBody(t, "/v1/me/cashback", "user_a")
	_, n := getBody(t, "/v1/me/cashback", "user_new")
	if a["balance"] != 5000.0 {
		t.Errorf("user_a balance = %v", a["balance"])
	}
	nt := n["today"].(map[string]any)
	if n["balance"] != 0.0 || nt["earned"] != 0.0 || nt["remaining"] != 50000.0 {
		t.Errorf("user_new = %v, want zeros and 50000 remaining", n)
	}
	assertBooks(t)
}

// storedRule reads the rule snapshot stored with a payment.
func storedRule(t *testing.T, id int64) (rate, minPay, dayCap, awarded int64, reason string) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `SELECT rate_bps, min_payment, daily_cap, cashback_awarded,
		cashback_reason FROM payments WHERE id = $1`, id).Scan(&rate, &minPay, &dayCap, &awarded, &reason)
	if err != nil {
		t.Fatal(err)
	}
	return rate, minPay, dayCap, awarded, reason
}

func TestPaySecondUserIsScopedAC48(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	if r := pay(t, "user_a", 940000); r.status != http.StatusCreated {
		t.Fatal(r.raw)
	}
	b := pay(t, "user_b", 100000)
	if b.status != http.StatusCreated {
		t.Fatal(b.raw)
	}
	if b.res.Cashback.Awarded != 5000 || b.res.Cashback.Reason != domain.ReasonAwarded {
		t.Errorf("user_b cashback = %+v, want 5000 AWARDED", b.res.Cashback)
	}
	for user, want := range map[string][3]float64{
		"user_a": {47000, 3000, 47000}, // earned, remaining, balance
		"user_b": {5000, 45000, 5000},
	} {
		_, m := getBody(t, "/v1/me/cashback", user)
		today := m["today"].(map[string]any)
		if today["earned"] != want[0] || today["remaining"] != want[1] || m["balance"] != want[2] {
			t.Errorf("%s = %v, want earned %v remaining %v balance %v", user, m, want[0], want[1], want[2])
		}
	}
	for user, want := range map[string]int64{"user_a": 47000, "user_b": 5000} {
		if n := queryInt(t, `SELECT earned FROM user_daily_earnings WHERE user_id = $1`, user); n != want {
			t.Errorf("%s stored earned = %d, want %d", user, n, want)
		}
	}
	assertBooks(t)
}

func TestPayWritesMoneyLogLine(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	var buf bytes.Buffer
	req := httptest.NewRequest(http.MethodPost, "/v1/payments", strings.NewReader(`{"amount":100000}`))
	req.Header.Set("X-User-ID", "user_a")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	req.Header.Set("X-Request-ID", "rid-42")
	rec := httptest.NewRecorder()
	payRouterLog(&buf).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var money map[string]any
	for _, l := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var line map[string]any
		if err := json.Unmarshal(l, &line); err != nil {
			t.Fatalf("log line %q: %v", l, err)
		}
		if line["msg"] == "money write" {
			money = line
		}
	}
	if money == nil {
		t.Fatalf("no money write line in %s", buf.String())
	}
	want := map[string]any{
		"request_id": "rid-42", "op": "payment", "user_id": "user_a", "payment_id": 1.0,
		"amount": 100000.0, "awarded": 5000.0, "reason": "AWARDED", "replayed": false,
	}
	for k, v := range want {
		if money[k] != v {
			t.Errorf("log %s = %v, want %v", k, money[k], v)
		}
	}
}

func TestPayRuleSnapshotAC15(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	first := pay(t, "user_a", 940000)
	if first.status != http.StatusCreated {
		t.Fatal(first.raw)
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE campaigns SET rate_bps = 1000, min_payment = 50000, daily_cap = 70000`); err != nil {
		t.Fatal(err)
	}
	// The first payment keeps the rule it was decided under.
	rate, minPay, dayCap, awarded, reason := storedRule(t, first.res.Payment.ID)
	if rate != 500 || minPay != 20000 || dayCap != 50000 || awarded != 47000 || reason != "AWARDED" {
		t.Errorf("first stored = %d %d %d %d %s, want 500 20000 50000 47000 AWARDED", rate, minPay, dayCap, awarded, reason)
	}
	// A payment the same day is decided with the new rate and minimum but the
	// user-day cap, and stores exactly that.
	second := pay(t, "user_a", 100000)
	if second.status != http.StatusCreated {
		t.Fatal(second.raw)
	}
	rate, minPay, dayCap, awarded, reason = storedRule(t, second.res.Payment.ID)
	if rate != 1000 || minPay != 50000 || dayCap != 50000 || awarded != 3000 || reason != "PARTIAL_DAILY_CAP" {
		t.Errorf("second stored = %d %d %d %d %s, want 1000 50000 50000 3000 PARTIAL_DAILY_CAP",
			rate, minPay, dayCap, awarded, reason)
	}
	assertBooks(t)
}

func TestPayCapChangedMidDayAC71(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	if r := pay(t, "user_a", 940000); r.status != http.StatusCreated {
		t.Fatal(r.raw)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE campaigns SET daily_cap = 60000`); err != nil {
		t.Fatal(err)
	}
	got := pay(t, "user_a", 100000)
	if got.status != http.StatusCreated {
		t.Fatalf("status = %d: %s", got.status, got.raw)
	}
	if got.res.Cashback.Awarded != 3000 || got.res.Cashback.Reason != domain.ReasonPartialDailyCap {
		t.Errorf("cashback = %+v, want 3000 PARTIAL_DAILY_CAP", got.res.Cashback)
	}
	if _, _, dayCap, _, _ := storedRule(t, got.res.Payment.ID); dayCap != 50000 {
		t.Errorf("day 1 stored daily_cap = %d, want 50000", dayCap)
	}
	_, m := getBody(t, "/v1/me/cashback", "user_a")
	if rem := m["today"].(map[string]any)["remaining"]; rem != 0.0 {
		t.Errorf("remaining = %v, want 0", rem)
	}

	mustSetNow(t, "2026-10-04T07:00:00Z")
	next := pay(t, "user_a", 100000)
	if next.status != http.StatusCreated || next.res.Cashback.Awarded != 5000 {
		t.Fatalf("next day = %d %+v", next.status, next.res)
	}
	if _, _, dayCap, _, _ := storedRule(t, next.res.Payment.ID); dayCap != 60000 {
		t.Errorf("day 2 stored daily_cap = %d, want 60000", dayCap)
	}
	_, m = getBody(t, "/v1/me/cashback", "user_a")
	if rem := m["today"].(map[string]any)["remaining"]; rem != 55000.0 {
		t.Errorf("next-day remaining = %v, want 55000", rem)
	}
	assertBooks(t)
}

func TestPayInvalidInputWritesNothingAC17AC18(t *testing.T) {
	good := map[string]string{"X-User-ID": "user_a", "Idempotency-Key": "123e4567-e89b-12d3-a456-426614174000"}
	with := func(k, v string) map[string]string {
		h := map[string]string{}
		for kk, vv := range good {
			h[kk] = vv
		}
		if v == "" {
			delete(h, k)
		} else {
			h[k] = v
		}
		return h
	}
	type tc struct {
		name    string
		headers map[string]string
		body    string
		status  int
		code    string
	}
	cases := []tc{
		{"amount 0", good, `{"amount":0}`, 422, "INVALID_AMOUNT"},
		{"amount -1", good, `{"amount":-1}`, 422, "INVALID_AMOUNT"},
		{"amount too big", good, `{"amount":10000001}`, 422, "INVALID_AMOUNT"},
		{"amount 1.5", good, `{"amount":1.5}`, 422, "INVALID_AMOUNT"},
		{"amount 100000.0", good, `{"amount":100000.0}`, 422, "INVALID_AMOUNT"},
		{"amount 1e5", good, `{"amount":1e5}`, 422, "INVALID_AMOUNT"},
		{"amount string", good, `{"amount":"100000"}`, 400, "MALFORMED_REQUEST"},
		{"amount null", good, `{"amount":null}`, 400, "MALFORMED_REQUEST"},
		{"missing amount", good, `{}`, 400, "MALFORMED_REQUEST"},
		{"unknown field", good, `{"amount":100000,"x":1}`, 400, "MALFORMED_REQUEST"},
		{"empty body", good, ``, 400, "MALFORMED_REQUEST"},
		{"not json", good, `nope`, 400, "MALFORMED_REQUEST"},
		{"missing user", with("X-User-ID", ""), `{"amount":100000}`, 400, "MISSING_USER"},
		{"uppercase user", with("X-User-ID", "User_A"), `{"amount":100000}`, 400, "INVALID_USER"},
		{"user with space", with("X-User-ID", "a b"), `{"amount":100000}`, 400, "INVALID_USER"},
		{"user of 65 characters", with("X-User-ID", strings.Repeat("a", 65)), `{"amount":100000}`, 400, "INVALID_USER"},
		{"missing key", with("Idempotency-Key", ""), `{"amount":100000}`, 400, "MISSING_IDEMPOTENCY_KEY"},
		{"key abc", with("Idempotency-Key", "abc"), `{"amount":100000}`, 400, "INVALID_IDEMPOTENCY_KEY"},
		{"key without dashes", with("Idempotency-Key", "123e4567e89b12d3a456426614174000"), `{"amount":100000}`, 400, "INVALID_IDEMPOTENCY_KEY"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reset(t, 10_000_000)
			got := postPayment(t, c.headers, c.body)
			if got.status != c.status || !strings.Contains(got.raw, `"code":"`+c.code+`"`) {
				t.Errorf("got %d %s, want %d %s", got.status, got.raw, c.status, c.code)
			}
			for _, table := range []string{"payments", "ledger_entries", "user_daily_earnings", "cashback_balances"} {
				if n := queryInt(t, `SELECT count(*) FROM `+table); n != 0 {
					t.Errorf("%s has %d rows, want 0", table, n)
				}
			}
			if n := queryInt(t, `SELECT spent FROM campaigns`); n != 0 {
				t.Errorf("spent = %d, want 0", n)
			}
		})
	}
}

func TestCampaignStatusAfterPayments(t *testing.T) {
	for name, tc := range map[string]struct {
		budget                    int64
		pauseAwards, pauseRedeems bool
		status, redemption        string
	}{
		"AC-41 spent partial":              {10_000_000, false, false, "ACTIVE", "AVAILABLE"},
		"AC-41 spent partial, paused":      {10_000_000, true, false, "PAUSED", "AVAILABLE"},
		"AC-41 spent equal":                {5000, false, false, "ENDED", "AVAILABLE"},
		"AC-41 spent equal, both paused":   {5000, true, true, "ENDED", "PAUSED"},
		"AC-41 spent partial, redeem only": {10_000_000, false, true, "ACTIVE", "PAUSED"},
	} {
		t.Run(name, func(t *testing.T) {
			reset(t, tc.budget)
			if r := pay(t, "user_a", 100000); r.status != http.StatusCreated {
				t.Fatal(r.raw)
			}
			if _, err := pool.Exec(context.Background(),
				`UPDATE campaigns SET awards_paused = $1, redemptions_paused = $2`, tc.pauseAwards, tc.pauseRedeems); err != nil {
				t.Fatal(err)
			}
			_, m := getBody(t, "/v1/campaign", "user_a")
			if m["status"] != tc.status || m["redemption_status"] != tc.redemption {
				t.Errorf("status = %v / %v, want %s / %s", m["status"], m["redemption_status"], tc.status, tc.redemption)
			}
			noBudgetKey(t, m, "/v1/campaign ")
			_, c := getBody(t, "/v1/me/cashback", "user_a")
			noBudgetKey(t, c, "/v1/me/cashback ")
		})
	}
}
