package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/riyanpratamap/flash-cashback/backend/internal/cache"
	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// The demo state (AC-57): user_a pays 940000 and redeems 32000, user_c pays
// 1000000, user_b does nothing.
const (
	demoPayA       = 940_000
	demoAwardA     = 47_000
	demoRedeemA    = 32_000
	demoPayC       = 1_000_000
	demoAwardC     = 50_000
	demoRequestTag = "demo-reset"
)

// ErrDemoStateNotReached: the services did not give the demo amounts, for
// example because the budget is below 97000.
var ErrDemoStateNotReached = errors.New("demo state not reached")

// Demo builds the demo state. It is never reachable from the API.
type Demo struct {
	tx       store.TxRunner
	payments *Payments
	redeems  *Redemptions
	inv      *cache.Invalidator
}

// NewDemo builds Demo; payments and redeems must share tx and inv.
func NewDemo(tx store.TxRunner, payments *Payments, redeems *Redemptions, inv *cache.Invalidator) *Demo {
	return &Demo{tx: tx, payments: payments, redeems: redeems, inv: inv}
}

// DemoResult is a finished reset. CacheErr is set when the cache keys could
// not be deleted: the database state stands and the key TTLs bound the
// staleness.
type DemoResult struct {
	At       string // WIB RFC 3339
	CacheErr error
}

// Reset truncates the money tables in one transaction, then builds the demo
// state through the services, so every row comes from the real rules, and
// last deletes every cache key (the truncate made every user's cashback body
// stale, not only the demo users'). The delete also runs when a step fails
// after the truncate.
func (d *Demo) Reset(ctx context.Context) (res DemoResult, err error) {
	var at string
	err = d.tx.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		t, err := store.DemoTruncate(ctx, tx, campaignID)
		at = domain.FormatTime(t)
		return err
	})
	if err != nil {
		return DemoResult{}, err
	}
	// The truncate has committed: every key is stale whatever happens next,
	// so the delete runs on every path, and a failed run can be repeated.
	defer func() { res.CacheErr = d.inv.DeleteAll(ctx) }()
	if err := d.pay(ctx, "user_a", demoPayA, demoAwardA); err != nil {
		return DemoResult{}, err
	}
	if err := d.redeem(ctx, "user_a", demoRedeemA); err != nil {
		return DemoResult{}, err
	}
	if err := d.pay(ctx, "user_c", demoPayC, demoAwardC); err != nil {
		return DemoResult{}, err
	}
	return DemoResult{At: at}, nil
}

func (d *Demo) pay(ctx context.Context, user domain.UserID, amount, wantAward int64) error {
	res, replayed, err := d.payments.Pay(ctx, domain.MoneyCommand{
		UserID: user, Key: uuid.New(), Amount: amount, Hash: domain.RequestHash(amount), RequestID: demoRequestTag})
	if err != nil {
		return fmt.Errorf("demo payment for %s: %w", user, err)
	}
	if replayed || res.Cashback.Awarded != wantAward || res.Cashback.Reason != domain.ReasonAwarded {
		return fmt.Errorf("%w: payment of %d for %s awarded %d (%s)", ErrDemoStateNotReached,
			amount, user, res.Cashback.Awarded, res.Cashback.Reason)
	}
	return nil
}

func (d *Demo) redeem(ctx context.Context, user domain.UserID, amount int64) error {
	_, replayed, err := d.redeems.Redeem(ctx, domain.MoneyCommand{
		UserID: user, Key: uuid.New(), Amount: amount, Hash: domain.RequestHash(amount), RequestID: demoRequestTag})
	if err != nil {
		return fmt.Errorf("demo redemption for %s: %w", user, err)
	}
	if replayed {
		return fmt.Errorf("%w: redemption for %s replayed", ErrDemoStateNotReached, user)
	}
	return nil
}
