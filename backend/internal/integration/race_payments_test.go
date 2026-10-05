//go:build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/riyanpratamap/flash-cashback/backend/internal/boot"
	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// raceConns is larger than the callers of any race test here, so the pool
// queue never decides who waits (tech-spec §11).
const raceConns = 60

// racePool is a dedicated pool of raceConns connections, all dialled before
// the race starts so connection setup does not stagger the requests.
func racePool(t *testing.T) http.Handler {
	t.Helper()
	ctx := context.Background()
	p, err := boot.Connect(ctx, testURL, raceConns, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	conns := make([]interface{ Release() }, 0, raceConns)
	for i := 0; i < raceConns; i++ {
		c, err := p.Acquire(ctx)
		if err != nil {
			t.Fatalf("warm connection %d: %v", i, err)
		}
		conns = append(conns, c)
	}
	for _, c := range conns {
		c.Release()
	}
	return payRouterOn(p, io.Discard)
}

type raceReq struct {
	user   string
	key    string // empty: a new UUID per request
	amount int64
}

type raceResult struct {
	reply payReply
	err   error
}

// payAllAtOnce sends every request from its own goroutine, released together.
// Nothing here holds a lock outside a request, so a failed run leaves no
// transaction to roll back. Goroutines only record; the test asserts.
func payAllAtOnce(h http.Handler, reqs []raceReq) []raceResult {
	out := make([]raceResult, len(reqs))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, r := range reqs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			key := r.key
			if key == "" {
				key = uuid.NewString()
			}
			out[i].reply, out[i].err = doPayment(h, map[string]string{
				"X-User-ID": r.user, "Idempotency-Key": key,
			}, `{"amount":`+itoa(r.amount)+`}`)
		}()
	}
	close(start)
	wg.Wait()
	return out
}

// reportNotCreated records, without stopping the test, every call that
// errored or answered other than 201: the count and the first few bodies. The
// caller still checks the tally and totals, so a red run shows them too.
func reportNotCreated(t *testing.T, res []raceResult) {
	t.Helper()
	const shown = 3
	bad := 0
	for i, r := range res {
		var msg string
		switch {
		case r.err != nil:
			msg = fmt.Sprintf("request %d: %v", i, r.err)
		case r.reply.status != http.StatusCreated:
			msg = fmt.Sprintf("request %d: status %d: %s", i, r.reply.status, r.reply.raw)
		default:
			continue
		}
		bad++
		if bad <= shown {
			t.Error(msg)
		}
	}
	if bad > 0 {
		t.Errorf("%d of %d requests were not 201", bad, len(res))
	}
}

type award struct {
	amount int64
	reason domain.Reason
}

func tally(res []raceResult) map[award]int {
	m := map[award]int{}
	for _, r := range res {
		if r.err != nil || r.reply.status != http.StatusCreated {
			continue
		}
		m[award{r.reply.res.Cashback.Awarded, r.reply.res.Cashback.Reason}]++
	}
	return m
}

func requireTally(t *testing.T, got, want map[award]int) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("awards = %v, want %v", got, want)
	}
}

// AC-25: 50 users pay at once against a budget of 12000.
func TestRacePayBudgetDrainAC25(t *testing.T) {
	reset(t, 12000)
	mustSetNow(t, "2026-10-03T07:32:00Z")
	h := racePool(t)

	reqs := make([]raceReq, 50)
	for i := range reqs {
		reqs[i] = raceReq{user: fmt.Sprintf("user_%02d", i), amount: 100000}
	}
	res := payAllAtOnce(h, reqs)

	reportNotCreated(t, res)
	requireTally(t, tally(res), map[award]int{
		{5000, domain.ReasonAwarded}:       2,
		{2000, domain.ReasonPartialBudget}: 1,
		{0, domain.ReasonCampaignEnded}:    47,
	})
	if n := queryInt(t, `SELECT spent FROM campaigns`); n != 12000 {
		t.Errorf("spent = %d, want 12000", n)
	}
	if n := queryInt(t, `SELECT count(*) FROM payments`); n != 50 {
		t.Errorf("payments = %d, want 50", n)
	}
	if n := queryInt(t, `SELECT COALESCE(sum(cashback_awarded), 0) FROM payments`); n != 12000 {
		t.Errorf("sum(cashback_awarded) = %d, want 12000", n)
	}
	assertReconciled(t)
}

