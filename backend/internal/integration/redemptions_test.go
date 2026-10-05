//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

type redeemReply struct {
	status   int
	raw      string
	res      domain.RedemptionResult
	replayed string // the Idempotent-Replayed header
}

// doRedeem sends one POST /redemptions through h. It never touches a
// testing.T, so goroutines can call it.
func doRedeem(h http.Handler, headers map[string]string, body string) (redeemReply, error) {
	req := httptest.NewRequest(http.MethodPost, "/v1/redemptions", strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	out := redeemReply{status: rec.Code, raw: rec.Body.String(), replayed: rec.Header().Get("Idempotent-Replayed")}
	if rec.Code == http.StatusCreated || rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out.res); err != nil {
			return out, fmt.Errorf("redemption body: %w: %s", err, rec.Body)
		}
	}
	return out, nil
}

// redeemKey sends a redemption of amount for user under key.
func redeemKey(t *testing.T, user, key string, amount int64) redeemReply {
	t.Helper()
	out, err := doRedeem(payRouter(), map[string]string{"X-User-ID": user, "Idempotency-Key": key},
		`{"amount":`+itoa(amount)+`}`)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// redeem sends a redemption of amount for user under a new key.
func redeem(t *testing.T, user string, amount int64) redeemReply {
	t.Helper()
	return redeemKey(t, user, uuid.NewString(), amount)
}

// fund gives user a balance of exactly balance through a real payment:
// 5% of balance*20 is the balance, as long as it fits the daily cap.
func fund(t *testing.T, user string, balance int64) {
	t.Helper()
	if r := pay(t, user, balance*20); r.status != http.StatusCreated || r.res.Cashback.Awarded != balance {
		t.Fatalf("fund %s %d = %d: %s", user, balance, r.status, r.raw)
	}
}

func balanceOf(t *testing.T, user string) int64 {
	t.Helper()
	return queryInt(t, `SELECT COALESCE(sum(balance), 0) FROM cashback_balances WHERE user_id = $1`, user)
}

// redeemShape is every figure a redemption can change.
type redeemShape struct{ redemptions, ledger, balance, spent int64 }

func readRedeemShape(t *testing.T, user string) redeemShape {
	t.Helper()
	return redeemShape{
		redemptions: queryInt(t, `SELECT count(*) FROM redemptions`),
		ledger:      queryInt(t, `SELECT count(*) FROM ledger_entries`),
		balance:     balanceOf(t, user),
		spent:       queryInt(t, `SELECT spent FROM campaigns`),
	}
}

func TestRedeemWholeBalanceAC29(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:32:00Z")
	fund(t, "user_a", 18000)

	got := redeem(t, "user_a", 18000)
	if got.status != http.StatusCreated || got.replayed != "" {
		t.Fatalf("status = %d, replayed %q: %s", got.status, got.replayed, got.raw)
	}
	want := domain.RedemptionResult{
		Redemption: domain.RedemptionView{
			ID: 1, Reference: "RDM-20261003-000001", Amount: 18000, Status: "COMPLETED",
			Destination: "MAIN_ACCOUNT", CreatedAt: "2026-10-03T14:32:00+07:00",
		},
		BalanceAfter: 0,
	}
	if got.res != want {
		t.Errorf("result = %+v, want %+v", got.res, want)
	}
	if n := queryInt(t, `SELECT count(*) FROM ledger_entries WHERE kind = 'REDEMPTION' AND amount = -18000
		AND redemption_id = 1 AND balance_after = 0 AND user_id = 'user_a'`); n != 1 {
		t.Errorf("matching REDEMPTION ledger entries = %d, want 1", n)
	}
	if n := balanceOf(t, "user_a"); n != 0 {
		t.Errorf("balance = %d, want 0", n)
	}
	if n := queryInt(t, `SELECT spent FROM campaigns`); n != 18000 {
		t.Errorf("spent = %d, want 18000: a redemption does not spend budget", n)
	}
	if n := queryInt(t, `SELECT count(*) FROM redemptions WHERE balance_after = 0 AND status = 'COMPLETED'`); n != 1 {
		t.Errorf("stored redemptions with balance_after 0 = %d, want 1", n)
	}
	assertReconciled(t)
}

func TestRedeemAgainstBalanceAC16AC30AC31(t *testing.T) {
	type tc struct {
		name    string
		user    string
		amount  int64
		status  int
		code    string
		after   int64 // balance_after on 201
		balance int64 // the user's balance afterwards
	}
	cases := []tc{
		{"AC-30 one rupiah below the balance", "user_a", 17999, 201, "", 1, 1},
		{"AC-30 whole balance", "user_a", 18000, 201, "", 0, 0},
		{"AC-30 one rupiah", "user_a", 1, 201, "", 17999, 17999},
		{"AC-31 one above the balance", "user_a", 18001, 422, "INSUFFICIENT_BALANCE", 0, 18000},
		{"AC-16 user with no rows redeems 1", "user_new", 1, 422, "INSUFFICIENT_BALANCE", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reset(t, 10_000_000)
			mustSetNow(t, "2026-10-03T07:00:00Z")
			fund(t, "user_a", 18000)
			before := readRedeemShape(t, c.user)

			got := redeem(t, c.user, c.amount)
			if got.status != c.status {
				t.Fatalf("status = %d, want %d: %s", got.status, c.status, got.raw)
			}
			after := readRedeemShape(t, c.user)
			if c.status == 201 {
				if got.res.BalanceAfter != c.after || got.res.Redemption.Amount != c.amount {
					t.Errorf("result = %+v, want amount %d balance_after %d", got.res, c.amount, c.after)
				}
				before.redemptions++
				before.ledger++
			} else if !strings.Contains(got.raw, `"code":"`+c.code+`"`) {
				t.Errorf("body = %s, want code %s", got.raw, c.code)
			}
			before.balance = c.balance
			if after != before {
				t.Errorf("shape = %+v, want %+v", after, before)
			}
			assertReconciled(t)
		})
	}
}

