//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/riyanpratamap/flash-cashback/backend/internal/admin"
	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
)

// adminOptions are the command's settings against the test database and Redis.
func adminOptions(lockMS int64) admin.Options {
	return admin.Options{DatabaseURL: testURL, LockTimeoutMS: lockMS, ConnectWait: 10 * time.Second, Cache: testInvalidator()}
}

// runAdmin runs the real command entry point against the test database and
// returns its exit code, stdout and stderr. A zero lockMS uses the command's
// own 5 s.
func runAdmin(t *testing.T, lockMS int64, args ...string) (int, string, string) {
	t.Helper()
	return runAdminWith(t, adminOptions(lockMS), args...)
}

// runAdminWith is runAdmin with the given options.
func runAdminWith(t *testing.T, opts admin.Options, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := admin.Command(context.Background(), args, opts, &out, &errOut)
	return code, out.String(), errOut.String()
}

// mustAdmin runs a command that must succeed and returns its single line.
func mustAdmin(t *testing.T, args ...string) map[string]any {
	t.Helper()
	code, out, errOut := runAdmin(t, 0, args...)
	if code != 0 || errOut != "" {
		t.Fatalf("admin %v = exit %d, stderr %q", args, code, errOut)
	}
	return parseActionLine(t, out)
}

func parseActionLine(t *testing.T, out string) map[string]any {
	t.Helper()
	if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
		t.Fatalf("stdout = %q, want exactly one line", out)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("stdout is not JSON: %q", out)
	}
	return m
}

func switchFlags(t *testing.T) (awards, redemptions bool) {
	t.Helper()
	if err := pool.QueryRow(context.Background(),
		`SELECT awards_paused, redemptions_paused FROM campaigns`).Scan(&awards, &redemptions); err != nil {
		t.Fatal(err)
	}
	return awards, redemptions
}

func redemptionStatus(t *testing.T) string {
	t.Helper()
	_, m := getBody(t, "/v1/campaign", "user_new")
	return m["redemption_status"].(string)
}

// AC-40: each command prints one JSON line with the switch, the action, the
// operator, old and new, changed and the time; a second run changes nothing,
// says so, and exits 0.
func TestSwitchCommandLineAC40(t *testing.T) {
	cases := []struct {
		cmd, sw, action string
		paused          bool
		start           string // SQL that puts the switch in the opposite state
	}{
		{"pause-awards", "awards", "pause", true, ``},
		{"resume-awards", "awards", "resume", false, `UPDATE campaigns SET awards_paused = true`},
		{"pause-redemptions", "redemptions", "pause", true, ``},
		{"resume-redemptions", "redemptions", "resume", false, `UPDATE campaigns SET redemptions_paused = true`},
	}
	for _, c := range cases {
		t.Run(c.cmd, func(t *testing.T) {
			reset(t, 10_000_000)
			if c.start != "" {
				setCampaignSQL(t, c.start)
			}
			first := mustAdmin(t, c.cmd, "--by", "riyan")
			at, ok := first["at"].(string)
			if !ok {
				t.Fatalf("at = %v", first["at"])
			}
			parsed, err := time.Parse(time.RFC3339, at)
			if err != nil || !strings.HasSuffix(at, "+07:00") || time.Since(parsed) > time.Minute || time.Until(parsed) > time.Minute {
				t.Errorf("at = %q (%v), want a recent WIB RFC 3339 time", at, err)
			}
			delete(first, "at")
			want := map[string]any{
				"event": "operator_action", "switch": c.sw, "action": c.action, "operator": "riyan",
				"old": !c.paused, "new": c.paused, "changed": true,
			}
			if !mapsEqual(first, want) {
				t.Errorf("first line = %v, want %v", first, want)
			}

			updatedBefore := campaignUpdatedAt(t)
			second := mustAdmin(t, c.cmd, "--by", "riyan")
			delete(second, "at")
			want["old"], want["changed"] = c.paused, false
			if !mapsEqual(second, want) {
				t.Errorf("second line = %v, want %v", second, want)
			}
			if after := campaignUpdatedAt(t); !after.Equal(updatedBefore) {
				t.Errorf("an unchanged switch rewrote updated_at")
			}
			assertReconciled(t)
		})
	}
}