// AC-26: one user pays 30 times at once against a daily cap of 50000.
func TestRacePayDailyCapAC26(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:32:00Z")
	h := racePool(t)

	reqs := make([]raceReq, 30)
	for i := range reqs {
		reqs[i] = raceReq{user: "user_a", amount: 120000}
	}
	res := payAllAtOnce(h, reqs)

	reportNotCreated(t, res)
	requireTally(t, tally(res), map[award]int{
		{6000, domain.ReasonAwarded}:         8,
		{2000, domain.ReasonPartialDailyCap}: 1,
		{0, domain.ReasonDailyCapReached}:    21,
	})
	if n := queryInt(t, `SELECT earned FROM user_daily_earnings WHERE user_id = 'user_a'`); n != 50000 {
		t.Errorf("earned = %d, want 50000", n)
	}
	if n := queryInt(t, `SELECT balance FROM cashback_balances WHERE user_id = 'user_a'`); n != 50000 {
		t.Errorf("balance = %d, want 50000", n)
	}
	if n := queryInt(t, `SELECT spent FROM campaigns`); n != 50000 {
		t.Errorf("spent = %d, want 50000", n)
	}
	if n := queryInt(t, `SELECT count(*) FROM payments`); n != 30 {
		t.Errorf("payments = %d, want 30", n)
	}
	assertReconciled(t)
}

// AC-23: 20 requests with one key and one body, at once. One creates the
// payment; the other nineteen replay it, byte for byte.
func TestRacePaySameKeyAC23(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:32:00Z")
	h := racePool(t)

	key := uuid.NewString()
	reqs := make([]raceReq, 20)
	for i := range reqs {
		reqs[i] = raceReq{user: "user_a", key: key, amount: 100000}
	}
	res := payAllAtOnce(h, reqs)

	created, createdRaw := 0, ""
	for i, r := range res {
		switch {
		case r.err != nil:
			t.Errorf("request %d: %v", i, r.err)
		case r.reply.status == http.StatusCreated:
			created++
			createdRaw = r.reply.raw
		}
	}
	if created != 1 {
		t.Errorf("%d requests answered 201, want 1", created)
	}
	bad := 0
	for i, r := range res {
		if r.err != nil || r.reply.status == http.StatusCreated {
			continue
		}
		if r.reply.status != http.StatusOK || r.reply.replayed != "true" || r.reply.raw != createdRaw {
			bad++
			if bad <= 3 {
				t.Errorf("request %d: status %d replayed=%q: %s, want 200 true %s",
					i, r.reply.status, r.reply.replayed, r.reply.raw, createdRaw)
			}
		}
	}
	if n := queryInt(t, `SELECT count(*) FROM payments`); n != 1 {
		t.Errorf("payments = %d, want 1", n)
	}
	if n := queryInt(t, `SELECT count(*) FROM ledger_entries`); n != 1 {
		t.Errorf("ledger entries = %d, want 1", n)
	}
	if n := queryInt(t, `SELECT balance FROM cashback_balances WHERE user_id = 'user_a'`); n != 5000 {
		t.Errorf("balance = %d, want 5000", n)
	}
	if n := queryInt(t, `SELECT spent FROM campaigns`); n != 5000 {
		t.Errorf("spent = %d, want 5000", n)
	}
	assertReconciled(t)
}

