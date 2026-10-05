//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/riyanpratamap/flash-cashback/backend/internal/reconcile"
)

// reconcileLine is one line of the command's output; the fields a line does
// not carry stay zero.
type reconcileLine struct {
	Check      string   `json:"check"`
	Invariant  string   `json:"invariant"`
	OK         bool     `json:"ok"`
	Violations int64    `json:"violations"`
	Liability  *int64   `json:"liability"`
	Budget     *int64   `json:"budget"`
	Spent      *int64   `json:"spent"`
	Summary    string   `json:"summary"`
	Failed     []string `json:"failed"`
}

func parseReconcile(t *testing.T, out string) []reconcileLine {
	t.Helper()
	var lines []reconcileLine
	for _, raw := range strings.Split(strings.TrimSpace(out), "\n") {
		var l reconcileLine
		if err := json.Unmarshal([]byte(raw), &l); err != nil {
			t.Fatalf("reconcile line %q: %v\nfull output:\n%s", raw, err, out)
		}
		lines = append(lines, l)
	}
	return lines
}

// moneyShape is the row count of the six tables plus every row of them as
// ordered JSON, so a changed value shows as well as a changed count.
func moneyShape(t *testing.T) string {
	t.Helper()
	var s string
	err := pool.QueryRow(context.Background(), `SELECT
		(SELECT count(*) FROM payments) || '/' || (SELECT count(*) FROM redemptions) || '/' ||
		(SELECT count(*) FROM ledger_entries) || '/' || (SELECT count(*) FROM cashback_balances) || '/' ||
		(SELECT count(*) FROM user_daily_earnings) || '/' || (SELECT count(*) FROM campaigns) || '/' ||
		COALESCE((SELECT string_agg(to_json(c)::text, ',') FROM campaigns c), '')`).Scan(&s)
	if err != nil {
		t.Fatalf("money shape: %v", err)
	}
	return s
}

// runReconcile runs the checks on the shared pool and fails if they changed
// any row.
func runReconcile(t *testing.T) (string, bool) {
	t.Helper()
	before := moneyShape(t)
	var buf bytes.Buffer
	ok, err := reconcile.Run(context.Background(), pool, &buf)
	if err != nil {
		t.Fatalf("reconcile.Run: %v\n%s", err, buf.String())
	}
	if after := moneyShape(t); after != before {
		t.Errorf("reconcile changed data:\nbefore %s\nafter  %s", before, after)
	}
	return buf.String(), ok
}

// assertReconciled fails the test if any invariant check fails (INV-01 to INV-09).
func assertReconciled(t *testing.T) {
	t.Helper()
	if out, ok := runReconcile(t); !ok {
		t.Errorf("reconcile failed:\n%s", out)
	}
}

// assertBroken fails unless reconcile reports the named check as failed.
func assertBroken(t *testing.T, name string) {
	t.Helper()
	out, ok := runReconcile(t)
	if ok {
		t.Fatalf("reconcile passed, want %s failed:\n%s", name, out)
	}
	lines := parseReconcile(t, out)
	last := lines[len(lines)-1]
	if last.Summary != "failed" || !slices.Contains(last.Failed, name) {
		t.Fatalf("summary = %+v, want failed containing %s:\n%s", last, name, out)
	}
	for _, l := range lines {
		if l.Check == name && (l.OK || l.Violations < 1) {
			t.Errorf("line for %s = %+v, want ok=false with violations", name, l)
		}
	}
}

func execSQL(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// dropConstraint removes a guard from the live test schema. Cleanup empties the
// money tables, then puts the constraint back exactly as the catalogue had it.
func dropConstraint(t *testing.T, table, name string) {
	t.Helper()
	ctx := context.Background()
	var def string
	err := pool.QueryRow(ctx, `SELECT pg_get_constraintdef(c.oid) FROM pg_constraint c
		WHERE c.conname = $1 AND c.conrelid = $2::regclass`, name, table).Scan(&def)
	if err != nil {
		t.Fatalf("constraint %s on %s: %v", name, table, err)
	}
	execSQL(t, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", table, name))
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `TRUNCATE campaigns, user_daily_earnings, payments, redemptions,
			cashback_balances, ledger_entries RESTART IDENTITY CASCADE`); err != nil {
			t.Errorf("cleanup truncate: %v", err)
		}
		if _, err := pool.Exec(ctx, fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s %s", table, name, def)); err != nil {
			t.Errorf("cleanup re-add %s: %v", name, err)
		}
	})
}

