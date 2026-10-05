package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Typed outcomes of a money transaction. The driver's own error is never
// wrapped: its text may carry SQL or values (tech-spec §4.4).
var (
	// ErrBusy: a lock wait, statement or overall deadline expired, or a
	// deadlock was detected, before COMMIT. Nothing was committed.
	ErrBusy = errors.New("store: busy, nothing committed")
	// ErrDeadlock marks the ErrBusy that came from a deadlock (40P01), so the
	// edge can log it at error level. It is always joined with ErrBusy.
	ErrDeadlock = errors.New("store: deadlock detected")
	// ErrInvariant: a CHECK, UNIQUE or append-only guard fired. A bug, not load.
	ErrInvariant = errors.New("store: invariant violated")
	// ErrUnknownOutcome: COMMIT failed, or the connection was lost or never
	// made; the caller cannot know whether the transaction committed.
	ErrUnknownOutcome = errors.New("store: outcome unknown")
)

// defaultCap bounds one money transaction (tech-spec §4.4).
const defaultCap = 5 * time.Second

// SQLSTATE codes the classifier reads. Never message text.
const (
	sqlLockNotAvailable = "55P03"
	sqlQueryCanceled    = "57014"
	sqlDeadlock         = "40P01"
	sqlCheckViolation   = "23514"
	sqlUniqueViolation  = "23505"
	sqlRaiseException   = "P0001" // fc_append_only is the only raiser in the schema
)

// TxRunner runs one money write as one transaction with server-side timeouts.
type TxRunner struct {
	Pool               *pgxpool.Pool
	LockTimeoutMS      int64
	StatementTimeoutMS int64
	Cap                time.Duration // zero means 5 s; tests shorten it
}

// InTx runs fn in a READ COMMITTED transaction and commits if fn returns nil.
// The parent context never cancels the transaction: once a request has begun
// a money write, the client hanging up must not leave it half-decided. The
// transaction is bounded by Cap instead. Both timeouts must be positive: a
// zero would be '0ms', which PostgreSQL reads as "no timeout".
func (r TxRunner) InTx(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) (err error) {
	if r.LockTimeoutMS <= 0 || r.StatementTimeoutMS <= 0 {
		return errors.New("store: lock and statement timeouts must be positive")
	}
	limit := r.Cap
	if limit == 0 {
		limit = defaultCap
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), limit)
	defer cancel()

	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: begin", ErrUnknownOutcome)
	}
	defer func() {
		// A fresh context: the transaction's own may have expired.
		rbCtx, rbCancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer rbCancel()
		if rbErr := tx.Rollback(rbCtx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) && err == nil {
			err = fmt.Errorf("%w: rollback", ErrUnknownOutcome)
		}
	}()

	// SET cannot take bind parameters; these are validated ints.
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL lock_timeout = '%dms'", r.LockTimeoutMS)); err != nil {
		return classifyBeforeCommit(tx, err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = '%dms'", r.StatementTimeoutMS)); err != nil {
		return classifyBeforeCommit(tx, err)
	}
	if err := fn(ctx, tx); err != nil {
		return classifyBeforeCommit(tx, err)
	}
	if err := tx.Commit(ctx); err != nil {
		// Whatever the cause, the server may or may not have committed.
		return fmt.Errorf("%w: commit", ErrUnknownOutcome)
	}
	return nil
}

// classifyBeforeCommit types an error that happened before COMMIT was sent,
// so the transaction is known to be rolled back. An error from fn that is not
// a database error passes through unchanged while the connection is alive; a
// closed connection becomes ErrUnknownOutcome.
func classifyBeforeCommit(tx pgx.Tx, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case sqlDeadlock:
			return fmt.Errorf("%w: %w", ErrBusy, ErrDeadlock)
		case sqlLockNotAvailable, sqlQueryCanceled:
			return ErrBusy
		case sqlCheckViolation, sqlUniqueViolation, sqlRaiseException:
			return ErrInvariant
		}
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrBusy
	}
	if tx.Conn().IsClosed() {
		return fmt.Errorf("%w: connection lost", ErrUnknownOutcome)
	}
	return err
}
