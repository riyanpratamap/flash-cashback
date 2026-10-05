//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
)

type redeemRaceReq struct {
	user   string
	key    string // empty: a new UUID per request
	amount int64
}

type redeemRaceResult struct {
	reply redeemReply
	err   error
}

// redeemAllAtOnce sends every redemption from its own goroutine, released
// together. Goroutines only record; the test asserts.
func redeemAllAtOnce(h http.Handler, reqs []redeemRaceReq) []redeemRaceResult {
	out := make([]redeemRaceResult, len(reqs))
	allAtOnce(len(reqs), func(i int) {
		r := reqs[i]
		key := r.key
		if key == "" {
			key = uuid.NewString()
		}
		out[i].reply, out[i].err = doRedeem(h, map[string]string{
			"X-User-ID": r.user, "Idempotency-Key": key,
		}, `{"amount":`+itoa(r.amount)+`}`)
	})
	return out
}

// reportRedeemNot records, without stopping the test, every redemption that
// errored or answered other than want: the count and the first few bodies.
func reportRedeemNot(t *testing.T, res []redeemRaceResult, want func(redeemReply) bool, label string) {
	t.Helper()
	const shown = 3
	bad := 0
	for i, r := range res {
		var msg string
		switch {
		case r.err != nil:
			msg = fmt.Sprintf("redeem %d: %v", i, r.err)
		case !want(r.reply):
			msg = fmt.Sprintf("redeem %d: status %d replayed=%q: %s", i, r.reply.status, r.reply.replayed, r.reply.raw)
		default:
			continue
		}
		bad++
		if bad <= shown {
			t.Error(msg)
		}
	}
	if bad > 0 {
		t.Errorf("%d of %d redemptions were not %s", bad, len(res), label)
	}
}

// AC-33: two redemptions of the whole balance, at once. One succeeds, the
// other is INSUFFICIENT_BALANCE, and the balance never goes below zero.
//
// A test transaction holds the balance row, so both requests are past
// their request handling and waiting on it before either may proceed: without
// the row lock, both would have read 18000 by then and both would debit.
func TestRaceRedeemOneBalanceAC33(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:32:00Z")
	fund(t, "user_a", 18000)
	h := racePool(t)

	release, holderPid := holdRowLock(t,
		`SELECT 1 FROM cashback_balances WHERE user_id = 'user_a' FOR UPDATE`)

	done := make(chan []redeemRaceResult, 1)
	go func() {
		done <- redeemAllAtOnce(h, []redeemRaceReq{
			{user: "user_a", amount: 18000}, {user: "user_a", amount: 18000},
		})
	}()
	waiting, pollErr := waitBlockedByN(holderPid, 2)
	release()
	res := <-done
	if !waiting {
		if pollErr != nil {
			t.Fatalf("poll query failed: %v", pollErr)
		}
		t.Fatal("two redemptions never waited on the balance lock")
	}

	created, insufficient := 0, 0
	for i, r := range res {
		switch {
		case r.err != nil:
			t.Errorf("redeem %d: %v", i, r.err)
		case r.reply.status == http.StatusCreated:
			created++
			if r.reply.res.BalanceAfter != 0 {
				t.Errorf("redeem %d: balance_after = %d, want 0", i, r.reply.res.BalanceAfter)
			}
		case r.reply.status == http.StatusUnprocessableEntity && strings.Contains(r.reply.raw, "INSUFFICIENT_BALANCE"):
			insufficient++
		default:
			t.Errorf("redeem %d: status %d: %s", i, r.reply.status, r.reply.raw)
		}
	}
	if created != 1 || insufficient != 1 {
		t.Errorf("201s = %d, 422 INSUFFICIENT_BALANCE = %d, want 1 and 1", created, insufficient)
	}
	if n := balanceOf(t, "user_a"); n != 0 {
		t.Errorf("balance = %d, want 0", n)
	}
	if n := queryInt(t, `SELECT count(*) FROM redemptions`); n != 1 {
		t.Errorf("redemptions = %d, want 1", n)
	}
	if n := queryInt(t, `SELECT count(*) FROM ledger_entries`); n != 2 {
		t.Errorf("ledger entries = %d, want 2 (the award and one redemption)", n)
	}
	assertReconciled(t)
}