func TestRedeemWhilePausedAC32(t *testing.T) {
	type tc struct {
		name    string
		user    string
		body    string
		status  int
		code    string
		balance int64
	}
	cases := []tc{
		{"amount 0 is invalid before the switch", "user_a", `{"amount":0}`, 422, "INVALID_AMOUNT", 18000},
		{"balance 0, amount 1 is paused before the balance check", "user_new", `{"amount":1}`, 409, "REDEMPTION_PAUSED", 0},
		{"balance enough is paused", "user_a", `{"amount":18000}`, 409, "REDEMPTION_PAUSED", 18000},
		{"balance short is paused", "user_a", `{"amount":18001}`, 409, "REDEMPTION_PAUSED", 18000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reset(t, 10_000_000)
			mustSetNow(t, "2026-10-03T07:00:00Z")
			fund(t, "user_a", 18000)
			execSQL(t, `UPDATE campaigns SET redemptions_paused = true`)
			before := readRedeemShape(t, c.user)

			got, err := doRedeem(payRouter(), map[string]string{"X-User-ID": c.user, "Idempotency-Key": uuid.NewString()}, c.body)
			if err != nil {
				t.Fatal(err)
			}
			if got.status != c.status || !strings.Contains(got.raw, `"code":"`+c.code+`"`) {
				t.Errorf("got %d %s, want %d %s", got.status, got.raw, c.status, c.code)
			}
			if after := readRedeemShape(t, c.user); after != before {
				t.Errorf("shape = %+v, want %+v", after, before)
			}
			assertReconciled(t)
		})
	}
}

func TestRedeemAfterCampaignEndedAC34(t *testing.T) {
	reset(t, 18000) // the one award takes the whole budget
	mustSetNow(t, "2026-10-03T07:00:00Z")
	fund(t, "user_a", 18000)
	if st := campaignStatus(t); st != "ENDED" {
		t.Fatalf("campaign status = %s, want ENDED", st)
	}
	got := redeem(t, "user_a", 18000)
	if got.status != http.StatusCreated || got.res.BalanceAfter != 0 {
		t.Fatalf("got %d %s, want 201 with balance_after 0", got.status, got.raw)
	}
	assertReconciled(t)
}

