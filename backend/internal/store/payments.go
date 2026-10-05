package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
)

// PayInput is one validated payment command.
type PayInput struct {
	CampaignID string
	User       domain.UserID
	Key        uuid.UUID
	Hash       [32]byte
	Amount     int64
}

// PayOutcome is what the payment transaction decided and wrote.
type PayOutcome struct {
	ID        int64
	Day       time.Time // the campaign day, a database date
	CreatedAt time.Time
	Awarded   int64
	Reason    domain.Reason
}

// Pay decides and records one payment inside tx (tech-spec §4.1 steps 4 and
// 6-9). Locks go in the D44 order: campaign row, user-day row, balance row.
// Every value the decision uses comes from the locked reads. Replay handling
// (steps 2, 5 and the conflict path of 9) is not here yet.
func Pay(ctx context.Context, tx pgx.Tx, in PayInput) (PayOutcome, error) {
	var (
		rateBps                         int
		minPayment, campaignCap, budget int64
		spent                           int64
		paused                          bool
		now                             time.Time
		day                             time.Time
	)
	err := tx.QueryRow(ctx, `SELECT rate_bps, min_payment, daily_cap, budget, spent, awards_paused,
			fc_now(), fc_campaign_day(fc_now())
		FROM campaigns WHERE id = $1 FOR NO KEY UPDATE`, in.CampaignID).Scan(
		&rateBps, &minPayment, &campaignCap, &budget, &spent, &paused, &now, &day)
	if err != nil {
		return PayOutcome{}, fmt.Errorf("read campaign: %w", err)
	}

	// The row must exist before it can be locked.
	if _, err := tx.Exec(ctx, `INSERT INTO user_daily_earnings (campaign_id, user_id, day, earned, daily_cap)
		VALUES ($1, $2, $3, 0, $4) ON CONFLICT DO NOTHING`, in.CampaignID, string(in.User), day, campaignCap); err != nil {
		return PayOutcome{}, fmt.Errorf("insert user day: %w", err)
	}
	var earned, userCap int64
	err = tx.QueryRow(ctx, `SELECT earned, daily_cap FROM user_daily_earnings
		WHERE campaign_id = $1 AND user_id = $2 AND day = $3 FOR UPDATE`,
		in.CampaignID, string(in.User), day).Scan(&earned, &userCap)
	if err != nil {
		return PayOutcome{}, fmt.Errorf("lock user day: %w", err)
	}

	// AC-71: the user-day row's cap, not the campaign's.
	rule := domain.Rule{RateBPS: int64(rateBps), MinPayment: minPayment, DailyCap: userCap}
	awarded, reason, err := domain.Award(in.Amount, rule, earned, budget, spent, paused)
	if err != nil {
		return PayOutcome{}, fmt.Errorf("award: %w", err)
	}

	var balance int64
	if awarded > 0 {
		if _, err := tx.Exec(ctx, `UPDATE campaigns SET spent = spent + $1, updated_at = $2 WHERE id = $3`,
			awarded, now, in.CampaignID); err != nil {
			return PayOutcome{}, fmt.Errorf("spend budget: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE user_daily_earnings SET earned = earned + $1
			WHERE campaign_id = $2 AND user_id = $3 AND day = $4`,
			awarded, in.CampaignID, string(in.User), day); err != nil {
			return PayOutcome{}, fmt.Errorf("add earned: %w", err)
		}
		err = tx.QueryRow(ctx, `INSERT INTO cashback_balances (user_id, balance, updated_at)
			VALUES ($1, $2, $3)
			ON CONFLICT (user_id) DO UPDATE
			SET balance = cashback_balances.balance + EXCLUDED.balance, updated_at = EXCLUDED.updated_at
			RETURNING balance`, string(in.User), awarded, now).Scan(&balance)
		if err != nil {
			return PayOutcome{}, fmt.Errorf("credit balance: %w", err)
		}
	}

	out := PayOutcome{Day: day, CreatedAt: now, Awarded: awarded, Reason: reason}
	err = tx.QueryRow(ctx, `INSERT INTO payments (campaign_id, user_id, idempotency_key, request_hash, amount,
			status, cashback_awarded, cashback_reason, rate_bps, min_payment, daily_cap, campaign_day, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING id`,
		in.CampaignID, string(in.User), in.Key, in.Hash[:], in.Amount, domain.PaymentStatusSucceeded,
		awarded, string(reason), rateBps, minPayment, userCap, day, now).Scan(&out.ID)
	if err != nil {
		return PayOutcome{}, fmt.Errorf("insert payment: %w", err)
	}
	if awarded > 0 {
		if _, err := tx.Exec(ctx, `INSERT INTO ledger_entries (user_id, kind, amount, payment_id, balance_after, created_at)
			VALUES ($1, 'AWARD', $2, $3, $4, $5)`, string(in.User), awarded, out.ID, balance, now); err != nil {
			return PayOutcome{}, fmt.Errorf("insert ledger: %w", err)
		}
	}
	return out, nil
}
