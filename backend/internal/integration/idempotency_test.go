//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// payKey sends a payment of amount for user under key.
func payKey(t *testing.T, user, key string, amount int64) payReply {
	t.Helper()
	return postPayment(t, map[string]string{"X-User-ID": user, "Idempotency-Key": key},
		`{"amount":`+itoa(amount)+`}`)
}

// counts is every row count and total a payment can change.
type counts struct{ payments, ledger, spent, earned, balance int64 }

func readCounts(t *testing.T) counts {
	t.Helper()
	return counts{
		payments: queryInt(t, `SELECT count(*) FROM payments`),
		ledger:   queryInt(t, `SELECT count(*) FROM ledger_entries`),
		spent:    queryInt(t, `SELECT spent FROM campaigns`),
		earned:   queryInt(t, `SELECT COALESCE(sum(earned), 0) FROM user_daily_earnings`),
		balance:  queryInt(t, `SELECT COALESCE(sum(balance), 0) FROM cashback_balances`),
	}
}

func TestPayReplayAC19(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:32:00Z")
	key := uuid.NewString()

	first := payKey(t, "user_a", key, 100000)
	if first.status != http.StatusCreated || first.replayed != "" {
		t.Fatalf("first = %d replayed=%q: %s", first.status, first.replayed, first.raw)
	}
	before := readCounts(t)

	second := payKey(t, "user_a", key, 100000)
	if second.status != http.StatusOK || second.replayed != "true" {
		t.Fatalf("resend = %d replayed=%q: %s", second.status, second.replayed, second.raw)
	}
	if second.raw != first.raw {
		t.Errorf("replay body = %s, want %s", second.raw, first.raw)
	}
	if after := readCounts(t); after != before {
		t.Errorf("counts changed on replay: %+v -> %+v", before, after)
	}
	assertBooks(t)
}

func TestPayKeyReusedAC20(t *testing.T) {
	for _, tc := range []struct {
		name   string
		first  int64
		awards int64
	}{
		{"awarded original", 100000, 5000},
		{"zero-award original", 19999, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reset(t, 10_000_000)
			key := uuid.NewString()
			if got := payKey(t, "user_a", key, tc.first); got.status != http.StatusCreated || got.res.Cashback.Awarded != tc.awards {
				t.Fatalf("first = %d: %s", got.status, got.raw)
			}
			before := readCounts(t)

			got := payKey(t, "user_a", key, 50000)
			if got.status != http.StatusConflict || got.replayed != "" {
				t.Fatalf("reuse = %d replayed=%q: %s", got.status, got.replayed, got.raw)
			}
			if !strings.Contains(got.raw, `"code":"IDEMPOTENCY_KEY_REUSED"`) {
				t.Errorf("body = %s, want code IDEMPOTENCY_KEY_REUSED", got.raw)
			}
			if after := readCounts(t); after != before {
				t.Errorf("counts changed on 409: %+v -> %+v", before, after)
			}
			assertBooks(t)
		})
	}
}

func TestPayReplayAfterPauseAndEndAC19(t *testing.T) {
	t.Run("after awards paused", func(t *testing.T) {
		reset(t, 10_000_000)
		key := uuid.NewString()
		first := payKey(t, "user_a", key, 100000)
		if _, err := pool.Exec(context.Background(), `UPDATE campaigns SET awards_paused = true`); err != nil {
			t.Fatal(err)
		}
		before := readCounts(t)
		got := payKey(t, "user_a", key, 100000)
		if got.status != http.StatusOK || got.replayed != "true" || got.raw != first.raw || got.res.Cashback.Awarded != 5000 {
			t.Errorf("replay = %d replayed=%q: %s, want 200 true %s", got.status, got.replayed, got.raw, first.raw)
		}
		if after := readCounts(t); after != before {
			t.Errorf("counts changed: %+v -> %+v", before, after)
		}
		assertBooks(t)
	})
	t.Run("after campaign ended", func(t *testing.T) {
		reset(t, 10000)
		key := uuid.NewString()
		first := payKey(t, "user_a", key, 100000)
		if got := pay(t, "user_b", 100000); got.res.Cashback.Awarded != 5000 {
			t.Fatalf("draining payment = %s", got.raw)
		}
		if s := campaignStatus(t); s != "ENDED" {
			t.Fatalf("status = %s, want ENDED", s)
		}
		before := readCounts(t)
		got := payKey(t, "user_a", key, 100000)
		if got.status != http.StatusOK || got.replayed != "true" || got.raw != first.raw || got.res.Cashback.Awarded != 5000 {
			t.Errorf("replay = %d replayed=%q: %s, want 200 true %s", got.status, got.replayed, got.raw, first.raw)
		}
		if after := readCounts(t); after != before {
			t.Errorf("counts changed: %+v -> %+v", before, after)
		}
		assertBooks(t)
	})
}

func TestPayKeyIsPerUserAC21(t *testing.T) {
	reset(t, 10_000_000)
	key := uuid.NewString()
	a := payKey(t, "user_a", key, 100000)
	b := payKey(t, "user_b", key, 100000)
	if a.status != http.StatusCreated || b.status != http.StatusCreated || b.replayed != "" {
		t.Fatalf("user_a = %d, user_b = %d replayed=%q: %s", a.status, b.status, b.replayed, b.raw)
	}
	if a.res.Payment.ID == b.res.Payment.ID {
		t.Errorf("both users got payment %d", a.res.Payment.ID)
	}
	if n := queryInt(t, `SELECT count(*) FROM payments WHERE idempotency_key = $1`, key); n != 2 {
		t.Errorf("payments for the key = %d, want 2", n)
	}
	assertBooks(t)
}

