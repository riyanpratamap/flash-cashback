//go:build integration

package integration

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// hookedRouter serves the API over a warmed pool whose money transactions
// call hooks (tech-spec §11).
func hookedRouter(t *testing.T, hooks store.Hooks) http.Handler {
	t.Helper()
	return payRouterTx(store.TxRunner{
		Pool: warmedPool(t), LockTimeoutMS: 5000, StatementTimeoutMS: 8000, Hooks: hooks,
	}, nopWriter{})
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

// gate is a pause point that opens once, by close or by cleanup.
type gate struct {
	ch   chan struct{}
	once sync.Once
}

func newGate(t *testing.T) *gate {
	g := &gate{ch: make(chan struct{})}
	t.Cleanup(g.open)
	return g
}

func (g *gate) open() { g.once.Do(func() { close(g.ch) }) }

// wait blocks until the gate opens or limit passes.
func (g *gate) wait(limit time.Duration) {
	select {
	case <-g.ch:
	case <-time.After(limit):
	}
}

// dbClock reads the database wall clock.
func dbClock(t *testing.T) time.Time {
	t.Helper()
	var ts time.Time
	if err := pool.QueryRow(context.Background(), `SELECT clock_timestamp()`).Scan(&ts); err != nil {
		t.Fatal(err)
	}
	return ts
}

type adminRun struct {
	code     int
	out, err string
}

func runAdminAsync(t *testing.T, lockMS int64, args ...string) <-chan adminRun {
	done := make(chan adminRun, 1)
	go func() {
		code, out, errOut := runAdmin(t, lockMS, args...)
		done <- adminRun{code, out, errOut}
	}()
	return done
}

// AC-38: a pause that lands while payments are in flight. The first payment
// to read the campaign row is held after the read; the pause waits for its
// lock. Every payment that began after the command returned is paused, and
// no payment decided on a stale "not paused" lands after a paused one.
func TestRacePauseAwardsAmongPaymentsAC38(t *testing.T) {
	reset(t, 10_000_000)
	g := newGate(t)
	var calls atomic.Int64
	h := hookedRouter(t, store.Hooks{AfterCampaignRead: func() {
		if calls.Add(1) == 1 {
			g.wait(5 * time.Second) // fallback; the test opens the gate once the pause waits
		}
	}})

	reqs := func(from int) []raceReq {
		out := make([]raceReq, 20)
		for i := range out {
			out[i] = raceReq{user: "user_" + itoa(int64(from+i)), amount: 100000}
		}
		return out
	}
	waveA := make(chan []raceResult, 1)
	go func() { waveA <- payAllAtOnce(h, reqs(0)) }()
	if !waitFor(5*time.Second, func() bool { return calls.Load() >= 1 }) {
		t.Fatal("no payment reached the hook")
	}
	pause := runAdminAsync(t, 0, "pause-awards", "--by", "owner")
	// The pause waits behind a chain of lock waiters that ends at a backend
	// idle in its transaction: the held payment. A payment between two
	// statements looks the same for an instant, so the sight must last 150 ms.
	waitingChain := func() bool {
		n, err := countInt(`WITH RECURSIVE chain(pid) AS (
				SELECT unnest(pg_blocking_pids(pid)) FROM pg_stat_activity
				WHERE backend_type = 'client backend' AND query LIKE 'SELECT awards_paused, now()%'
				UNION
				SELECT unnest(pg_blocking_pids(chain.pid)) FROM chain)
			SELECT count(*) FROM pg_stat_activity a JOIN chain USING (pid) WHERE a.state = 'idle in transaction'`)
		return err == nil && n >= 1
	}
	sawWaiting := waitFor(time.Second, func() bool {
		if !waitingChain() {
			return false
		}
		time.Sleep(150 * time.Millisecond)
		return waitingChain()
	})
	if sawWaiting {
		g.open() // the pause queued behind the held payment; let it commit
	}
	cmd := <-pause
	after := dbClock(t)
	waveB := payAllAtOnce(h, reqs(20))
	g.open() // no-op when open; releases the held payment after wave B if the pause never waited
	resA := <-waveA

	all := append(resA, waveB...)
	reportNotCreated(t, all)
	if !sawWaiting {
		t.Error("pause-awards was never seen waiting behind the held payment")
	}
	if cmd.code != 0 || cmd.err != "" {
		t.Fatalf("pause-awards = exit %d, stderr %q", cmd.code, cmd.err)
	}
	if line := parseActionLine(t, cmd.out); line["changed"] != true {
		t.Errorf("line = %v, want changed true", line)
	}
	if len(all) != 40 {
		t.Fatalf("%d replies, want 40", len(all))
	}

	rows, err := pool.Query(context.Background(), `SELECT cashback_reason FROM payments ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var reasons []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			t.Fatal(err)
		}
		reasons = append(reasons, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	paused := string(domain.ReasonCampaignPaused)
	first := slices.Index(reasons, paused)
	if first <= 0 {
		t.Fatalf("reasons by id = %v, want awarded rows then paused rows", reasons)
	}
	for i, r := range reasons[first:] {
		if r != paused {
			t.Errorf("payment %d (id order) is %s after a paused one; reasons = %v", first+i+1, r, reasons)
			break
		}
	}
	if n := queryInt(t, `SELECT count(*) FROM payments WHERE created_at > $1 AND cashback_reason <> $2`,
		after, paused); n != 0 {
		t.Errorf("%d payments began after the command returned and were not paused", n)
	}
	if n := queryInt(t, `SELECT count(*) FROM payments WHERE created_at > $1`, after); n < 20 {
		t.Errorf("only %d payments began after the command returned, want at least 20", n)
	}
	assertReconciled(t)
}

// AC-39: a redemption that has read the flag and written, and is still open,
// does not block the pause command; the next redemption is refused; the open
// one commits.
func TestRacePauseRedemptionsHeldAC39(t *testing.T) {
	reset(t, 10_000_000)
	fund(t, "user_a", 5000)
	fund(t, "user_b", 5000)
	g := newGate(t)
	var calls, entered atomic.Int64
	h := hookedRouter(t, store.Hooks{RedeemHeld: func() {
		if calls.Add(1) == 1 {
			entered.Store(1)
			g.wait(3 * time.Second)
		}
	}})

	held := make(chan redeemRaceResult, 1)
	go func() {
		var r redeemRaceResult
		r.reply, r.err = doRedeem(h, map[string]string{"X-User-ID": "user_a", "Idempotency-Key": uuid.NewString()},
			`{"amount":1000}`)
		held <- r
	}()
	if !waitFor(5*time.Second, func() bool { return entered.Load() == 1 }) {
		t.Fatal("the redemption never reached the hook")
	}
	code, out, errOut := runAdmin(t, 1000, "pause-redemptions", "--by", "owner")
	after := dbClock(t)
	if code != 0 || errOut != "" {
		g.open()
		<-held
		t.Fatalf("pause-redemptions = exit %d, stderr %q while a redemption is open, want 0", code, errOut)
	}
	if line := parseActionLine(t, out); line["changed"] != true {
		t.Errorf("line = %v, want changed true", line)
	}

	late, err := doRedeem(h, map[string]string{"X-User-ID": "user_b", "Idempotency-Key": uuid.NewString()},
		`{"amount":1000}`)
	if err != nil {
		t.Error(err)
	}
	if late.status != http.StatusConflict || !strings.Contains(late.raw, "REDEMPTION_PAUSED") {
		t.Errorf("late redemption = %d: %s, want 409 REDEMPTION_PAUSED", late.status, late.raw)
	}
	if n := balanceOf(t, "user_b"); n != 5000 {
		t.Errorf("user_b balance = %d, want 5000", n)
	}
	if n := queryInt(t, `SELECT count(*) FROM redemptions WHERE created_at > $1`, after); n != 0 {
		t.Errorf("%d redemptions began after the command returned", n)
	}

	g.open()
	r := <-held
	if r.err != nil || r.reply.status != http.StatusCreated || r.reply.res.BalanceAfter != 4000 {
		t.Errorf("held redemption = %v %d: %s, want 201 with balance_after 4000", r.err, r.reply.status, r.reply.raw)
	}
	if n := balanceOf(t, "user_a"); n != 4000 {
		t.Errorf("user_a balance = %d, want 4000", n)
	}
	assertReconciled(t)
}

// AC-39: a redemption already waiting on the balance lock has not read the
// flag yet, so a pause that commits meanwhile refuses it.
func TestRacePauseRedemptionsWaitingAC39(t *testing.T) {
	reset(t, 10_000_000)
	fund(t, "user_a", 5000)
	h := racePool(t)

	release, pid := holdRowLock(t, `SELECT 1 FROM cashback_balances WHERE user_id = 'user_a' FOR UPDATE`)
	done := make(chan redeemRaceResult, 1)
	go func() {
		var r redeemRaceResult
		r.reply, r.err = doRedeem(h, map[string]string{"X-User-ID": "user_a", "Idempotency-Key": uuid.NewString()},
			`{"amount":1000}`)
		done <- r
	}()
	if blocked, pollErr := waitBlockedBy(pid); !blocked {
		release()
		<-done
		t.Fatalf("the redemption never waited on the balance lock (poll error %v)", pollErr)
	}
	code, _, errOut := runAdmin(t, 0, "pause-redemptions", "--by", "owner")
	release()
	r := <-done

	if code != 0 || errOut != "" {
		t.Fatalf("pause-redemptions = exit %d, stderr %q, want 0", code, errOut)
	}
	if r.err != nil || r.reply.status != http.StatusConflict || !strings.Contains(r.reply.raw, "REDEMPTION_PAUSED") {
		t.Errorf("waiting redemption = %v %d: %s, want 409 REDEMPTION_PAUSED", r.err, r.reply.status, r.reply.raw)
	}
	if n := balanceOf(t, "user_a"); n != 5000 {
		t.Errorf("balance = %d, want 5000", n)
	}
	if n := queryInt(t, `SELECT count(*) FROM redemptions`); n != 0 {
		t.Errorf("redemptions = %d, want 0", n)
	}
	assertReconciled(t)
}

// AC-39: 40 users redeem at once. Twenty have read the flag, written and are
// held before COMMIT; the other twenty wait on their balance lock, so they have
// not read the flag. The pause lands between the two groups: the held twenty
// answer 201, the waiting twenty answer 409 and change nothing.
func TestRacePauseRedemptionsAmongUsersAC39(t *testing.T) {
	reset(t, 10_000_000)
	const users, held = 40, 20
	names := make([]string, users)
	for i := range names {
		names[i] = "user_" + itoa(int64(i))
		fund(t, names[i], 5000)
	}
	release, pid := holdRowLock(t,
		`SELECT 1 FROM cashback_balances WHERE user_id = ANY($1) FOR UPDATE`, names[held:])

	g := newGate(t)
	var calls atomic.Int64
	h := hookedRouter(t, store.Hooks{RedeemHeld: func() {
		if calls.Add(1) <= held {
			g.wait(10 * time.Second) // fallback; the test opens the gate
		}
	}})

	reqs := make([]redeemRaceReq, users)
	for i := range reqs {
		reqs[i] = redeemRaceReq{user: names[i], amount: 1000}
	}
	results := make(chan []redeemRaceResult, 1)
	go func() { results <- redeemAllAtOnce(h, reqs) }()

	if !waitFor(10*time.Second, func() bool { return calls.Load() == held }) {
		release()
		g.open()
		<-results
		t.Fatalf("%d redemptions reached the hook, want %d", calls.Load(), held)
	}
	if blocked, pollErr := waitBlockedByN(pid, users-held); !blocked {
		release()
		g.open()
		<-results
		t.Fatalf("fewer than %d redemptions waited on the balance lock (poll error %v)", users-held, pollErr)
	}
	code, _, errOut := runAdmin(t, 0, "pause-redemptions", "--by", "owner")
	after := dbClock(t)
	release()
	// The waiting twenty read the flag now; give them time to answer before the held ones commit.
	waitFor(5*time.Second, func() bool {
		n, err := countInt(`SELECT count(*) FROM pg_stat_activity
			WHERE backend_type = 'client backend' AND cardinality(pg_blocking_pids(pid)) > 0`)
		return err == nil && n == 0
	})
	g.open()
	res := <-results

	if code != 0 || errOut != "" {
		t.Fatalf("pause-redemptions = exit %d, stderr %q, want 0", code, errOut)
	}
	created, paused := 0, 0
	for i, r := range res {
		user := names[i]
		switch {
		case r.err != nil:
			t.Errorf("redeem %d: %v", i, r.err)
		case r.reply.status == http.StatusCreated:
			created++
			if i >= held {
				t.Errorf("%s: 201 for a redemption that had not read the flag before the pause", user)
			}
			if n := balanceOf(t, user); n != 4000 {
				t.Errorf("%s: 201 but balance %d, want 4000", user, n)
			}
			if n := queryInt(t, `SELECT count(*) FROM redemptions WHERE user_id = $1`, user); n != 1 {
				t.Errorf("%s: 201 but %d redemption rows, want 1", user, n)
			}
		case r.reply.status == http.StatusConflict && strings.Contains(r.reply.raw, "REDEMPTION_PAUSED"):
			paused++
			if n := balanceOf(t, user); n != 5000 {
				t.Errorf("%s: 409 but balance %d, want 5000", user, n)
			}
			if n := queryInt(t, `SELECT count(*) FROM redemptions WHERE user_id = $1`, user); n != 0 {
				t.Errorf("%s: 409 but %d redemption rows, want 0", user, n)
			}
		default:
			t.Errorf("redeem %d: status %d: %s", i, r.reply.status, r.reply.raw)
		}
	}
	if created != held || paused != users-held {
		t.Errorf("201: %d, 409 REDEMPTION_PAUSED: %d, want %d and %d", created, paused, held, users-held)
	}
	if n := queryInt(t, `SELECT count(*) FROM redemptions WHERE created_at > $1`, after); n != 0 {
		t.Errorf("%d redemptions began after the command returned and succeeded", n)
	}
	assertReconciled(t)
}
