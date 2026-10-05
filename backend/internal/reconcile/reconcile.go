// Package reconcile proves the money invariants (INV-01 to INV-09) over one
// snapshot of the database. It only reports: it never writes, and its
// transaction is never committed.
package reconcile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// check is one invariant check. Its SQL returns one row with one column: the
// number of violating rows. No NULL can come back.
type check struct {
	name string
	inv  string
	sql  string
}

var checks = []check{
	{"ledger_sum_equals_balance", "INV-01", `
		SELECT count(*) FROM (SELECT user_id, sum(amount) AS total FROM ledger_entries GROUP BY user_id) l
		FULL OUTER JOIN cashback_balances b ON b.user_id = l.user_id
		WHERE COALESCE(l.total, 0) <> COALESCE(b.balance, 0)`},

	// Assumes the one campaign row, flash-cashback (tech-spec section 2): the
	// payments sum is scoped to the campaign, the ledger AWARD sum is global.
	{"spent_within_budget_and_equals_awards", "INV-02", `
		SELECT count(*) FROM campaigns c
		WHERE c.spent > c.budget
		   OR c.spent <> COALESCE((SELECT sum(p.cashback_awarded) FROM payments p WHERE p.campaign_id = c.id), 0)
		   OR c.spent <> COALESCE((SELECT sum(l.amount) FROM ledger_entries l WHERE l.kind = 'AWARD'), 0)`},

	{"daily_earned_matches_awards", "INV-03", `
		SELECT count(*) FROM user_daily_earnings u
		FULL OUTER JOIN (
			SELECT campaign_id, user_id, campaign_day AS day, sum(cashback_awarded) AS awarded
			FROM payments GROUP BY campaign_id, user_id, campaign_day) p
		  ON p.campaign_id = u.campaign_id AND p.user_id = u.user_id AND p.day = u.day
		WHERE COALESCE(u.earned, 0) <> COALESCE(p.awarded, 0) OR u.earned > u.daily_cap`},

	{"no_negative_balance", "INV-04", `
		SELECT (SELECT count(*) FROM cashback_balances WHERE balance < 0)
		     + (SELECT count(*) FROM ledger_entries WHERE balance_after < 0)
		     + (SELECT count(*) FROM redemptions WHERE balance_after < 0)`},

	{"idempotency_key_unique", "INV-05", `
		SELECT (SELECT count(*) FROM (SELECT 1 FROM payments
		          GROUP BY user_id, idempotency_key HAVING count(*) > 1) d)
		     + (SELECT count(*) FROM (SELECT 1 FROM redemptions
		          GROUP BY user_id, idempotency_key HAVING count(*) > 1) d)`},

	{"one_ledger_entry_per_money_row", "INV-06", `
		SELECT (SELECT count(*) FROM payments p WHERE
		          (SELECT count(*) FROM ledger_entries l WHERE l.payment_id = p.id) <> (p.cashback_awarded > 0)::int
		       OR (SELECT count(*) FROM ledger_entries l WHERE l.payment_id = p.id AND l.kind = 'AWARD'
		             AND l.amount = p.cashback_awarded AND l.user_id = p.user_id) <> (p.cashback_awarded > 0)::int)
		     + (SELECT count(*) FROM redemptions r WHERE
		          (SELECT count(*) FROM ledger_entries l WHERE l.redemption_id = r.id) <> 1
		       OR (SELECT count(*) FROM ledger_entries l WHERE l.redemption_id = r.id AND l.kind = 'REDEMPTION'
		             AND l.amount = -r.amount AND l.user_id = r.user_id) <> 1)
		     + (SELECT count(*) FROM ledger_entries WHERE payment_id IS NULL AND redemption_id IS NULL)`},

	{"award_within_rate_and_reason", "INV-07", `
		SELECT count(*) FROM payments
		WHERE cashback_awarded * 10000 > amount * rate_bps
		   OR (cashback_awarded > 0) <>
		      (cashback_reason IN ('AWARDED', 'PARTIAL_DAILY_CAP', 'PARTIAL_BUDGET'))`},

	{"campaign_day_matches_created_at", "INV-08", `
		SELECT (SELECT count(*) FROM payments WHERE campaign_day <> fc_campaign_day(created_at))
		     + (SELECT count(*) FROM redemptions WHERE campaign_day <> fc_campaign_day(created_at))`},

	{"balance_after_running_sum", "INV-09", `
		SELECT (SELECT count(*) FROM (
		          SELECT balance_after, sum(amount) OVER (PARTITION BY user_id ORDER BY id) AS running
		          FROM ledger_entries) x WHERE x.running <> x.balance_after)
		     + (SELECT count(*) FROM redemptions r WHERE EXISTS (
		          SELECT 1 FROM ledger_entries l
		          WHERE l.redemption_id = r.id AND l.balance_after <> r.balance_after))`},
}

