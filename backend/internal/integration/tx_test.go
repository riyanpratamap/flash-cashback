//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

type txFn = func(ctx context.Context, tx pgx.Tx) error

func runner(lockMS, stmtMS int64) store.TxRunner {
	return store.TxRunner{Pool: pool, LockTimeoutMS: lockMS, StatementTimeoutMS: stmtMS}
}

// mark writes a row that is visible only if the transaction committed.
func mark(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `INSERT INTO user_daily_earnings (campaign_id, user_id, day, earned, daily_cap)
		SELECT id, 'tx_marker', current_date, 0, daily_cap FROM campaigns`)
	return err
}

func markers(t *testing.T) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM user_daily_earnings WHERE user_id = 'tx_marker'`).Scan(&n); err != nil {
		t.Fatalf("count markers: %v", err)
	}
	return n
}

// markThen writes the marker, then runs step.
func markThen(step txFn) txFn {
	return func(ctx context.Context, tx pgx.Tx) error {
		if err := mark(ctx, tx); err != nil {
			return err
		}
		return step(ctx, tx)
	}
}

func exec(sql string) txFn {
	return func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql)
		return err
	}
}

func wantNothingCommitted(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if n := markers(t); n != 0 {
		t.Fatalf("%d marker rows committed, want 0", n)
	}
}

// AC-27: a lock wait that exceeds lock_timeout is ErrBusy and rolls back.
func TestTxLockTimeoutIsBusy(t *testing.T) {
	reset(t, 10_000_000)
	ctx := context.Background()
	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = holder.Rollback(context.Background()) })
	if _, err := holder.Exec(ctx, `SELECT 1 FROM campaigns FOR NO KEY UPDATE`); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err = runner(100, 10_000).InTx(ctx, markThen(exec(`SELECT 1 FROM campaigns FOR NO KEY UPDATE`)))
	wantNothingCommitted(t, err, store.ErrBusy)
	// The 5 s cap would also say busy; only lock_timeout gives up this early.
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("gave up after %v, lock_timeout is 100 ms", d)
	}
	if errors.Is(err, store.ErrDeadlock) {
		t.Fatal("a lock timeout is not a deadlock")
	}
}

// AC-27: statement_timeout (57014) is ErrBusy.
func TestTxStatementTimeoutIsBusy(t *testing.T) {
	reset(t, 10_000_000)
	err := runner(1000, 100).InTx(context.Background(), markThen(exec(`SELECT pg_sleep(1)`)))
	wantNothingCommitted(t, err, store.ErrBusy)
}

// AC-27: the overall cap expiring before COMMIT is ErrBusy.
func TestTxCapBeforeCommitIsBusy(t *testing.T) {
	reset(t, 10_000_000)
	r := runner(1000, 10_000)
	r.Cap = 200 * time.Millisecond
	err := r.InTx(context.Background(), markThen(exec(`SELECT pg_sleep(1)`)))
	wantNothingCommitted(t, err, store.ErrBusy)
}

// TC3: a deadlock (40P01) is ErrBusy flagged as ErrDeadlock; the other
// transaction commits.
func TestRaceTxDeadlockIsBusyAndDistinguishable(t *testing.T) {
	reset(t, 10_000_000)
	r := runner(5000, 10_000) // above the 1 s deadlock detector
	// Barrier: each side closes its own channel once it holds its first lock,
	// then waits for the other's. A side that fails first returns before
	// closing, so the waiter ends on ctx.Done or the timeout, never forever.
	ready := [2]chan struct{}{make(chan struct{}), make(chan struct{})}
	run := func(me, first, second int) error {
		return r.InTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, first); err != nil {
				return err
			}
			close(ready[me])
			select {
			case <-ready[1-me]:
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				return errors.New("peer never held its first lock")
			}
			_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, second)
			return err
		})
	}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, keys := range [][2]int{{1, 2}, {2, 1}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = run(i, keys[0], keys[1])
		}()
	}
	wg.Wait()
	var nilN, deadN int
	for _, err := range errs {
		switch {
		case err == nil:
			nilN++
		case errors.Is(err, store.ErrBusy) && errors.Is(err, store.ErrDeadlock):
			deadN++
		default:
			t.Fatalf("unexpected error %v", err)
		}
	}
	if nilN != 1 || deadN != 1 {
		t.Fatalf("committed %d, deadlocked %d; want 1 and 1 (%v)", nilN, deadN, errs)
	}
}

// A zero timeout would be '0ms', which PostgreSQL treats as "no timeout".
func TestTxRejectsNonPositiveTimeouts(t *testing.T) {
	for _, c := range []struct {
		name           string
		lockMS, stmtMS int64
	}{
		{"zero lock", 0, 1000}, {"zero statement", 1000, 0}, {"negative lock", -1, 1000}, {"negative statement", 1000, -1},
	} {
		t.Run(c.name, func(t *testing.T) {
			called := false
			err := runner(c.lockMS, c.stmtMS).InTx(context.Background(), func(context.Context, pgx.Tx) error {
				called = true
				return nil
			})
			if err == nil {
				t.Fatal("InTx = nil, want a configuration error")
			}
			if errors.Is(err, store.ErrBusy) || errors.Is(err, store.ErrUnknownOutcome) {
				t.Fatalf("err = %v, want a plain error", err)
			}
			if called {
				t.Fatal("fn ran with a disabled timeout")
			}
		})
	}
}

// Invariant guards: CHECK, UNIQUE and the append-only raise are ErrInvariant.
func TestTxInvariantViolations(t *testing.T) {
	cases := []struct {
		name string
		step txFn
	}{
		{"check 23514", exec(`UPDATE campaigns SET spent = budget + 1`)},
		{"unique 23505", func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `CREATE TEMP TABLE u (k int UNIQUE) ON COMMIT DROP`); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO u VALUES (1), (1)`)
			return err
		}},
		{"append-only P0001", func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO payments (campaign_id, user_id, idempotency_key, request_hash,
				amount, status, cashback_awarded, cashback_reason, rate_bps, min_payment, daily_cap,
				campaign_day, created_at)
				SELECT id, 'tx_marker', gen_random_uuid(), decode(repeat('00', 32), 'hex'), 100, 'SUCCEEDED', 0,
				'BELOW_MINIMUM', rate_bps, min_payment, daily_cap, current_date, now() FROM campaigns`); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE payments SET amount = 101`)
			return err
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reset(t, 10_000_000)
			err := runner(1000, 10_000).InTx(context.Background(), markThen(c.step))
			wantNothingCommitted(t, err, store.ErrInvariant)
			var n int
			if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM payments`).Scan(&n); err != nil || n != 0 {
				t.Fatalf("payments rows = %d (%v), want 0", n, err)
			}
		})
	}
}

// An error at COMMIT leaves the outcome unknown, even when its SQLSTATE
// (23505, deferred) would be an invariant before COMMIT.
func TestTxCommitErrorIsUnknownOutcome(t *testing.T) {
	reset(t, 10_000_000)
	err := runner(1000, 10_000).InTx(context.Background(), markThen(func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `CREATE TEMP TABLE d (k int, UNIQUE (k) DEFERRABLE INITIALLY DEFERRED) ON COMMIT DROP`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO d VALUES (1), (1)`)
		return err
	}))
	wantNothingCommitted(t, err, store.ErrUnknownOutcome)
	if errors.Is(err, store.ErrInvariant) || errors.Is(err, store.ErrBusy) {
		t.Fatalf("COMMIT error must be unknown only, got %v", err)
	}
}

