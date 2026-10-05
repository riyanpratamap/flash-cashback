//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/riyanpratamap/flash-cashback/backend/internal/cache"
)

// runDemoReset runs demo-reset through the real command with FC_DEMO set.
func runDemoReset(t *testing.T) (int, string, string) {
	t.Helper()
	opts := adminOptions(0)
	opts.Demo = true
	return runAdminWith(t, opts, "demo-reset")
}

type demoState struct {
	balanceA, earnedA, earnedC, spent int64
	rowsB                             int64
	awardsPaused, redemptionsPaused   bool
	budget                            int64
	paymentIDs, redemptionIDs, ledger string
}

func readDemoState(t *testing.T) demoState {
	t.Helper()
	var s demoState
	s.balanceA = balanceOf(t, "user_a")
	s.earnedA = queryInt(t, `SELECT COALESCE(sum(earned), 0) FROM user_daily_earnings WHERE user_id = 'user_a' AND day = fc_campaign_day(fc_now())`)
	s.earnedC = queryInt(t, `SELECT COALESCE(sum(earned), 0) FROM user_daily_earnings WHERE user_id = 'user_c' AND day = fc_campaign_day(fc_now())`)
	s.rowsB = queryInt(t, `SELECT (SELECT count(*) FROM payments WHERE user_id = 'user_b')
		+ (SELECT count(*) FROM redemptions WHERE user_id = 'user_b')
		+ (SELECT count(*) FROM cashback_balances WHERE user_id = 'user_b')
		+ (SELECT count(*) FROM ledger_entries WHERE user_id = 'user_b')
		+ (SELECT count(*) FROM user_daily_earnings WHERE user_id = 'user_b')`)
	err := pool.QueryRow(context.Background(), `SELECT spent, budget, awards_paused, redemptions_paused FROM campaigns`).
		Scan(&s.spent, &s.budget, &s.awardsPaused, &s.redemptionsPaused)
	if err != nil {
		t.Fatal(err)
	}
	for q, dst := range map[string]*string{
		`SELECT COALESCE(string_agg(id::text, ',' ORDER BY id), '') FROM payments`:       &s.paymentIDs,
		`SELECT COALESCE(string_agg(id::text, ',' ORDER BY id), '') FROM redemptions`:    &s.redemptionIDs,
		`SELECT COALESCE(string_agg(id::text, ',' ORDER BY id), '') FROM ledger_entries`: &s.ledger,
	} {
		if err := pool.QueryRow(context.Background(), q).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func redisKeys(t *testing.T) []string {
	t.Helper()
	keys, err := rdb.Keys(context.Background(), "fc:v1:*").Result()
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

// AC-57: without the demo flag the command refuses and changes nothing.
func TestDemoResetRefusesWithoutFlagAC57(t *testing.T) {
	reset(t, 10_000_000)
	fund(t, "user_b", 5000)
	setCampaignSQL(t, `UPDATE campaigns SET awards_paused = true`)
	plantKeys(t, cache.CampaignKey, cashbackB)
	before := readDemoState(t)

	code, out, errOut := runAdmin(t, 0, "demo-reset") // Demo false
	if code != 2 || out != "" || !strings.Contains(errOut, "FC_DEMO=1") {
		t.Fatalf("exit %d, stdout %q, stderr %q; want 2, empty, a refusal naming FC_DEMO=1", code, out, errOut)
	}
	if after := readDemoState(t); after != before {
		t.Errorf("state changed:\nbefore %+v\nafter  %+v", before, after)
	}
	wantKeys(t, "refused", map[string]bool{cache.CampaignKey: true, cashbackB: true})
	assertReconciled(t)
}

// AC-57: from a dirty state the command lands on the demo state, and again.
func TestDemoResetBuildsTheDemoStateAC57(t *testing.T) {
	reset(t, 7_654_321) // a budget no demo constant equals: it must be kept
	fund(t, "user_b", 5000)
	fund(t, "user_a", 1000)
	setCampaignSQL(t, `UPDATE campaigns SET awards_paused = true, redemptions_paused = true`)
	plantKeys(t, cache.CampaignKey, cashbackA, cashbackB, cache.CashbackKey("user_z"))

	var first demoState
	for run := 1; run <= 2; run++ {
		code, out, errOut := runDemoReset(t)
		if code != 0 || errOut != "" {
			t.Fatalf("run %d: exit %d, stderr %q", run, code, errOut)
		}
		var line map[string]any
		if err := json.Unmarshal([]byte(out), &line); err != nil || line["event"] != "demo_reset" {
			t.Fatalf("run %d: stdout %q (%v), want one demo_reset line", run, out, err)
		}
		at, _ := line["at"].(string)
		if ts, err := time.Parse(time.RFC3339, at); err != nil || time.Since(ts) > time.Minute {
			t.Errorf("run %d: at = %q (%v)", run, at, err)
		}
		got := readDemoState(t)
		want := demoState{balanceA: 15000, earnedA: 47000, earnedC: 50000, spent: 97000, budget: 7_654_321,
			paymentIDs: "1,2", redemptionIDs: "1", ledger: "1,2,3"}
		if got != want {
			t.Errorf("run %d: state\n got %+v\nwant %+v", run, got, want)
		}
		if keys := redisKeys(t); len(keys) != 0 {
			t.Errorf("run %d: keys left: %v", run, keys)
		}
		assertReconciled(t)
		if run == 1 {
			first = got
		} else if got != first {
			t.Errorf("second run differs from the first:\n%+v\n%+v", first, got)
		}
	}
}

// With Redis down the database state stands: a warning, exit 0.
func TestDemoResetWithRedisDownWarnsAC57(t *testing.T) {
	reset(t, 10_000_000)
	opts := adminOptions(0)
	opts.Demo = true
	opts.Cache = deadInvalidator(t)
	code, out, errOut := runAdminWith(t, opts, "demo-reset")
	if code != 0 || !strings.Contains(errOut, "warning") || strings.Contains(errOut, "127.0.0.1") {
		t.Fatalf("exit %d, stderr %q; want 0 and a warning without the address", code, errOut)
	}
	if !strings.Contains(out, `"event":"demo_reset"`) {
		t.Errorf("stdout = %q", out)
	}
	if got := readDemoState(t); got.balanceA != 15000 || got.spent != 97000 {
		t.Errorf("state = %+v", got)
	}
	assertReconciled(t)
}

// AC-57: a budget below 97000 cannot give the demo amounts: exit 1 with the
// fixed text (no SQL), nothing on stdout, and no cache key left, because the
// truncate had already made them stale.
func TestDemoResetStateNotReachedAC57(t *testing.T) {
	reset(t, 50_000)
	plantKeys(t, cache.CampaignKey, cashbackA, cashbackB, cache.CashbackKey("user_z"))

	code, out, errOut := runDemoReset(t)
	if code != 1 || out != "" || !strings.Contains(errOut, "demo state not reached") {
		t.Fatalf("exit %d, stdout %q, stderr %q; want 1, empty, demo state not reached", code, out, errOut)
	}
	if !strings.Contains(errOut, "run demo-reset again") ||
		strings.Contains(strings.ToUpper(errOut), "SELECT") || strings.Contains(strings.ToUpper(errOut), "INSERT") {
		t.Errorf("stderr %q: want the repair hint and no SQL", errOut)
	}
	if keys := redisKeys(t); len(keys) != 0 {
		t.Errorf("keys left after a failed run: %v", keys)
	}
	assertReconciled(t)
}