// liabilitySQL reports what the platform owes (the sum of balances) next to
// the budget and what is spent. Operator output only; it never fails a run.
const liabilitySQL = `
	SELECT COALESCE((SELECT sum(balance) FROM cashback_balances), 0)::bigint,
	       COALESCE((SELECT sum(budget) FROM campaigns), 0)::bigint,
	       COALESCE((SELECT sum(spent) FROM campaigns), 0)::bigint`

type checkLine struct {
	Check      string `json:"check"`
	Invariant  string `json:"invariant"`
	OK         bool   `json:"ok"`
	Violations int64  `json:"violations"`
}

type liabilityLine struct {
	Check     string `json:"check"`
	Liability int64  `json:"liability"`
	Budget    int64  `json:"budget"`
	Spent     int64  `json:"spent"`
}

type summaryLine struct {
	Summary string   `json:"summary"`
	Failed  []string `json:"failed"`
}

// Run executes every check in one REPEATABLE READ READ ONLY transaction, so all
// of them see one snapshot, and writes one JSON line per check, the liability
// line, and a summary. ok is true when no check found a violation. The
// transaction is rolled back, never committed. When a query fails, nothing is
// written and the error carries the check name but no SQL.
func Run(ctx context.Context, pool *pgxpool.Pool, w io.Writer) (ok bool, err error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return false, fmt.Errorf("reconcile: begin: %w", redact(err))
	}
	defer func() { _ = tx.Rollback(ctx) }() // read only: nothing to commit, so rollback is the normal end

	lines := make([]any, 0, len(checks)+2)
	failed := make([]string, 0)
	for _, c := range checks {
		var n int64
		if err := tx.QueryRow(ctx, c.sql).Scan(&n); err != nil {
			return false, fmt.Errorf("reconcile: check %s: %w", c.name, redact(err))
		}
		if n != 0 {
			failed = append(failed, c.name)
		}
		lines = append(lines, checkLine{Check: c.name, Invariant: c.inv, OK: n == 0, Violations: n})
	}
	l := liabilityLine{Check: "liability"}
	if err := tx.QueryRow(ctx, liabilitySQL).Scan(&l.Liability, &l.Budget, &l.Spent); err != nil {
		return false, fmt.Errorf("reconcile: check liability: %w", redact(err))
	}
	lines = append(lines, l)

	summary := summaryLine{Summary: "ok", Failed: failed}
	if len(failed) > 0 {
		summary.Summary = "failed"
	}
	lines = append(lines, summary)

	enc := json.NewEncoder(w)
	for _, line := range lines {
		if err := enc.Encode(line); err != nil {
			return false, fmt.Errorf("reconcile: write output: %w", err)
		}
	}
	return len(failed) == 0, nil
}

// redact names why a statement failed without the driver text, which can echo
// the SQL or the host.
func redact(err error) error {
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr):
		return fmt.Errorf("SQLSTATE %s", pgErr.Code)
	case errors.Is(err, context.Canceled):
		return errors.New("canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("timeout")
	default:
		return errors.New("database error")
	}
}