// A cancelled parent context does not cancel a transaction in flight.
func TestTxParentCancelDoesNotCancelTx(t *testing.T) {
	reset(t, 10_000_000)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := runner(1000, 10_000).InTx(parent, func(ctx context.Context, tx pgx.Tx) error {
		time.AfterFunc(50*time.Millisecond, cancel)
		if _, err := tx.Exec(ctx, `SELECT pg_sleep(0.2)`); err != nil {
			return err
		}
		return mark(ctx, tx)
	})
	if err != nil {
		t.Fatalf("InTx = %v, want nil", err)
	}
	if parent.Err() == nil {
		t.Fatal("parent was not cancelled")
	}
	if n := markers(t); n != 1 {
		t.Fatalf("markers = %d, want 1", n)
	}
}

// AC-55: PostgreSQL unreachable is an unknown outcome (500), not busy.
func TestTxPostgresDownIsUnknownOutcome(t *testing.T) {
	down, err := pgxpool.New(context.Background(), "postgres://flash:flash@127.0.0.1:1/flash?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer down.Close()
	called := false
	err = store.TxRunner{Pool: down, LockTimeoutMS: 100, StatementTimeoutMS: 100}.InTx(context.Background(),
		func(context.Context, pgx.Tx) error { called = true; return nil })
	if !errors.Is(err, store.ErrUnknownOutcome) || errors.Is(err, store.ErrBusy) {
		t.Fatalf("err = %v, want unknown outcome only", err)
	}
	if called {
		t.Fatal("fn ran without a transaction")
	}
}

// A committed transaction returns nil: the deferred rollback finds the
// transaction closed and says nothing.
func TestTxCommitSucceeds(t *testing.T) {
	reset(t, 10_000_000)
	if err := runner(1000, 10_000).InTx(context.Background(), mark); err != nil {
		t.Fatalf("InTx = %v, want nil", err)
	}
	if n := markers(t); n != 1 {
		t.Fatalf("markers = %d, want 1", n)
	}
}

// A domain error from fn passes through unchanged and rolls back.
func TestTxDomainErrorPassesThrough(t *testing.T) {
	reset(t, 10_000_000)
	domainErr := errors.New("domain")
	err := runner(1000, 10_000).InTx(context.Background(), markThen(func(context.Context, pgx.Tx) error { return domainErr }))
	if err != domainErr {
		t.Fatalf("err = %v, want the domain error itself", err)
	}
	if n := markers(t); n != 0 {
		t.Fatalf("markers = %d, want 0", n)
	}
}