// disableTrigger switches a trigger off in the live test schema until cleanup.
func disableTrigger(t *testing.T, table, trigger string) {
	t.Helper()
	execSQL(t, fmt.Sprintf("ALTER TABLE %s DISABLE TRIGGER %s", table, trigger))
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(),
			fmt.Sprintf("ALTER TABLE %s ENABLE TRIGGER %s", table, trigger)); err != nil {
			t.Errorf("cleanup enable %s: %v", trigger, err)
		}
	})
}

// seedBooks builds a clean state through the services: user_a pays 100000 and
// 19999 (a zero award), user_c pays 100000, user_d pays 100000 and redeems 2000.
func seedBooks(t *testing.T) {
	t.Helper()
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	for _, p := range []struct {
		user   string
		amount int64
	}{{"user_a", 100000}, {"user_c", 100000}, {"user_a", 19999}, {"user_d", 100000}} {
		if r := pay(t, p.user, p.amount); r.status != 201 {
			t.Fatalf("seed payment %+v = %d: %s", p, r.status, r.raw)
		}
	}
	if r := redeem(t, "user_d", 2000); r.status != 201 || r.res.BalanceAfter != 3000 {
		t.Fatalf("seed redemption = %d: %s", r.status, r.raw)
	}
}

// insertPaymentCopy inserts, under a new key, a copy of payment 1 with the
// given award, reason, and day offset.
func insertPaymentCopy(t *testing.T, award int64, reason string, dayOffset int) {
	t.Helper()
	execSQL(t, `INSERT INTO payments (campaign_id, user_id, idempotency_key, request_hash, amount, status,
		cashback_awarded, cashback_reason, rate_bps, min_payment, daily_cap, campaign_day, created_at)
		SELECT campaign_id, user_id, gen_random_uuid(), request_hash, amount, status, $1, $2, rate_bps,
		min_payment, daily_cap, campaign_day + $3::int, created_at FROM payments WHERE id = 1`,
		award, reason, dayOffset)
}

// insertRedemption inserts a redemption row by SQL (the break tests need
// rows the services never write) and returns its id. An empty key means a
// new one.
func insertRedemption(t *testing.T, user, key string, amount, balanceAfter int64, day string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `INSERT INTO redemptions (campaign_id, user_id, idempotency_key,
		request_hash, amount, status, destination, balance_after, campaign_day, created_at)
		VALUES ('flash-cashback', $1, COALESCE(NULLIF($2, '')::uuid, gen_random_uuid()),
		decode(repeat('00', 32), 'hex'), $3, 'COMPLETED', 'MAIN_ACCOUNT', $4, $5::date,
		'2026-10-03T07:00:00Z') RETURNING id`, user, key, amount, balanceAfter, day).Scan(&id)
	if err != nil {
		t.Fatalf("insert redemption: %v", err)
	}
	return id
}

func insertRedemptionEntry(t *testing.T, user string, redemptionID, amount, balanceAfter int64) {
	t.Helper()
	execSQL(t, `INSERT INTO ledger_entries (user_id, kind, amount, redemption_id, balance_after, created_at)
		VALUES ($1, 'REDEMPTION', $2, $3, $4, '2026-10-03T07:00:00Z')`, user, amount, redemptionID, balanceAfter)
}

var allChecks = []string{
	"ledger_sum_equals_balance", "spent_within_budget_and_equals_awards", "daily_earned_matches_awards",
	"no_negative_balance", "idempotency_key_unique", "one_ledger_entry_per_money_row",
	"award_within_rate_and_reason", "campaign_day_matches_created_at", "balance_after_running_sum",
}

