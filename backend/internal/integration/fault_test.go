//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// fault is one injection: the statement whose text contains marker. The
// first skip matches pass through; later ones run before (if set) and then
// pass through, or fail with err when err is set.
type fault struct {
	marker string
	skip   int
	before func()
	err    error
}

// faultTx is a real transaction that can fail or pause chosen statements,
// to reach store error paths the database never produces on its own.
type faultTx struct {
	pgx.Tx
	f    fault
	seen int
}

// hit reports whether sql is the faulted statement, and runs before.
func (x *faultTx) hit(sql string) bool {
	if !strings.Contains(sql, x.f.marker) {
		return false
	}
	x.seen++
	if x.seen <= x.f.skip {
		return false
	}
	if x.f.before != nil {
		x.f.before()
	}
	return x.f.err != nil
}

func (x *faultTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if x.hit(sql) {
		return pgconn.CommandTag{}, x.f.err
	}
	return x.Tx.Exec(ctx, sql, args...)
}

func (x *faultTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if x.hit(sql) {
		return errRow{x.f.err}
	}
	return x.Tx.QueryRow(ctx, sql, args...)
}

type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

// inFaultTx runs fn on a faultTx inside a real transaction.
func inFaultTx(f fault, fn func(ctx context.Context, tx pgx.Tx) error) error {
	return runner(2000, 5000).InTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return fn(ctx, &faultTx{Tx: tx, f: f})
	})
}

var errInjected = errors.New("injected driver failure")

func payIn(user string, key uuid.UUID, amount int64) store.PayInput {
	return store.PayInput{CampaignID: "flash-cashback", User: domain.UserID(user), Key: key,
		Hash: domain.RequestHash(amount), Amount: amount}
}

func redeemIn(user string, key uuid.UUID, amount int64) store.RedeemInput {
	return store.RedeemInput{CampaignID: "flash-cashback", User: domain.UserID(user), Key: key,
		Hash: domain.RequestHash(amount), Amount: amount}
}

// insertZeroPayment commits, through the pool, a BELOW_MINIMUM payment for
// (user, key): a competitor that won the key and changes no balance.
func insertZeroPayment(t *testing.T, user string, key uuid.UUID) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `INSERT INTO payments (campaign_id, user_id, idempotency_key,
		request_hash, amount, status, cashback_awarded, cashback_reason, rate_bps, min_payment, daily_cap,
		campaign_day, created_at)
		SELECT id, $1, $2, decode(repeat('00', 32), 'hex'), 19999, 'SUCCEEDED', 0, 'BELOW_MINIMUM',
		rate_bps, min_payment, daily_cap, fc_campaign_day(fc_now()), fc_now() FROM campaigns
		RETURNING id`, user, key).Scan(&id)
	if err != nil {
		t.Fatalf("competing payment: %v", err)
	}
	return id
}

// Step 9 of a payment: the insert loses the key to a commit that landed
// after step 5. The result is ErrReplay with the winner's row, and every
// write the loser made (budget, earned, balance, ledger) is rolled back.
func TestPayInsertConflictIsReplay(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	key := uuid.New()
	var winner int64
	before := readCounts(t)

	var out store.PayOutcome
	err := inFaultTx(fault{marker: "INSERT INTO payments", before: func() {
		winner = insertZeroPayment(t, "user_a", key)
	}}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = store.Pay(ctx, tx, payIn("user_a", key, 100000))
		return err
	})
	if !errors.Is(err, store.ErrReplay) || out.Replay == nil {
		t.Fatalf("err = %v, replay = %v, want ErrReplay with the winner's row", err, out.Replay)
	}
	if r := out.Replay; r.ID != winner || r.Amount != 19999 || r.Awarded != 0 || r.Reason != domain.ReasonBelowMinimum {
		t.Errorf("replay = %+v, want the winner %d (19999, 0, BELOW_MINIMUM)", r, winner)
	}
	want := before
	want.payments++ // the winner only
	if after := readCounts(t); after != want {
		t.Errorf("counts = %+v, want %+v: the loser's writes must roll back", after, want)
	}
	assertReconciled(t)
}

// The conflict signal with no row behind it is an error, never a replay.
func TestPayConflictWithoutStoredRowIsError(t *testing.T) {
	reset(t, 10_000_000)
	before := readCounts(t)
	err := inFaultTx(fault{marker: "INSERT INTO payments", err: pgx.ErrNoRows},
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := store.Pay(ctx, tx, payIn("user_a", uuid.New(), 100000))
			return err
		})
	if err == nil || errors.Is(err, store.ErrReplay) || !strings.Contains(err.Error(), "conflict without a stored row") {
		t.Fatalf("err = %v, want a conflict-without-row error", err)
	}
	if after := readCounts(t); after != before {
		t.Errorf("counts = %+v, want %+v", after, before)
	}
}

