// Package boot connects to PostgreSQL, migrates, and seeds the campaign.
package boot

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/riyanpratamap/flash-cashback/backend/migrations"
)

const retryEvery = 500 * time.Millisecond

// Connect opens a pool and pings it, retrying every 500 ms until wait has
// elapsed. Errors never carry the URL or the password.
func Connect(ctx context.Context, url string, maxConns int32, wait time.Duration) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("boot: database url is invalid")
	}
	cfg.MaxConns = maxConns
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("boot: cannot create the connection pool")
	}

	deadline := time.Now().Add(wait)
	var lastErr error
	for {
		pingCtx, cancel := context.WithDeadline(ctx, deadline)
		err = pool.Ping(pingCtx)
		cancel()
		if err == nil {
			return pool, nil
		}
		// A ping cut short by our own deadline says nothing about the database;
		// keep the earlier, real cause.
		if lastErr == nil || !errors.Is(err, context.DeadlineExceeded) {
			lastErr = err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 || ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(min(retryEvery, remaining)):
		}
	}
	pool.Close()
	return nil, fmt.Errorf("boot: database not reachable after %s (last error: %s)", wait, redactedCause(lastErr))
}

// redactedCause names why a ping failed without echoing the driver message,
// which can carry the host or the user name.
func redactedCause(err error) string {
	var pgErr *pgconn.PgError
	var netErr net.Error
	switch {
	case errors.As(err, &pgErr):
		return "SQLSTATE " + pgErr.Code
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "connection failed"
	}
}

// Migrate applies the embedded migrations under a PostgreSQL session lock, so
// processes starting together migrate once.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }() // releases the sql.DB wrapper; the pool stays open for the caller

	locker, err := lock.NewPostgresSessionLocker(lock.WithLockTimeout(1, 60))
	if err != nil {
		return fmt.Errorf("boot: create session locker: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS,
		goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("boot: create migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("boot: apply migrations: %w", err)
	}
	return nil
}

// Seed inserts the one campaign with the given budget. A second call changes
// nothing, so a restart never resets the budget.
func Seed(ctx context.Context, pool *pgxpool.Pool, budget int64) error {
	_, err := pool.Exec(ctx, `INSERT INTO campaigns (id, name, rate_bps, min_payment, daily_cap, budget)
		VALUES ('flash-cashback', 'Flash Cashback', 500, 20000, 50000, $1)
		ON CONFLICT (id) DO NOTHING`, budget)
	if err != nil {
		return fmt.Errorf("boot: seed campaign: %w", err)
	}
	return nil
}

// Run migrates, then seeds.
func Run(ctx context.Context, pool *pgxpool.Pool, budget int64) error {
	if err := Migrate(ctx, pool); err != nil {
		return err
	}
	return Seed(ctx, pool, budget)
}