func TestReconcileCleanBooks(t *testing.T) {
	seedBooks(t)
	out, ok := runReconcile(t)
	if !ok {
		t.Fatalf("clean books failed:\n%s", out)
	}
	lines := parseReconcile(t, out)
	if len(lines) != len(allChecks)+2 {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(allChecks)+2, out)
	}
	for i, name := range allChecks {
		l := lines[i]
		wantInv := fmt.Sprintf("INV-%02d", i+1)
		if l.Check != name || l.Invariant != wantInv || !l.OK || l.Violations != 0 {
			t.Errorf("line %d = %+v, want %s %s ok with 0 violations", i, l, name, wantInv)
		}
	}
	liab := lines[len(allChecks)]
	if liab.Check != "liability" || liab.Liability == nil || liab.Budget == nil || liab.Spent == nil {
		t.Fatalf("liability line = %+v", liab)
	}
	if *liab.Liability != 13000 || *liab.Budget != 10_000_000 || *liab.Spent != 15000 {
		t.Errorf("liability %d budget %d spent %d, want 13000 10000000 15000", *liab.Liability, *liab.Budget, *liab.Spent)
	}
	if sum := lines[len(lines)-1]; sum.Summary != "ok" || len(sum.Failed) != 0 {
		t.Errorf("summary = %+v, want ok", sum)
	}
}

func TestReconcileOnEmptyBooks(t *testing.T) {
	reset(t, 10_000_000)
	assertReconciled(t) // a user with no rows yet, and no rows at all
}