// AC-24: 20 requests with one key and two different bodies, at once. The
// body that wins decides the payment; the other body is a reused key.
func TestRacePaySameKeyTwoBodiesAC24(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:32:00Z")
	h := racePool(t)

	key := uuid.NewString()
	reqs := make([]raceReq, 20)
	for i := range reqs {
		amount := int64(100000)
		if i%2 == 1 {
			amount = 50000
		}
		reqs[i] = raceReq{user: "user_a", key: key, amount: amount}
	}
	res := payAllAtOnce(h, reqs)

	created, winAmount, winRaw := 0, int64(0), ""
	for i, r := range res {
		switch {
		case r.err != nil:
			t.Errorf("request %d: %v", i, r.err)
		case r.reply.status == http.StatusCreated:
			created++
			winAmount, winRaw = reqs[i].amount, r.reply.raw
		}
	}
	if created != 1 {
		t.Errorf("%d requests answered 201, want 1", created)
	}
	bad := 0
	for i, r := range res {
		if r.err != nil || r.reply.status == http.StatusCreated {
			continue
		}
		var ok bool
		if reqs[i].amount == winAmount {
			ok = r.reply.status == http.StatusOK && r.reply.replayed == "true" && r.reply.raw == winRaw
		} else {
			ok = r.reply.status == http.StatusConflict && r.reply.replayed != "true" &&
				strings.Contains(r.reply.raw, "IDEMPOTENCY_KEY_REUSED")
		}
		if !ok {
			bad++
			if bad <= 3 {
				t.Errorf("request %d (amount %d, winner %d): status %d replayed=%q: %s",
					i, reqs[i].amount, winAmount, r.reply.status, r.reply.replayed, r.reply.raw)
			}
		}
	}
	want := winAmount * 5 / 100
	if n := queryInt(t, `SELECT count(*) FROM payments`); n != 1 {
		t.Errorf("payments = %d, want 1", n)
	}
	if n := queryInt(t, `SELECT count(*) FROM ledger_entries`); n != 1 {
		t.Errorf("ledger entries = %d, want 1", n)
	}
	if n := queryInt(t, `SELECT balance FROM cashback_balances WHERE user_id = 'user_a'`); n != want {
		t.Errorf("balance = %d, want %d", n, want)
	}
	assertReconciled(t)
}

// holdCampaignLock opens a transaction that holds the campaign row lock. The
// returned release rolls it back once, whoever calls it first (pgx.Tx is not
// safe for concurrent use, so the Once guards the timer and the cleanup). pid
// is the holder's backend pid, read before any timer can touch the holder.
func holdCampaignLock(t *testing.T) (release func(), pid int64) {
	t.Helper()
	ctx := context.Background()
	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		_ = holder.Rollback(ctx)
		t.Fatal(err)
	}
	var once sync.Once
	release = func() { once.Do(func() { _ = holder.Rollback(ctx) }) }
	t.Cleanup(release)
	if _, err := holder.Exec(ctx,
		`SELECT 1 FROM campaigns WHERE id = 'flash-cashback' FOR NO KEY UPDATE`); err != nil {
		t.Fatal(err)
	}
	return release, pid
}

// AC-27: a payment that waits longer than the lock timeout answers 503
// SERVICE_BUSY quickly and writes nothing; the same key then succeeds.
func TestRacePayLockTimeoutAC27(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:32:00Z")
	h := payRouterTx(store.TxRunner{Pool: pool, LockTimeoutMS: 300, StatementTimeoutMS: 5000}, io.Discard)
	before := readCounts(t)

	release, _ := holdCampaignLock(t)
	// The hold ends with the response, or after 2 s if the lock timeout is
	// broken, so a mutation shows as a slow answer rather than a hang.
	timer := time.AfterFunc(2*time.Second, release)
	defer timer.Stop()

	key := uuid.NewString()
	headers := map[string]string{"X-User-ID": "user_a", "Idempotency-Key": key}
	start := time.Now()
	got, err := doPayment(h, headers, `{"amount":100000}`)
	elapsed := time.Since(start)
	release()
	if err != nil {
		t.Fatal(err)
	}
	if got.status != http.StatusServiceUnavailable || !strings.Contains(got.raw, "SERVICE_BUSY") {
		t.Errorf("status %d: %s, want 503 SERVICE_BUSY", got.status, got.raw)
	}
	if elapsed > time.Second {
		t.Errorf("answered after %v, want under 1s (lock timeout 300ms)", elapsed)
	}
	if after := readCounts(t); after != before {
		t.Errorf("counts changed: %+v -> %+v", before, after)
	}

	again := payKey(t, "user_a", key, 100000)
	if again.status != http.StatusCreated || again.replayed == "true" {
		t.Errorf("retry = %d replayed=%q: %s, want 201 not replayed", again.status, again.replayed, again.raw)
	}
	assertReconciled(t)
}