// AC-69: ten redemptions with one key and one body, at once. One debits; the
// other nine replay it, byte for byte.
func TestRaceRedeemSameKeyAC69(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:32:00Z")
	fund(t, "user_a", 18000)
	h := racePool(t)

	key := uuid.NewString()
	reqs := make([]redeemRaceReq, 10)
	for i := range reqs {
		reqs[i] = redeemRaceReq{user: "user_a", key: key, amount: 1000}
	}
	res := redeemAllAtOnce(h, reqs)

	created, createdRaw := 0, ""
	for i, r := range res {
		switch {
		case r.err != nil:
			t.Errorf("redeem %d: %v", i, r.err)
		case r.reply.status == http.StatusCreated:
			created++
			createdRaw = r.reply.raw
			if r.reply.replayed == "true" {
				t.Errorf("redeem %d: 201 marked replayed", i)
			}
		}
	}
	if created != 1 {
		t.Errorf("%d redemptions answered 201, want 1", created)
	}
	bad := 0
	for i, r := range res {
		if r.err != nil || r.reply.status == http.StatusCreated {
			continue
		}
		if r.reply.status != http.StatusOK || r.reply.replayed != "true" || r.reply.raw != createdRaw ||
			r.reply.res.BalanceAfter != 17000 {
			bad++
			if bad <= 3 {
				t.Errorf("redeem %d: status %d replayed=%q: %s, want 200 true %s",
					i, r.reply.status, r.reply.replayed, r.reply.raw, createdRaw)
			}
		}
	}
	if n := queryInt(t, `SELECT count(*) FROM redemptions`); n != 1 {
		t.Errorf("redemptions = %d, want 1", n)
	}
	if n := queryInt(t, `SELECT count(*) FROM ledger_entries WHERE kind = 'REDEMPTION'`); n != 1 {
		t.Errorf("REDEMPTION ledger entries = %d, want 1", n)
	}
	if n := balanceOf(t, "user_a"); n != 17000 {
		t.Errorf("balance = %d, want 17000", n)
	}
	assertReconciled(t)
}

// AC-28: payments and redemptions of one user, interleaved and at once, never
// deadlock: the award takes the campaign row without a key lock, the
// redemption only reads it.
func TestRaceAwardsAndRedemptionsAC28(t *testing.T) {
	reset(t, 10_000_000)
	// Fund on one campaign day, then race on the next, so today's earned is 0
	// and every redemption has its funds before the first award lands.
	mustSetNow(t, "2026-10-02T07:32:00Z")
	fund(t, "user_a", 10000)
	mustSetNow(t, "2026-10-03T07:32:00Z")
	h := racePool(t)

	const payments, redemptions = 20, 10
	payRes := make([]raceResult, payments)
	redRes := make([]redeemRaceResult, redemptions)
	type call struct {
		redeem bool
		idx    int
	}
	calls := make([]call, 0, payments+redemptions)
	for i := 0; i < payments; i++ {
		calls = append(calls, call{false, i})
		if i%2 == 1 {
			calls = append(calls, call{true, i / 2})
		}
	}
	allAtOnce(len(calls), func(i int) {
		c := calls[i]
		if c.redeem {
			redRes[c.idx].reply, redRes[c.idx].err = doRedeem(h, map[string]string{
				"X-User-ID": "user_a", "Idempotency-Key": uuid.NewString(),
			}, `{"amount":1000}`)
			return
		}
		payRes[c.idx].reply, payRes[c.idx].err = doPayment(h, map[string]string{
			"X-User-ID": "user_a", "Idempotency-Key": uuid.NewString(),
		}, `{"amount":100000}`)
	})

	reportNotCreated(t, payRes)
	reportRedeemNot(t, redRes, func(r redeemReply) bool { return r.status == http.StatusCreated }, "201")
	requireTally(t, tally(payRes), map[award]int{
		{5000, domain.ReasonAwarded}:      10,
		{0, domain.ReasonDailyCapReached}: 10,
	})
	if n := queryInt(t, `SELECT count(*) FROM redemptions`); n != redemptions {
		t.Errorf("redemptions = %d, want %d", n, redemptions)
	}
	if n := balanceOf(t, "user_a"); n != 50000 {
		t.Errorf("balance = %d, want 50000 (10000 + 50000 - 10000)", n)
	}
	if n := queryInt(t, `SELECT earned FROM user_daily_earnings WHERE user_id = 'user_a' AND day = '2026-10-03'`); n != 50000 {
		t.Errorf("earned on the race day = %d, want 50000", n)
	}
	assertReconciled(t)
}