func TestReconcileBreaks(t *testing.T) {
	type tc struct {
		name  string
		check string
		apply func(t *testing.T)
	}
	cases := []tc{
		{"1 balance plus one", "ledger_sum_equals_balance", func(t *testing.T) {
			execSQL(t, `UPDATE cashback_balances SET balance = balance + 1 WHERE user_id = 'user_a'`)
		}},
		{"1 balance without ledger rows", "ledger_sum_equals_balance", func(t *testing.T) {
			execSQL(t, `INSERT INTO cashback_balances (user_id, balance, updated_at) VALUES ('user_new', 1, now())`)
		}},
		{"1 balance row deleted", "ledger_sum_equals_balance", func(t *testing.T) {
			execSQL(t, `DELETE FROM cashback_balances WHERE user_id = 'user_c'`)
		}},
		{"2 spent plus one", "spent_within_budget_and_equals_awards", func(t *testing.T) {
			execSQL(t, `UPDATE campaigns SET spent = spent + 1`)
		}},
		{"2 budget below spent", "spent_within_budget_and_equals_awards", func(t *testing.T) {
			// spent still equals the awards, so only the spent > budget branch can fire
			dropConstraint(t, "campaigns", "campaigns_spent_within_budget")
			execSQL(t, `UPDATE campaigns SET budget = spent - 1`)
		}},
		{"3 earned minus one", "daily_earned_matches_awards", func(t *testing.T) {
			execSQL(t, `UPDATE user_daily_earnings SET earned = earned - 1 WHERE user_id = 'user_a'`)
		}},
		{"3 cap below earned", "daily_earned_matches_awards", func(t *testing.T) {
			// earned still equals the payments, so only the earned > daily_cap branch can fire
			dropConstraint(t, "user_daily_earnings", "ude_earned_within_cap")
			execSQL(t, `UPDATE user_daily_earnings SET daily_cap = earned - 1 WHERE user_id = 'user_a'`)
		}},
		{"3 earnings row missing", "daily_earned_matches_awards", func(t *testing.T) {
			execSQL(t, `DELETE FROM user_daily_earnings WHERE user_id = 'user_c'`)
		}},
		{"4 negative balance", "no_negative_balance", func(t *testing.T) {
			dropConstraint(t, "cashback_balances", "balances_nonneg")
			execSQL(t, `UPDATE cashback_balances SET balance = -1 WHERE user_id = 'user_c'`)
		}},
		{"4 negative ledger balance_after", "no_negative_balance", func(t *testing.T) {
			dropConstraint(t, "ledger_entries", "ledger_balance_after_nonneg")
			disableTrigger(t, "ledger_entries", "ledger_append_only")
			execSQL(t, `UPDATE ledger_entries SET balance_after = -1 WHERE payment_id = 2`)
		}},
		{"4 negative redemption balance_after", "no_negative_balance", func(t *testing.T) {
			dropConstraint(t, "redemptions", "redemptions_balance_after_nonneg")
			insertRedemption(t, "user_c", "", 1000, -1, "2026-10-03")
		}},
		{"5 duplicate payment key", "idempotency_key_unique", func(t *testing.T) {
			dropConstraint(t, "payments", "payments_user_key_unique")
			execSQL(t, `INSERT INTO payments (campaign_id, user_id, idempotency_key, request_hash, amount, status,
				cashback_awarded, cashback_reason, rate_bps, min_payment, daily_cap, campaign_day, created_at)
				SELECT campaign_id, user_id, idempotency_key, request_hash, amount, status, 0, 'BELOW_MINIMUM',
				rate_bps, min_payment, daily_cap, campaign_day, created_at FROM payments WHERE id = 1`)
		}},
		{"5 duplicate redemption key", "idempotency_key_unique", func(t *testing.T) {
			dropConstraint(t, "redemptions", "redemptions_user_key_unique")
			key := "123e4567-e89b-12d3-a456-426614174000"
			insertRedemption(t, "user_c", key, 1000, 4000, "2026-10-03")
			insertRedemption(t, "user_c", key, 1000, 4000, "2026-10-03")
		}},
		{"6 award entry deleted", "one_ledger_entry_per_money_row", func(t *testing.T) {
			disableTrigger(t, "ledger_entries", "ledger_append_only")
			execSQL(t, `DELETE FROM ledger_entries WHERE payment_id = 1`)
		}},
		{"6 award entry of another user", "one_ledger_entry_per_money_row", func(t *testing.T) {
			// user_a's award moves to user_c with both balances and balance_after
			// adjusted, so checks 1, 2, 3 and 9 stay consistent
			disableTrigger(t, "ledger_entries", "ledger_append_only")
			execSQL(t, `UPDATE ledger_entries SET user_id = 'user_c' WHERE payment_id = 1`)
			execSQL(t, `UPDATE ledger_entries SET balance_after = 10000 WHERE payment_id = 2`)
			execSQL(t, `UPDATE cashback_balances SET balance = 0 WHERE user_id = 'user_a'`)
			execSQL(t, `UPDATE cashback_balances SET balance = 10000 WHERE user_id = 'user_c'`)
		}},
		{"6 entry with neither payment nor redemption", "one_ledger_entry_per_money_row", func(t *testing.T) {
			dropConstraint(t, "ledger_entries", "ledger_kind_shape")
			execSQL(t, `INSERT INTO ledger_entries (user_id, kind, amount, balance_after, created_at)
				VALUES ('user_c', 'AWARD', 0, 5000, '2026-10-03T07:00:00Z')`)
		}},
		{"6 award entry on a zero-award payment", "one_ledger_entry_per_money_row", func(t *testing.T) {
			execSQL(t, `INSERT INTO ledger_entries (user_id, kind, amount, payment_id, balance_after, created_at)
				VALUES ('user_a', 'AWARD', 1, 3, 5001, '2026-10-03T07:00:00Z')`)
		}},
		{"6 redemption without an entry", "one_ledger_entry_per_money_row", func(t *testing.T) {
			insertRedemption(t, "user_c", "", 1000, 4000, "2026-10-03")
		}},
		{"6 redemption entry of another amount", "one_ledger_entry_per_money_row", func(t *testing.T) {
			id := insertRedemption(t, "user_c", "", 1000, 4000, "2026-10-03")
			insertRedemptionEntry(t, "user_c", id, -999, 4000)
		}},
		{"7 award above the rate", "award_within_rate_and_reason", func(t *testing.T) {
			dropConstraint(t, "payments", "payments_award_within_rate")
			insertPaymentCopy(t, 5001, "AWARDED", 0)
		}},
		{"7 reason does not match the award", "award_within_rate_and_reason", func(t *testing.T) {
			dropConstraint(t, "payments", "payments_reason_matches_award")
			insertPaymentCopy(t, 0, "AWARDED", 0)
		}},
		{"8 payment on the wrong day", "campaign_day_matches_created_at", func(t *testing.T) {
			insertPaymentCopy(t, 0, "BELOW_MINIMUM", 1)
		}},
		{"8 redemption on the wrong day", "campaign_day_matches_created_at", func(t *testing.T) {
			insertRedemption(t, "user_c", "", 1000, 4000, "2026-10-04")
		}},
		{"9 ledger balance_after plus one", "balance_after_running_sum", func(t *testing.T) {
			disableTrigger(t, "ledger_entries", "ledger_append_only")
			execSQL(t, `UPDATE ledger_entries SET balance_after = balance_after + 1 WHERE payment_id = 1`)
		}},
		{"9 redemption balance_after differs from its entry", "balance_after_running_sum", func(t *testing.T) {
			id := insertRedemption(t, "user_c", "", 1000, 10, "2026-10-03")
			insertRedemptionEntry(t, "user_c", id, -1000, 4000)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedBooks(t)
			assertReconciled(t)
			c.apply(t)
			assertBroken(t, c.check)
		})
	}
}