func TestRedeemReplayAC35(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:32:00Z")
	fund(t, "user_a", 18000)
	key := uuid.NewString()

	first := redeemKey(t, "user_a", key, 18000)
	if first.status != http.StatusCreated || first.res.BalanceAfter != 0 {
		t.Fatalf("first = %d: %s", first.status, first.raw)
	}
	if r := pay(t, "user_a", 100000); r.status != 201 { // balance is now 5000, not 0
		t.Fatalf("earn = %d: %s", r.status, r.raw)
	}
	before := readRedeemShape(t, "user_a")

	second := redeemKey(t, "user_a", key, 18000)
	if second.status != http.StatusOK || second.replayed != "true" {
		t.Fatalf("replay = %d, replayed %q: %s", second.status, second.replayed, second.raw)
	}
	if second.raw != first.raw {
		t.Errorf("replay body differs:\nfirst  %s\nsecond %s", first.raw, second.raw)
	}
	if second.res.BalanceAfter != 0 {
		t.Errorf("replayed balance_after = %d, want the stored 0, not the current balance", second.res.BalanceAfter)
	}
	if after := readRedeemShape(t, "user_a"); after != before {
		t.Errorf("replay changed rows: %+v -> %+v", before, after)
	}

	// A replay is answered even while redemptions are paused.
	execSQL(t, `UPDATE campaigns SET redemptions_paused = true`)
	if r := redeemKey(t, "user_a", key, 18000); r.status != http.StatusOK || r.raw != first.raw {
		t.Errorf("replay while paused = %d %s, want 200 and the first body", r.status, r.raw)
	}
	// The same key with another amount is rejected, paused or not.
	other := redeemKey(t, "user_a", key, 5000)
	if other.status != http.StatusConflict || !strings.Contains(other.raw, `"code":"IDEMPOTENCY_KEY_REUSED"`) || other.replayed != "" {
		t.Errorf("other body = %d %s, want 409 IDEMPOTENCY_KEY_REUSED", other.status, other.raw)
	}
	if after := readRedeemShape(t, "user_a"); after != before {
		t.Errorf("rejected key changed rows: %+v -> %+v", before, after)
	}
	assertReconciled(t)
}

// AC-21: the idempotency key of a payment is free for a redemption, because
// the two operations keep separate key spaces.
func TestRedeemWithPaymentKeyAC21(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	key := uuid.NewString()
	if r := payKey(t, "user_a", key, 360000); r.status != http.StatusCreated || r.res.Cashback.Awarded != 18000 {
		t.Fatalf("payment = %d: %s", r.status, r.raw)
	}
	got := redeemKey(t, "user_a", key, 18000)
	if got.status != http.StatusCreated || got.replayed != "" {
		t.Fatalf("redeem with payment key = %d, replayed %q: %s", got.status, got.replayed, got.raw)
	}
	if again := payKey(t, "user_a", key, 360000); again.status != http.StatusOK || again.replayed != "true" {
		t.Errorf("payment resend = %d, replayed %q, want 200 true", again.status, again.replayed)
	}
	assertReconciled(t)
}

// AC-48: the balance redeemed is the caller's own.
func TestRedeemIsScopedToCallerAC48(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	fund(t, "user_a", 18000)
	before := readRedeemShape(t, "user_a")

	got := redeem(t, "user_b", 1)
	if got.status != http.StatusUnprocessableEntity || !strings.Contains(got.raw, `"code":"INSUFFICIENT_BALANCE"`) {
		t.Errorf("user_b = %d %s, want 422 INSUFFICIENT_BALANCE", got.status, got.raw)
	}
	if after := readRedeemShape(t, "user_a"); after != before {
		t.Errorf("user_a changed: %+v -> %+v", before, after)
	}
	assertReconciled(t)
}