// The re-read after a conflict can fail too: its error is returned as is.
func TestPayConflictRereadErrorIsReturned(t *testing.T) {
	reset(t, 10_000_000)
	// A fault matches one marker; the re-read is the second payments lookup,
	// so the insert conflict is made real by a committed winner.
	key := uuid.New()
	before := readCounts(t)
	err := inFaultTx(fault{marker: "FROM payments WHERE user_id", skip: 1, err: errInjected},
		func(ctx context.Context, tx pgx.Tx) error {
			// Step 5 passes (skip 1); the insert then conflicts.
			_, err := store.Pay(ctx, &conflictOnPaymentInsert{Tx: tx, t: t, key: key}, payIn("user_a", key, 100000))
			return err
		})
	if !errors.Is(err, errInjected) || !strings.Contains(err.Error(), "find payment") {
		t.Fatalf("err = %v, want the find payment failure", err)
	}
	want := before
	want.payments++ // the committed winner only: the loser's writes roll back
	if after := readCounts(t); after != want {
		t.Errorf("counts = %+v, want %+v: the loser's writes must roll back", after, want)
	}
	assertReconciled(t)
}

// conflictOnPaymentInsert commits a competing payment just before the insert.
type conflictOnPaymentInsert struct {
	pgx.Tx
	t   *testing.T
	key uuid.UUID
}

func (c *conflictOnPaymentInsert) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if strings.Contains(sql, "INSERT INTO payments") {
		insertZeroPayment(c.t, "user_a", c.key)
	}
	return c.Tx.QueryRow(ctx, sql, args...)
}

// Every driver failure inside Pay comes back wrapped with its step name and
// keeps the cause for errors.Is. Later steps also prove their earlier writes
// roll back; the first fault ("find payment") fires before any write.
func TestPayDriverErrorsAreWrappedAndRolledBack(t *testing.T) {
	for _, tc := range []struct {
		marker, wrap string
	}{
		{"FROM payments WHERE user_id", "find payment"},
		{"INSERT INTO user_daily_earnings", "insert user day"},
		{"SELECT earned, daily_cap FROM user_daily_earnings", "lock user day"},
		{"UPDATE campaigns SET spent", "spend budget"},
		{"UPDATE user_daily_earnings SET earned", "add earned"},
		{"INSERT INTO cashback_balances", "credit balance"},
		{"INSERT INTO payments", "insert payment"},
		{"INSERT INTO ledger_entries", "insert ledger"},
	} {
		t.Run(tc.wrap, func(t *testing.T) {
			reset(t, 10_000_000)
			before := readCounts(t)
			err := inFaultTx(fault{marker: tc.marker, err: errInjected}, func(ctx context.Context, tx pgx.Tx) error {
				_, err := store.Pay(ctx, tx, payIn("user_a", uuid.New(), 100000))
				return err
			})
			if !errors.Is(err, errInjected) || !strings.Contains(err.Error(), tc.wrap+":") {
				t.Fatalf("err = %v, want %q wrapping the cause", err, tc.wrap)
			}
			if after := readCounts(t); after != before {
				t.Errorf("counts = %+v, want %+v", after, before)
			}
			assertReconciled(t)
		})
	}
}

// An amount the rules reject is an "award" error and writes nothing.
func TestPayAwardRuleErrorIsWrapped(t *testing.T) {
	reset(t, 10_000_000)
	before := readCounts(t)
	err := runner(2000, 5000).InTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := store.Pay(ctx, tx, payIn("user_a", uuid.New(), -1))
		return err
	})
	if !errors.Is(err, domain.ErrOutOfRange) || !strings.Contains(err.Error(), "award:") {
		t.Fatalf("err = %v, want award wrapping ErrOutOfRange", err)
	}
	if after := readCounts(t); after != before {
		t.Errorf("counts = %+v, want %+v", after, before)
	}
}

// Step 10 of a redemption: the insert loses the key to a commit that landed
// after step 5. The loser's debit is rolled back; the winner's row is the replay.
func TestRedeemInsertConflictIsReplay(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	fund(t, "user_a", 18000)
	key := uuid.New()
	var winner int64
	before := readRedeemShape(t, "user_a")

	var out store.RedeemOutcome
	err := inFaultTx(fault{marker: "INSERT INTO redemptions", before: func() {
		winner = insertRedemption(t, "user_a", key.String(), 5000, 13000, "2026-10-03")
	}}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = store.Redeem(ctx, tx, redeemIn("user_a", key, 5000))
		return err
	})
	if !errors.Is(err, store.ErrReplay) || out.Replay == nil {
		t.Fatalf("err = %v, replay = %v, want ErrReplay with the winner's row", err, out.Replay)
	}
	if r := out.Replay; r.ID != winner || r.Amount != 5000 || r.BalanceAfter != 13000 {
		t.Errorf("replay = %+v, want the winner %d (5000, balance_after 13000)", r, winner)
	}
	want := before
	want.redemptions++ // the winner only: no debit, no ledger entry from the loser
	if after := readRedeemShape(t, "user_a"); after != want {
		t.Errorf("shape = %+v, want %+v: the loser's debit must roll back", after, want)
	}
	// Finish the winner's books, as its own transaction would have, then prove them.
	execSQL(t, `UPDATE cashback_balances SET balance = 13000 WHERE user_id = 'user_a'`)
	insertRedemptionEntry(t, "user_a", winner, -5000, 13000)
	assertReconciled(t)
}