func TestReconcileCommand(t *testing.T) {
	run := func(url string, wait time.Duration) (int, string, string) {
		var out, errOut bytes.Buffer
		code := reconcile.Command(context.Background(), url, wait, &out, &errOut)
		return code, out.String(), errOut.String()
	}

	t.Run("clean books exit 0", func(t *testing.T) {
		seedBooks(t)
		code, out, errOut := run(testURL, 5*time.Second)
		if code != 0 || errOut != "" {
			t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
		}
		lines := parseReconcile(t, out)
		if last := lines[len(lines)-1]; last.Summary != "ok" {
			t.Errorf("summary = %+v", last)
		}
	})

	t.Run("a broken invariant exits 1 and names the check", func(t *testing.T) {
		seedBooks(t)
		execSQL(t, `UPDATE cashback_balances SET balance = balance + 1 WHERE user_id = 'user_a'`)
		code, out, _ := run(testURL, 5*time.Second)
		if code != 1 {
			t.Fatalf("exit %d, want 1\n%s", code, out)
		}
		lines := parseReconcile(t, out)
		if last := lines[len(lines)-1]; last.Summary != "failed" || !slices.Equal(last.Failed, []string{"ledger_sum_equals_balance"}) {
			t.Errorf("summary = %+v", last)
		}
	})

	t.Run("a query failing inside the snapshot exits 2 with the check and SQLSTATE", func(t *testing.T) {
		seedBooks(t)
		execSQL(t, `ALTER FUNCTION fc_campaign_day(timestamptz) RENAME TO fc_campaign_day_hidden`)
		t.Cleanup(func() { // runs on failure too; later tests need the function
			if _, err := pool.Exec(context.Background(),
				`ALTER FUNCTION fc_campaign_day_hidden(timestamptz) RENAME TO fc_campaign_day`); err != nil {
				t.Errorf("cleanup restore fc_campaign_day: %v", err)
			}
		})
		code, out, errOut := run(testURL, 5*time.Second)
		if code != 2 {
			t.Fatalf("exit %d, want 2\n%s\n%s", code, out, errOut)
		}
		if out != "" {
			t.Errorf("stdout %q, want nothing written when a check cannot run", out)
		}
		for _, want := range []string{"campaign_day_matches_created_at", "SQLSTATE 42883"} {
			if !strings.Contains(errOut, want) {
				t.Errorf("stderr %q lacks %q", errOut, want)
			}
		}
		if strings.Contains(errOut, "SELECT") {
			t.Errorf("stderr leaks SQL: %s", errOut)
		}
	})

	t.Run("an unreachable database exits 2 without secrets", func(t *testing.T) {
		code, out, errOut := run("postgres://someone:s3cret@127.0.0.1:1/flash?sslmode=disable", time.Second)
		if code != 2 {
			t.Fatalf("exit %d, want 2", code)
		}
		if out != "" || errOut == "" {
			t.Errorf("stdout %q, stderr %q, want only an error message", out, errOut)
		}
		for _, secret := range []string{"s3cret", "someone", "127.0.0.1", "SELECT"} {
			if strings.Contains(errOut, secret) {
				t.Errorf("stderr leaks %q: %s", secret, errOut)
			}
		}
	})
}