func campaignUpdatedAt(t *testing.T) time.Time {
	t.Helper()
	var at time.Time
	if err := pool.QueryRow(context.Background(), `SELECT updated_at FROM campaigns`).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// AC-40: without --by, with an empty --by, or with an unknown command the
// command exits 2, prints nothing on stdout and changes nothing.
func TestSwitchCommandUsageChangesNothingAC40(t *testing.T) {
	for name, args := range map[string][]string{
		"no --by":         {"pause-awards"},
		"empty --by":      {"pause-awards", "--by", ""},
		"redemptions too": {"pause-redemptions"},
		"unknown command": {"pause-everything", "--by", "owner"},
		"no command":      {},
		"extra argument":  {"pause-awards", "--by", "owner", "now"},
	} {
		t.Run(name, func(t *testing.T) {
			reset(t, 10_000_000)
			code, out, errOut := runAdmin(t, 0, args...)
			if code != 2 {
				t.Errorf("exit = %d, want 2 (stderr %q)", code, errOut)
			}
			if out != "" {
				t.Errorf("stdout = %q, want empty", out)
			}
			if a, r := switchFlags(t); a || r {
				t.Errorf("flags = %v %v, want both false", a, r)
			}
			assertReconciled(t)
		})
	}
}

// AC-36: pause-awards shows PAUSED, payments earn 0 CAMPAIGN_PAUSED,
// redemptions are unaffected; resume-awards restores ACTIVE.
func TestPauseAwardsAC36(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	fund(t, "user_a", 18000)

	mustAdmin(t, "pause-awards", "--by", "owner")
	if st := campaignStatus(t); st != "PAUSED" {
		t.Errorf("status = %s, want PAUSED", st)
	}
	if rs := redemptionStatus(t); rs != "AVAILABLE" {
		t.Errorf("redemption_status = %s, want AVAILABLE", rs)
	}
	if got := pay(t, "user_a", 100000); got.status != http.StatusCreated || got.res.Cashback.Awarded != 0 ||
		got.res.Cashback.Reason != domain.ReasonCampaignPaused {
		t.Errorf("payment while paused = %d %s, want 201, 0 CAMPAIGN_PAUSED", got.status, got.raw)
	}
	if got := redeem(t, "user_a", 1000); got.status != http.StatusCreated {
		t.Errorf("redemption while awards paused = %d %s, want 201", got.status, got.raw)
	}

	mustAdmin(t, "resume-awards", "--by", "owner")
	if st := campaignStatus(t); st != "ACTIVE" {
		t.Errorf("status after resume = %s, want ACTIVE", st)
	}
	if got := pay(t, "user_a", 100000); got.status != http.StatusCreated || got.res.Cashback.Awarded != 5000 {
		t.Errorf("payment after resume = %d %s, want 5000", got.status, got.raw)
	}
	assertReconciled(t)
}

// AC-37: pause-redemptions shows redemption_status PAUSED, a redemption is
// 409 REDEMPTION_PAUSED and changes nothing, payments still award;
// resume-redemptions makes a redemption 201.
func TestPauseRedemptionsAC37(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	fund(t, "user_a", 18000)

	mustAdmin(t, "pause-redemptions", "--by", "owner")
	if rs := redemptionStatus(t); rs != "PAUSED" {
		t.Errorf("redemption_status = %s, want PAUSED", rs)
	}
	if st := campaignStatus(t); st != "ACTIVE" {
		t.Errorf("status = %s, want ACTIVE", st)
	}
	before := readRedeemShape(t, "user_a")
	got := redeem(t, "user_a", 1000)
	if got.status != http.StatusConflict || !strings.Contains(got.raw, `"code":"REDEMPTION_PAUSED"`) {
		t.Errorf("redemption while paused = %d %s, want 409 REDEMPTION_PAUSED", got.status, got.raw)
	}
	if after := readRedeemShape(t, "user_a"); after != before {
		t.Errorf("a paused redemption changed rows: %+v -> %+v", before, after)
	}
	if p := pay(t, "user_b", 100000); p.status != http.StatusCreated || p.res.Cashback.Awarded != 5000 {
		t.Errorf("payment while redemptions paused = %d %s, want 5000", p.status, p.raw)
	}

	mustAdmin(t, "resume-redemptions", "--by", "owner")
	if got := redeem(t, "user_a", 1000); got.status != http.StatusCreated {
		t.Errorf("redemption after resume = %d %s, want 201", got.status, got.raw)
	}
	assertReconciled(t)
}

// A switch change that cannot get the campaign row within the lock timeout
// exits 1 with "busy, retry", prints nothing on stdout, changes nothing and
// returns quickly.
func TestSwitchCommandBusy(t *testing.T) {
	reset(t, 10_000_000)
	release, _ := holdCampaignLock(t)
	// The hold ends after 3 s if the lock timeout is broken, so a mutation
	// shows as a slow answer rather than a hang.
	timer := time.AfterFunc(3*time.Second, release)
	defer timer.Stop()

	start := time.Now()
	code, out, errOut := runAdmin(t, 300, "pause-awards", "--by", "owner")
	elapsed := time.Since(start)
	release()

	if code != 1 || !strings.Contains(errOut, "busy, retry") {
		t.Errorf("exit %d, stderr %q, want 1 and busy, retry", code, errOut)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty", out)
	}
	if elapsed > 2*time.Second {
		t.Errorf("answered after %v, want about the 300ms lock timeout", elapsed)
	}
	if a, _ := switchFlags(t); a {
		t.Error("awards_paused changed although the command was busy")
	}
	assertReconciled(t)
}

// A command that starts while another change to the same switch is open must
// wait for it and then see its result: old true, changed false, and the other
// change's updated_at kept. Without the row lock on the switch read the
// command reads the old value, waits only at its UPDATE, and rewrites it.
func TestRaceSwitchWaitsForOpenChange(t *testing.T) {
	reset(t, 10_000_000)
	ctx := context.Background()
	const heldAt = "2026-01-02T03:04:05Z"

	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	closeHolder := func(commit bool) {
		once.Do(func() {
			if commit {
				if err := holder.Commit(ctx); err != nil {
					t.Error(err)
				}
				return
			}
			_ = holder.Rollback(ctx)
		})
	}
	t.Cleanup(func() { closeHolder(false) })
	var pid int64
	if err := holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(ctx,
		`UPDATE campaigns SET awards_paused = true, updated_at = $1`, heldAt); err != nil {
		t.Fatal(err)
	}

	type result struct {
		code     int
		out, err string
	}
	done := make(chan result, 1)
	go func() {
		code, out, errOut := runAdmin(t, 0, "pause-awards", "--by", "owner")
		done <- result{code, out, errOut}
	}()

	blocked, pollErr := waitBlockedBy(pid)
	if !blocked {
		closeHolder(false)
		<-done
		t.Fatalf("the command never waited on the open change (poll error %v)", pollErr)
	}
	closeHolder(true)
	r := <-done

	if r.code != 0 || r.err != "" {
		t.Fatalf("exit %d, stderr %q, want 0", r.code, r.err)
	}
	line := parseActionLine(t, r.out)
	if line["old"] != true || line["new"] != true || line["changed"] != false {
		t.Errorf("line = %v, want old true, new true, changed false", line)
	}
	want, _ := time.Parse(time.RFC3339, heldAt)
	if got := campaignUpdatedAt(t); !got.Equal(want) {
		t.Errorf("updated_at = %v, want the other change's %v", got, want)
	}
	assertReconciled(t)
}
