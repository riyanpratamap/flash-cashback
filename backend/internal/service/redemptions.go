package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/riyanpratamap/flash-cashback/backend/internal/cache"
	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// Redemptions is the redemption use case.
type Redemptions struct {
	tx  store.TxRunner
	inv *cache.Invalidator
	log *slog.Logger
}

// NewRedemptions builds Redemptions on the money transaction runner and the
// cache invalidator it calls after COMMIT. A nil logger uses the default one.
func NewRedemptions(tx store.TxRunner, inv *cache.Invalidator, log *slog.Logger) *Redemptions {
	if log == nil {
		log = slog.Default()
	}
	return &Redemptions{tx: tx, inv: inv, log: log}
}

// Redeem moves cashback out of the balance in one transaction and logs the
// outcome. The bool is true when the key already had a redemption and its
// stored result is returned (tech-spec §4.2 steps 2, 5, 10). The log line is
// written after COMMIT only (§7); a replay logs too, with replayed=true.
func (p *Redemptions) Redeem(ctx context.Context, cmd domain.MoneyCommand) (domain.RedemptionResult, bool, error) {
	// Step 2: a committed redemption answers without a lock, even while paused.
	stored, found, err := store.FindRedemption(ctx, p.tx.Pool, cmd.UserID, cmd.Key)
	if err != nil {
		return domain.RedemptionResult{}, false, fmt.Errorf("fast replay lookup: %w", err)
	}
	if found {
		return p.replay(stored, cmd)
	}

	in := store.RedeemInput{CampaignID: campaignID, User: cmd.UserID, Key: cmd.Key, Hash: cmd.Hash, Amount: cmd.Amount,
		Held: p.tx.Hooks.RedeemHeld}
	var out store.RedeemOutcome
	err = p.tx.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = store.Redeem(ctx, tx, in)
		return err
	})
	if errors.Is(err, store.ErrReplay) && out.Replay != nil {
		return p.replay(*out.Replay, cmd)
	}
	if errors.Is(err, store.ErrUnknownOutcome) {
		// The commit may have landed: a delete is safe either way (§6).
		p.inv.Delete(ctx, cache.CashbackKey(cmd.UserID))
	}
	if err != nil {
		return domain.RedemptionResult{}, false, err
	}
	// After COMMIT, on a context of its own (§4.2 step 12, §6).
	p.inv.Delete(ctx, cache.CashbackKey(cmd.UserID))
	payout(cmd)
	p.logMoney(cmd, out.ID, cmd.Amount, false)
	return redemptionResult(out.ID, out.Day, out.CreatedAt, cmd.Amount, out.BalanceAfter), false, nil
}

// replay answers from a stored row: 409 when the body differs, else the
// original result rebuilt from the row, with the balance_after stored then.
func (p *Redemptions) replay(stored store.StoredRedemption, cmd domain.MoneyCommand) (domain.RedemptionResult, bool, error) {
	if stored.Hash != cmd.Hash {
		return domain.RedemptionResult{}, false, domain.ErrIdempotencyKeyReused
	}
	p.logMoney(cmd, stored.ID, stored.Amount, true)
	return redemptionResult(stored.ID, stored.Day, stored.CreatedAt, stored.Amount, stored.BalanceAfter), true, nil
}

// payout is the main-account transfer. It is a stub that completes at once
// and makes no call: TC13 is stated only, so no PENDING or FAILED state exists.
func payout(domain.MoneyCommand) {}

func (p *Redemptions) logMoney(cmd domain.MoneyCommand, id, amount int64, replayed bool) {
	p.log.Info("money write", "request_id", cmd.RequestID, "op", "redeem", "user_id", string(cmd.UserID),
		"redemption_id", id, "amount", amount, "replayed", replayed)
}

func redemptionResult(id int64, day, createdAt time.Time, amount, balanceAfter int64) domain.RedemptionResult {
	return domain.RedemptionResult{
		Redemption: domain.RedemptionView{
			ID:          id,
			Reference:   domain.Reference(domain.RefRedemption, day, id),
			Amount:      amount,
			Status:      domain.RedemptionCompleted,
			Destination: domain.DestinationMainAccount,
			CreatedAt:   domain.FormatTime(createdAt),
		},
		BalanceAfter: balanceAfter,
	}
}