func TestPayRejectedKeyIsNotStoredAC22(t *testing.T) {
	reset(t, 10_000_000)
	key := uuid.NewString()
	if got := payKey(t, "user_a", key, 0); got.status != http.StatusUnprocessableEntity {
		t.Fatalf("amount 0 = %d: %s", got.status, got.raw)
	}
	got := payKey(t, "user_a", key, 100000)
	if got.status != http.StatusCreated || got.replayed != "" {
		t.Errorf("after 422 = %d replayed=%q: %s, want 201", got.status, got.replayed, got.raw)
	}
	assertBooks(t)
}

// Step 5: a payment committed before the transaction takes the campaign lock
// is found under that lock, and nothing the attempt wrote survives.
func TestPayStepFiveLookupFindsCommittedPayment(t *testing.T) {
	reset(t, 10_000_000)
	key := uuid.New()
	first := payKey(t, "user_a", key.String(), 100000)
	before := readCounts(t)

	runner := store.TxRunner{Pool: pool, LockTimeoutMS: 2000, StatementTimeoutMS: 5000}
	var out store.PayOutcome
	var spentInTx, earnedInTx int64
	err := runner.InTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = store.Pay(ctx, tx, store.PayInput{
			CampaignID: "flash-cashback", User: "user_a", Key: key,
			Hash: domain.RequestHash(100000), Amount: 100000,
		})
		// Step 5 returns before any write; step 9 would show the award added.
		if qerr := tx.QueryRow(ctx, `SELECT spent FROM campaigns`).Scan(&spentInTx); qerr != nil {
			return qerr
		}
		if qerr := tx.QueryRow(ctx, `SELECT COALESCE(sum(earned), 0) FROM user_daily_earnings`).Scan(&earnedInTx); qerr != nil {
			return qerr
		}
		return err
	})
	if !errors.Is(err, store.ErrReplay) || out.Replay == nil {
		t.Fatalf("err = %v, replay = %v, want ErrReplay with the stored row", err, out.Replay)
	}
	if out.Replay.ID != first.res.Payment.ID || out.Replay.Awarded != 5000 || out.Replay.Amount != 100000 {
		t.Errorf("stored = %+v, want payment %d awarded 5000", out.Replay, first.res.Payment.ID)
	}
	if spentInTx != before.spent || earnedInTx != before.earned {
		t.Errorf("in the failed attempt spent = %d, earned = %d, want %d and %d: it wrote before replaying",
			spentInTx, earnedInTx, before.spent, before.earned)
	}
	if after := readCounts(t); after != before {
		t.Errorf("counts changed: %+v -> %+v", before, after)
	}
	assertBooks(t)
}

// Step 2: a committed payment is answered without the campaign lock. A
// transaction holds that lock; the resend must not wait for it.
func TestPayFastReplayTakesNoCampaignLock(t *testing.T) {
	reset(t, 10_000_000)
	key := uuid.NewString()
	first := payKey(t, "user_a", key, 100000)
	if first.status != http.StatusCreated {
		t.Fatalf("first = %d: %s", first.status, first.raw)
	}

	holder, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = holder.Rollback(context.Background()) })
	if _, err := holder.Exec(context.Background(),
		`SELECT 1 FROM campaigns WHERE id = 'flash-cashback' FOR NO KEY UPDATE`); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	got := payKey(t, "user_a", key, 100000)
	elapsed := time.Since(start)
	if got.status != http.StatusOK || got.replayed != "true" || got.raw != first.raw {
		t.Fatalf("resend = %d replayed=%q: %s, want 200 true %s", got.status, got.replayed, got.raw, first.raw)
	}
	if elapsed > time.Second {
		t.Errorf("resend took %v while the campaign lock was held, want it to skip the lock (timeout is 2s)", elapsed)
	}
}

// moneyLines is every "money write" log line in buf.
func moneyLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var line map[string]any
		if err := json.Unmarshal(l, &line); err != nil {
			t.Fatalf("log line %q: %v", l, err)
		}
		if line["msg"] == "money write" {
			out = append(out, line)
		}
	}
	return out
}

func TestPayReplayMoneyLog(t *testing.T) {
	reset(t, 10_000_000)
	key := uuid.NewString()
	first := payKey(t, "user_a", key, 100000)

	send := func(buf *bytes.Buffer, amount string) int {
		req := httptest.NewRequest(http.MethodPost, "/v1/payments", strings.NewReader(`{"amount":`+amount+`}`))
		req.Header.Set("X-User-ID", "user_a")
		req.Header.Set("Idempotency-Key", key)
		rec := httptest.NewRecorder()
		payRouterLog(buf).ServeHTTP(rec, req)
		return rec.Code
	}

	var replayBuf bytes.Buffer
	if code := send(&replayBuf, "100000"); code != http.StatusOK {
		t.Fatalf("resend = %d", code)
	}
	lines := moneyLines(t, &replayBuf)
	if len(lines) != 1 {
		t.Fatalf("replay wrote %d money lines, want 1: %s", len(lines), replayBuf.String())
	}
	if lines[0]["replayed"] != true || lines[0]["payment_id"] != float64(first.res.Payment.ID) {
		t.Errorf("replay line = %v, want replayed true and payment_id %d", lines[0], first.res.Payment.ID)
	}

	var conflictBuf bytes.Buffer
	if code := send(&conflictBuf, "50000"); code != http.StatusConflict {
		t.Fatalf("reuse = %d", code)
	}
	if lines := moneyLines(t, &conflictBuf); len(lines) != 0 {
		t.Errorf("409 wrote money lines: %v", lines)
	}
}