func TestRedeemConflictWithoutStoredRowIsError(t *testing.T) {
	reset(t, 10_000_000)
	fund(t, "user_a", 18000)
	before := readRedeemShape(t, "user_a")
	err := inFaultTx(fault{marker: "INSERT INTO redemptions", err: pgx.ErrNoRows},
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := store.Redeem(ctx, tx, redeemIn("user_a", uuid.New(), 5000))
			return err
		})
	if err == nil || errors.Is(err, store.ErrReplay) || !strings.Contains(err.Error(), "conflict without a stored row") {
		t.Fatalf("err = %v, want a conflict-without-row error", err)
	}
	if after := readRedeemShape(t, "user_a"); after != before {
		t.Errorf("shape = %+v, want %+v", after, before)
	}
}

type conflictOnRedemptionInsert struct {
	pgx.Tx
	t   *testing.T
	key uuid.UUID
}

func (c *conflictOnRedemptionInsert) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if strings.Contains(sql, "INSERT INTO redemptions") {
		insertRedemption(c.t, "user_a", c.key.String(), 5000, 13000, "2026-10-03")
	}
	return c.Tx.QueryRow(ctx, sql, args...)
}

func TestRedeemConflictRereadErrorIsReturned(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	fund(t, "user_a", 18000)
	key := uuid.New()
	before := readRedeemShape(t, "user_a")
	err := inFaultTx(fault{marker: "FROM redemptions WHERE user_id", skip: 1, err: errInjected},
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := store.Redeem(ctx, &conflictOnRedemptionInsert{Tx: tx, t: t, key: key}, redeemIn("user_a", key, 5000))
			return err
		})
	if !errors.Is(err, errInjected) || !strings.Contains(err.Error(), "find redemption") {
		t.Fatalf("err = %v, want the find redemption failure", err)
	}
	want := before
	want.redemptions++ // the committed winner only: no debit, no ledger entry from the loser
	if after := readRedeemShape(t, "user_a"); after != want {
		t.Errorf("shape = %+v, want %+v: the loser's debit must roll back", after, want)
	}
	// Finish the winner's books, as its own transaction would have, then prove them.
	var winner int64
	if err := pool.QueryRow(context.Background(), `SELECT id FROM redemptions WHERE user_id = 'user_a'`).Scan(&winner); err != nil {
		t.Fatalf("winner id: %v", err)
	}
	execSQL(t, `UPDATE cashback_balances SET balance = 13000 WHERE user_id = 'user_a'`)
	insertRedemptionEntry(t, "user_a", winner, -5000, 13000)
	assertReconciled(t)
}

func TestRedeemDriverErrorsAreWrappedAndRolledBack(t *testing.T) {
	for _, tc := range []struct {
		marker, wrap string
	}{
		{"FROM cashback_balances WHERE user_id = $1 FOR UPDATE", "lock balance"},
		{"FROM redemptions WHERE user_id", "find redemption"},
		{"redemptions_paused, fc_now()", "read campaign"},
		{"UPDATE cashback_balances SET balance = balance - $1", "debit balance"},
		{"INSERT INTO redemptions", "insert redemption"},
		{"INSERT INTO ledger_entries", "insert ledger"},
	} {
		t.Run(tc.wrap, func(t *testing.T) {
			reset(t, 10_000_000)
			fund(t, "user_a", 18000)
			before := readRedeemShape(t, "user_a")
			err := inFaultTx(fault{marker: tc.marker, err: errInjected}, func(ctx context.Context, tx pgx.Tx) error {
				_, err := store.Redeem(ctx, tx, redeemIn("user_a", uuid.New(), 5000))
				return err
			})
			if !errors.Is(err, errInjected) || !strings.Contains(err.Error(), tc.wrap+":") {
				t.Fatalf("err = %v, want %q wrapping the cause", err, tc.wrap)
			}
			if after := readRedeemShape(t, "user_a"); after != before {
				t.Errorf("shape = %+v, want %+v", after, before)
			}
			assertReconciled(t)
		})
	}
}

