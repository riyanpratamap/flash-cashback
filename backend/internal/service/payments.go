package service

import (
	"context"
	"log/slog"

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
// log line is written after COMMIT only (tech-spec §4.1 step 10, §7).
func (p *Payments) Pay(ctx context.Context, cmd domain.MoneyCommand) (domain.PaymentResult, error) {
	in := store.PayInput{CampaignID: campaignID, User: cmd.UserID, Key: cmd.Key, Hash: cmd.Hash, Amount: cmd.Amount}
	var out store.PayOutcome
	err := p.tx.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = store.Pay(ctx, tx, in)
		return err
	})
	if err != nil {
		return domain.PaymentResult{}, err
	}
	p.log.Info("money write", "request_id", cmd.RequestID, "op", "payment", "user_id", string(cmd.UserID), "payment_id", out.ID,
		"amount", cmd.Amount, "awarded", out.Awarded, "reason", string(out.Reason), "replayed", false)
	return domain.PaymentResult{
		Payment: domain.PaymentView{
			ID:        out.ID,
			Reference: domain.Reference(domain.RefPayment, out.Day, out.ID),
			Amount:    cmd.Amount,
			Status:    domain.PaymentStatusSucceeded,
			CreatedAt: domain.FormatTime(out.CreatedAt),
		},
		Cashback: domain.AwardView{Awarded: out.Awarded, Reason: out.Reason},
	}, nil
}
