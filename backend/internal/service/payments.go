package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// Payments is the payment use case.
type Payments struct {
	tx  store.TxRunner
	log *slog.Logger
}

// NewPayments builds Payments on the money transaction runner. A nil logger
// uses the default one.
func NewPayments(tx store.TxRunner, log *slog.Logger) *Payments {
	if log == nil {
		log = slog.Default()
	}
	return &Payments{tx: tx, log: log}
}

// Pay decides one payment in one transaction and logs the outcome. The
// bool is true when the key already had a payment and its stored result is
// returned (tech-spec §4.1 steps 2, 5, 9). The log line is written after
// COMMIT only (step 10, §7); a replay logs too, with replayed=true.
func (p *Payments) Pay(ctx context.Context, cmd domain.MoneyCommand) (domain.PaymentResult, bool, error) {
	// Step 2: a committed payment answers without taking any lock.
	stored, found, err := store.FindPayment(ctx, p.tx.Pool, cmd.UserID, cmd.Key)
	if err != nil {
		return domain.PaymentResult{}, false, fmt.Errorf("fast replay lookup: %w", err)
	}
	if found {
		return p.replay(stored, cmd)
	}

	in := store.PayInput{CampaignID: campaignID, User: cmd.UserID, Key: cmd.Key, Hash: cmd.Hash, Amount: cmd.Amount,
		AfterCampaignRead: p.tx.Hooks.AfterCampaignRead}
	var out store.PayOutcome
	err = p.tx.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = store.Pay(ctx, tx, in)
		return err
	})
	if errors.Is(err, store.ErrReplay) && out.Replay != nil {
		return p.replay(*out.Replay, cmd)
	}
	if err != nil {
		return domain.PaymentResult{}, false, err
	}
	p.logMoney(cmd, out.ID, out.Awarded, out.Reason, false)
	return result(out.ID, out.Day, out.CreatedAt, cmd.Amount, out.Awarded, out.Reason), false, nil
}

// replay answers from a stored row: 409 when the body differs, else the
// original result rebuilt from the row.
func (p *Payments) replay(stored store.StoredPayment, cmd domain.MoneyCommand) (domain.PaymentResult, bool, error) {
	if stored.Hash != cmd.Hash {
		return domain.PaymentResult{}, false, domain.ErrIdempotencyKeyReused
	}
	p.logMoney(cmd, stored.ID, stored.Awarded, stored.Reason, true)
	return result(stored.ID, stored.Day, stored.CreatedAt, stored.Amount, stored.Awarded, stored.Reason), true, nil
}

func (p *Payments) logMoney(cmd domain.MoneyCommand, id, awarded int64, reason domain.Reason, replayed bool) {
	p.log.Info("money write", "request_id", cmd.RequestID, "op", "payment", "user_id", string(cmd.UserID), "payment_id", id,
		"amount", cmd.Amount, "awarded", awarded, "reason", string(reason), "replayed", replayed)
}

func result(id int64, day, createdAt time.Time, amount, awarded int64, reason domain.Reason) domain.PaymentResult {
	return domain.PaymentResult{
		Payment: domain.PaymentView{
			ID:        id,
			Reference: domain.Reference(domain.RefPayment, day, id),
			Amount:    amount,
			Status:    domain.PaymentStatusSucceeded,
			CreatedAt: domain.FormatTime(createdAt),
		},
		Cashback: domain.AwardView{Awarded: awarded, Reason: reason},
	}
}