// fakeQuerier answers every read with a canned row.
type fakeQuerier struct{ row pgx.Row }

func (q fakeQuerier) QueryRow(context.Context, string, ...any) pgx.Row { return q.row }

// shortHashRow scans a 31-byte hash into the second destination.
type shortHashRow struct{}

func (shortHashRow) Scan(dest ...any) error {
	*(dest[1].(*[]byte)) = make([]byte, 31)
	return nil
}

func TestFindRejectsOddRows(t *testing.T) {
	ctx := context.Background()
	key := uuid.New()
	for _, tc := range []struct {
		name string
		row  pgx.Row
		want string
	}{
		{"driver error", errRow{errInjected}, "find"},
		{"short hash", shortHashRow{}, "wrong length"},
	} {
		t.Run("payment "+tc.name, func(t *testing.T) {
			_, found, err := store.FindPayment(ctx, fakeQuerier{tc.row}, "user_a", key)
			if err == nil || found || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("found = %v, err = %v, want an error containing %q", found, err, tc.want)
			}
		})
		t.Run("redemption "+tc.name, func(t *testing.T) {
			_, found, err := store.FindRedemption(ctx, fakeQuerier{tc.row}, "user_a", key)
			if err == nil || found || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("found = %v, err = %v, want an error containing %q", found, err, tc.want)
			}
		})
	}
}

func TestSetSwitchErrors(t *testing.T) {
	reset(t, 10_000_000)
	for _, tc := range []struct {
		name   string
		sw     domain.Switch
		marker string
		want   string
	}{
		{"unknown switch", domain.Switch("bogus"), "", "unknown switch"},
		{"lock", domain.SwitchAwards, "FOR NO KEY UPDATE", "lock campaign:"},
		{"update awards", domain.SwitchAwards, "UPDATE campaigns SET awards_paused", "update switch:"},
		{"update redemptions", domain.SwitchRedemptions, "UPDATE campaigns SET redemptions_paused", "update switch:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fault{marker: tc.marker, err: errInjected}
			if tc.marker == "" {
				f = fault{marker: "never matches in SQL text"}
			}
			err := inFaultTx(f, func(ctx context.Context, tx pgx.Tx) error {
				_, err := store.SetSwitch(ctx, tx, "flash-cashback", tc.sw, true)
				return err
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if tc.marker != "" && !errors.Is(err, errInjected) {
				t.Errorf("err = %v, want the cause kept", err)
			}
			if got := queryInt(t, `SELECT count(*) FROM campaigns WHERE awards_paused OR redemptions_paused`); got != 0 {
				t.Errorf("%d campaigns paused, want 0: the fault fires before the change, so nothing is paused", got)
			}
		})
	}
}

func TestDemoTruncateErrorsAreWrapped(t *testing.T) {
	for _, tc := range []struct{ marker, want string }{
		{"TRUNCATE", "truncate money tables:"},
		{"UPDATE campaigns SET spent = 0", "reset campaign:"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			reset(t, 10_000_000)
			pay(t, "user_a", 100000)
			before := readCounts(t)
			err := inFaultTx(fault{marker: tc.marker, err: errInjected}, func(ctx context.Context, tx pgx.Tx) error {
				_, err := store.DemoTruncate(ctx, tx, "flash-cashback")
				return err
			})
			if !errors.Is(err, errInjected) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q wrapping the cause", err, tc.want)
			}
			if after := readCounts(t); after != before {
				t.Errorf("counts = %+v, want %+v", after, before)
			}
		})
	}
}

func TestNewestQueryErrorIsWrapped(t *testing.T) {
	reset(t, 10_000_000)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rows, err := store.Newest(ctx, pool, "user_a", 10)
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "read history:") || rows != nil {
		t.Fatalf("rows = %v, err = %v, want read history wrapping context.Canceled", rows, err)
	}
}

// A function that fails after its connection died: the outcome is unknown,
// not the plain error, because nothing says the rollback reached the server.
func TestTxLostConnectionIsUnknownOutcome(t *testing.T) {
	reset(t, 10_000_000)
	err := runner(2000, 5000).InTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		var pid int
		if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `SELECT pg_terminate_backend($1)`, pid); err != nil {
			return err
		}
		// pgx learns the connection is gone on its next use; wait for that.
		deadline := time.Now().Add(5 * time.Second)
		for !tx.Conn().IsClosed() && time.Now().Before(deadline) {
			_, _ = tx.Exec(ctx, `SELECT 1`)
			time.Sleep(5 * time.Millisecond)
		}
		return errors.New("work failed")
	})
	if !errors.Is(err, store.ErrUnknownOutcome) {
		t.Fatalf("err = %v, want ErrUnknownOutcome", err)
	}
}
