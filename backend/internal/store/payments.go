package store

import (
	"context"
	"errors"
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

	AfterCampaignRead func() // test hook, nil in production
}

// ErrReplay: the key already has a payment or a redemption. Pay and Redeem
// return it with the outcome's Replay set so InTx rolls back whatever this
// attempt wrote; the outcome type tells which operation it was.
var ErrReplay = errors.New("store: key already recorded")

// StoredPayment is a committed payment row, enough to rebuild its response.
type StoredPayment struct {
	ID        int64
	Hash      [32]byte
	Amount    int64
	Awarded   int64
	Reason    domain.Reason
	Day       time.Time
	CreatedAt time.Time
}

// querier is the one read FindPayment needs; *pgxpool.Pool and pgx.Tx satisfy it.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// FindPayment reads the payment stored for (user, key), if any.
func FindPayment(ctx context.Context, q querier, user domain.UserID, key uuid.UUID) (StoredPayment, bool, error) {
	var (
		sp     StoredPayment
		hash   []byte
		reason string
	)
	err := q.QueryRow(ctx, `SELECT id, request_hash, amount, cashback_awarded, cashback_reason, campaign_day, created_at
		FROM payments WHERE user_id = $1 AND idempotency_key = $2`, string(user), key).Scan(
		&sp.ID, &hash, &sp.Amount, &sp.Awarded, &reason, &sp.Day, &sp.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredPayment{}, false, nil
	}
	if err != nil {
		return StoredPayment{}, false, fmt.Errorf("find payment: %w", err)
	}
	if len(hash) != len(sp.Hash) {
		return StoredPayment{}, false, errors.New("find payment: stored hash has the wrong length")
	}
	copy(sp.Hash[:], hash)
	sp.Reason = domain.Reason(reason)
	return sp, true, nil
}

// PayOutcome is what the payment transaction decided and wrote. Replay is set
// only together with ErrReplay.
type PayOutcome struct {
	Replay    *StoredPayment
	ID        int64
	Day       time.Time // the campaign day, a database date
	CreatedAt time.Time
	Awarded   int64
	Reason    domain.Reason
}

// Pay decides and records one payment inside tx (tech-spec §4.1 steps 4 and
// 6-9). Locks go in the D44 order: campaign row, user-day row, balance row.
// Every value the decision uses comes from the locked reads. A key that
// already has a payment ends in ErrReplay: at step 5 under the campaign lock,
// or at step 9 when the insert conflicts.
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

	// Step 5: under the campaign lock, a committed payment for this key wins.
	if stored, found, err := FindPayment(ctx, tx, in.User, in.Key); err != nil {
		return PayOutcome{}, err
	} else if found {
		return PayOutcome{Replay: &stored}, ErrReplay
	}

	if in.AfterCampaignRead != nil {
		in.AfterCampaignRead()
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
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT ON CONSTRAINT payments_user_key_unique DO NOTHING RETURNING id`,
		in.CampaignID, string(in.User), in.Key, in.Hash[:], in.Amount, domain.PaymentStatusSucceeded,
		awarded, string(reason), rateBps, minPayment, userCap, day, now).Scan(&out.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		// A concurrent commit won the key; the caller rolls back steps 6-8.
		stored, found, err := FindPayment(ctx, tx, in.User, in.Key)
		if err != nil {
			return PayOutcome{}, err
		}
		if !found {
			return PayOutcome{}, errors.New("insert payment: conflict without a stored row")
		}
		return PayOutcome{Replay: &stored}, ErrReplay
	}
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