func TestRedeemInvalidInputWritesNothingAC17AC18(t *testing.T) {
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
		{"amount 1e5", good, `{"amount":1e5}`, 422, "INVALID_AMOUNT"},
		{"amount string", good, `{"amount":"100"}`, 400, "MALFORMED_REQUEST"},
		{"amount null", good, `{"amount":null}`, 400, "MALFORMED_REQUEST"},
		{"missing amount", good, `{}`, 400, "MALFORMED_REQUEST"},
		{"unknown field", good, `{"amount":100,"x":1}`, 400, "MALFORMED_REQUEST"},
		{"empty body", good, ``, 400, "MALFORMED_REQUEST"},
		{"missing user", with("X-User-ID", ""), `{"amount":100}`, 400, "MISSING_USER"},
		{"uppercase user", with("X-User-ID", "User_A"), `{"amount":100}`, 400, "INVALID_USER"},
		{"missing key", with("Idempotency-Key", ""), `{"amount":100}`, 400, "MISSING_IDEMPOTENCY_KEY"},
		{"key abc", with("Idempotency-Key", "abc"), `{"amount":100}`, 400, "INVALID_IDEMPOTENCY_KEY"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reset(t, 10_000_000)
			mustSetNow(t, "2026-10-03T07:00:00Z")
			fund(t, "user_a", 18000)
			before := readRedeemShape(t, "user_a")
			got, err := doRedeem(payRouter(), c.headers, c.body)
			if err != nil {
				t.Fatal(err)
			}
			if got.status != c.status || !strings.Contains(got.raw, `"code":"`+c.code+`"`) {
				t.Errorf("got %d %s, want %d %s", got.status, got.raw, c.status, c.code)
			}
			if after := readRedeemShape(t, "user_a"); after != before {
				t.Errorf("shape = %+v, want %+v", after, before)
			}
			assertReconciled(t)
		})
	}
}

// Step 5: with the fast path (step 2) in front, an HTTP resend never reaches
// the lookup under the balance lock, so it is tested directly. Without it the
// attempt would fail the balance check (the balance is 0 after the first).
func TestRedeemStepFiveLookupFindsCommittedRedemption(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	fund(t, "user_a", 18000)
	key := uuid.New()
	first := redeemKey(t, "user_a", key.String(), 18000)
	if first.status != http.StatusCreated {
		t.Fatalf("first = %d: %s", first.status, first.raw)
	}
	before := readRedeemShape(t, "user_a")

	runner := store.TxRunner{Pool: pool, LockTimeoutMS: 2000, StatementTimeoutMS: 5000}
	var out store.RedeemOutcome
	var redemptionsInTx int64
	err := runner.InTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = store.Redeem(ctx, tx, store.RedeemInput{
			CampaignID: "flash-cashback", User: "user_a", Key: key,
			Hash: domain.RequestHash(18000), Amount: 18000,
		})
		// Step 5 returns before any write; step 10 would show a second row.
		if qerr := tx.QueryRow(ctx, `SELECT count(*) FROM redemptions`).Scan(&redemptionsInTx); qerr != nil {
			return qerr
		}
		return err
	})
	if !errors.Is(err, store.ErrReplay) || out.Replay == nil {
		t.Fatalf("err = %v, replay = %v, want ErrReplay with the stored row", err, out.Replay)
	}
	if out.Replay.ID != first.res.Redemption.ID || out.Replay.Amount != 18000 || out.Replay.BalanceAfter != 0 {
		t.Errorf("stored = %+v, want redemption %d amount 18000 balance_after 0", out.Replay, first.res.Redemption.ID)
	}
	if redemptionsInTx != before.redemptions {
		t.Errorf("redemptions in the failed attempt = %d, want %d: it wrote before replaying", redemptionsInTx, before.redemptions)
	}
	if after := readRedeemShape(t, "user_a"); after != before {
		t.Errorf("shape changed: %+v -> %+v", before, after)
	}
	assertReconciled(t)
}

func TestRedeemWritesMoneyLogLine(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	fund(t, "user_a", 18000)
	var buf bytes.Buffer
	h := payRouterLog(&buf)
	key := uuid.NewString()
	for i, replayed := range []bool{false, true} {
		buf.Reset()
		req := httptest.NewRequest(http.MethodPost, "/v1/redemptions", strings.NewReader(`{"amount":5000}`))
		req.Header.Set("X-User-ID", "user_a")
		req.Header.Set("Idempotency-Key", key)
		req.Header.Set("X-Request-ID", "rid-42")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if want := []int{201, 200}[i]; rec.Code != want {
			t.Fatalf("call %d status = %d: %s", i, rec.Code, rec.Body)
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
			t.Fatalf("call %d: no money write line in %s", i, buf.String())
		}
		want := map[string]any{
			"request_id": "rid-42", "op": "redeem", "user_id": "user_a", "redemption_id": 1.0,
			"amount": 5000.0, "replayed": replayed,
		}
		for k, v := range want {
			if money[k] != v {
				t.Errorf("call %d log %s = %v, want %v", i, k, money[k], v)
			}
		}
	}
	assertReconciled(t)
}