// waitFor polls cond every 10 ms until it holds or limit passes.
func waitFor(limit time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

func countInt(sql string, args ...any) (int64, error) {
	var n int64
	err := pool.QueryRow(context.Background(), sql, args...).Scan(&n)
	return n, err
}

// AC-70: the client hangs up while its payment waits for the campaign lock.
// The payment still commits, and the resend with the same key replays it.
func TestRacePayClientDisconnectAC70(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:32:00Z")
	srv := httptest.NewServer(payRouter())
	t.Cleanup(srv.Close)

	release, holderPid := holdCampaignLock(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	key := uuid.NewString()
	done := make(chan struct{})
	go func() {
		defer close(done)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/v1/payments",
			strings.NewReader(`{"amount":100000}`))
		if err != nil {
			return
		}
		req.Header.Set("X-User-ID", "user_a")
		req.Header.Set("Idempotency-Key", key)
		req.Header.Set("Content-Type", "application/json")
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}()

	var pollErr error
	waiting := waitFor(5*time.Second, func() bool {
		n, err := countInt(`SELECT count(*) FROM pg_stat_activity
			WHERE backend_type = 'client backend' AND $1::int = ANY(pg_blocking_pids(pid))`, holderPid)
		pollErr = err
		return err == nil && n > 0
	})
	if !waiting {
		release()
		cancel()
		<-done
		if pollErr != nil {
			t.Fatalf("poll query failed: %v", pollErr)
		}
		t.Fatal("the payment never waited on the campaign lock")
	}
	cancel()
	<-done
	time.Sleep(500 * time.Millisecond) // let the server see the disconnect
	release()

	waitFor(5*time.Second, func() bool {
		n, err := countInt(`SELECT count(*) FROM payments`)
		return err == nil && n == 1
	})
	if n := queryInt(t, `SELECT count(*) FROM payments`); n != 1 {
		t.Errorf("payments = %d, want 1 after the client hung up", n)
	}
	if n := queryInt(t, `SELECT count(*) FROM ledger_entries`); n != 1 {
		t.Errorf("ledger entries = %d, want 1", n)
	}
	// No row means no award: 0, so a red run still reaches reconcile.
	if n := queryInt(t, `SELECT COALESCE((SELECT balance FROM cashback_balances WHERE user_id = 'user_a'), 0)`); n != 5000 {
		t.Errorf("balance = %d, want 5000", n)
	}

	got := payKey(t, "user_a", key, 100000)
	if got.status != http.StatusOK || got.replayed != "true" {
		t.Errorf("resend = %d replayed=%q: %s, want 200 true", got.status, got.replayed, got.raw)
	}
	if got.status == http.StatusOK {
		stored := queryInt(t, `SELECT min(id) FROM payments`)
		r := got.res
		if r.Payment.ID != stored || r.Payment.Amount != 100000 || r.Cashback.Awarded != 5000 ||
			r.Cashback.Reason != domain.ReasonAwarded {
			t.Errorf("resend body = %s, want payment %d, amount 100000, awarded 5000 AWARDED", got.raw, stored)
		}
	}
	if n := queryInt(t, `SELECT count(*) FROM payments`); n != 1 {
		t.Errorf("payments = %d after the resend, want 1", n)
	}
	assertReconciled(t)
}
