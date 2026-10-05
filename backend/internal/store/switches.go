package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
)

// The two statements are fixed text; the switch picks one, so no column name
// ever comes from input.
const (
	selectAwardsSwitch = `SELECT awards_paused, now() FROM campaigns WHERE id = $1 FOR NO KEY UPDATE`
	updateAwardsSwitch = `UPDATE campaigns SET awards_paused = $2, updated_at = now() WHERE id = $1`

	selectRedemptionsSwitch = `SELECT redemptions_paused, now() FROM campaigns WHERE id = $1 FOR NO KEY UPDATE`
	updateRedemptionsSwitch = `UPDATE campaigns SET redemptions_paused = $2, updated_at = now() WHERE id = $1`
)

// SwitchOutcome is what a switch change found and did.
type SwitchOutcome struct {
	Old     bool
	Changed bool
	At      time.Time // the transaction's now()
}

// SetSwitch sets one switch inside tx (tech-spec §4.3). The campaign row is
// locked FOR NO KEY UPDATE, the lock an award holds, so the change waits for
// in-flight awards and no award straddles it. A value that already holds
// keeps the lock but skips the UPDATE, so updated_at stays.
func SetSwitch(ctx context.Context, tx pgx.Tx, campaignID string, sw domain.Switch, paused bool) (SwitchOutcome, error) {
	var sel, upd string
	switch sw {
	case domain.SwitchAwards:
		sel, upd = selectAwardsSwitch, updateAwardsSwitch
	case domain.SwitchRedemptions:
		sel, upd = selectRedemptionsSwitch, updateRedemptionsSwitch
	default:
		return SwitchOutcome{}, fmt.Errorf("set switch: unknown switch %q", sw)
	}
	var out SwitchOutcome
	if err := tx.QueryRow(ctx, sel, campaignID).Scan(&out.Old, &out.At); err != nil {
		return SwitchOutcome{}, fmt.Errorf("lock campaign: %w", err)
	}
	if out.Old == paused {
		return out, nil
	}
	if _, err := tx.Exec(ctx, upd, campaignID, paused); err != nil {
		return SwitchOutcome{}, fmt.Errorf("update switch: %w", err)
	}
	out.Changed = true
	return out, nil
}
