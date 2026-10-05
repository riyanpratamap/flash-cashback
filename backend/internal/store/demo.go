package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// DemoTruncate empties the five money tables, restarts their identities, and
// puts the campaign back to spent 0 with both switches off; the budget stays
// (tech-spec §9). It is the one exception to INV-06 and to the rule that
// spent never falls, reachable only through demo-reset behind FC_DEMO=1
// (AC-57). TRUNCATE fires no row trigger, so the append-only guards stay in
// place for every other statement. It returns the transaction's now().
func DemoTruncate(ctx context.Context, tx pgx.Tx, campaignID string) (time.Time, error) {
	if _, err := tx.Exec(ctx, `TRUNCATE user_daily_earnings, payments, redemptions, cashback_balances,
		ledger_entries RESTART IDENTITY`); err != nil {
		return time.Time{}, fmt.Errorf("truncate money tables: %w", err)
	}
	var at time.Time
	err := tx.QueryRow(ctx, `UPDATE campaigns SET spent = 0, awards_paused = false, redemptions_paused = false,
		updated_at = now() WHERE id = $1 RETURNING updated_at`, campaignID).Scan(&at)
	if err != nil {
		return time.Time{}, fmt.Errorf("reset campaign: %w", err)
	}
	return at, nil
}
