//go:build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/riyanpratamap/flash-cashback/backend/internal/boot"
	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
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
			out[i].reply, out[i].err = doPayment(h, map[string]string{
				"X-User-ID": r.user, "Idempotency-Key": uuid.NewString(),
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
		reqs[i] = raceReq{fmt.Sprintf("user_%02d", i), 100000}
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
		reqs[i] = raceReq{"user_a", 120000}
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
