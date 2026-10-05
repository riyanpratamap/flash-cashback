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

// RedeemInput is one validated redemption command.
type RedeemInput struct {
	CampaignID string
	User       domain.UserID
	Key        uuid.UUID
	Hash       [32]byte
	Amount     int64

	Held func() // test hook, nil in production
}

// StoredRedemption is a committed redemption row, enough to rebuild its
// response. BalanceAfter is the value stored at redemption time, never the
// current balance.
type StoredRedemption struct {
	ID           int64
	Hash         [32]byte
	Amount       int64
	BalanceAfter int64
	Day          time.Time
	CreatedAt    time.Time
}

// FindRedemption reads the redemption stored for (user, key), if any.
func FindRedemption(ctx context.Context, q querier, user domain.UserID, key uuid.UUID) (StoredRedemption, bool, error) {
	var (
		sr   StoredRedemption
		hash []byte
	)
	err := q.QueryRow(ctx, `SELECT id, request_hash, amount, balance_after, campaign_day, created_at
		FROM redemptions WHERE user_id = $1 AND idempotency_key = $2`, string(user), key).Scan(
		&sr.ID, &hash, &sr.Amount, &sr.BalanceAfter, &sr.Day, &sr.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredRedemption{}, false, nil
	}
	if err != nil {
		return StoredRedemption{}, false, fmt.Errorf("find redemption: %w", err)
	}
	if len(hash) != len(sr.Hash) {
		return StoredRedemption{}, false, errors.New("find redemption: stored hash has the wrong length")
	}
	copy(sr.Hash[:], hash)
	return sr, true, nil
}

// RedeemOutcome is what the redemption transaction decided and wrote. Replay
// is set only together with ErrReplay.
type RedeemOutcome struct {
	Replay       *StoredRedemption
	ID           int64
	Day          time.Time // the campaign day, a database date
	CreatedAt    time.Time
	BalanceAfter int64
}

// Redeem decides and records one redemption inside tx (tech-spec §4.2 steps
// 4-10). The balance row is locked first, then the campaign is read without a
// lock (D46). A key that already has a redemption ends in ErrReplay: at step
// 5 under the balance lock, or at step 10 when the insert conflicts.
func Redeem(ctx context.Context, tx pgx.Tx, in RedeemInput) (RedeemOutcome, error) {
	// Step 4: a user with no row has balance 0 and nothing to lock.
	var balance int64
	err := tx.QueryRow(ctx, `SELECT balance FROM cashback_balances WHERE user_id = $1 FOR UPDATE`,
		string(in.User)).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) {
		balance = 0
	} else if err != nil {
		return RedeemOutcome{}, fmt.Errorf("lock balance: %w", err)
	}

	// Step 5: under the balance lock, a committed redemption for this key wins.
	if stored, found, err := FindRedemption(ctx, tx, in.User, in.Key); err != nil {
		return RedeemOutcome{}, err
	} else if found {
		return RedeemOutcome{Replay: &stored}, ErrReplay
	}

	// Step 6: a plain read, after the balance lock (D43, D46).
	var (
		paused bool
		now    time.Time
		day    time.Time
	)
	err = tx.QueryRow(ctx, `SELECT redemptions_paused, fc_now(), fc_campaign_day(fc_now())
		FROM campaigns WHERE id = $1`, in.CampaignID).Scan(&paused, &now, &day)
	if err != nil {
		return RedeemOutcome{}, fmt.Errorf("read campaign: %w", err)
	}
	// Step 7 before step 8: the switch is checked before the balance.
	if paused {
		return RedeemOutcome{}, domain.ErrRedemptionPaused
	}
	if in.Amount > balance {
		return RedeemOutcome{}, domain.ErrInsufficientBalance
	}

	// Step 9: the amount is at most the locked balance, so this stays >= 0.
	var balanceAfter int64
	err = tx.QueryRow(ctx, `UPDATE cashback_balances SET balance = balance - $1, updated_at = $2
		WHERE user_id = $3 RETURNING balance`, in.Amount, now, string(in.User)).Scan(&balanceAfter)
	if err != nil {
		return RedeemOutcome{}, fmt.Errorf("debit balance: %w", err)
	}

	out := RedeemOutcome{Day: day, CreatedAt: now, BalanceAfter: balanceAfter}
	err = tx.QueryRow(ctx, `INSERT INTO redemptions (campaign_id, user_id, idempotency_key, request_hash, amount,
			status, destination, balance_after, campaign_day, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT ON CONSTRAINT redemptions_user_key_unique DO NOTHING RETURNING id`,
		in.CampaignID, string(in.User), in.Key, in.Hash[:], in.Amount, domain.RedemptionCompleted,
		domain.DestinationMainAccount, balanceAfter, day, now).Scan(&out.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		// A concurrent commit won the key; the caller rolls back step 9.
		stored, found, err := FindRedemption(ctx, tx, in.User, in.Key)
		if err != nil {
			return RedeemOutcome{}, err
		}
		if !found {
			return RedeemOutcome{}, errors.New("insert redemption: conflict without a stored row")
		}
		return RedeemOutcome{Replay: &stored}, ErrReplay
	}
	if err != nil {
		return RedeemOutcome{}, fmt.Errorf("insert redemption: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ledger_entries (user_id, kind, amount, redemption_id, balance_after, created_at)
		VALUES ($1, 'REDEMPTION', $2, $3, $4, $5)`, string(in.User), -in.Amount, out.ID, balanceAfter, now); err != nil {
		return RedeemOutcome{}, fmt.Errorf("insert ledger: %w", err)
	}
	if in.Held != nil {
		in.Held()
	}
	return out, nil
}
