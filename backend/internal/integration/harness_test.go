//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/riyanpratamap/flash-cashback/backend/internal/boot"
	"github.com/riyanpratamap/flash-cashback/backend/internal/cache"
	"github.com/riyanpratamap/flash-cashback/backend/internal/config"
)

const defaultTestDatabaseURL = "postgres://flash:flash@localhost:55432/flash?sslmode=disable"

var (
	pool    *pgxpool.Pool
	rdb     *redis.Client // the test Redis; reset empties it
	testURL string
)

// testRedisTimeout is the invalidator's deadline in tests: longer than the
// production 50 ms so a loaded machine does not turn a delete into a flake.
const testRedisTimeout = 500 * time.Millisecond

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	testURL = os.Getenv("FC_TEST_DATABASE_URL")
	if testURL == "" {
		testURL = defaultTestDatabaseURL
	}
	ctx := context.Background()
	var err error
	pool, err = boot.Connect(ctx, testURL, 10, 30*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "integration: connect:", err)
		return 1
	}
	defer pool.Close()
	// The production client options, with a longer timeout (testRedisTimeout).
	rdb = cache.NewClient(config.Config{RedisAddr: testRedisAddr(), RedisTimeout: testRedisTimeout})
	defer rdb.Close()
	if err := boot.Run(ctx, pool, 10_000_000); err != nil {
		fmt.Fprintln(os.Stderr, "integration: boot:", err)
		return 1
	}
	return m.Run()
}

// testInvalidator deletes keys on the test Redis.
func testInvalidator() *cache.Invalidator {
	return cache.NewInvalidator(rdb, testRedisTimeout, slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

// deadInvalidator points at a closed port: every delete fails.
func deadInvalidator(t *testing.T) *cache.Invalidator {
	t.Helper()
	return deadInvalidatorLog(t, io.Discard)
}

// deadInvalidatorLog is deadInvalidator logging to w.
func deadInvalidatorLog(t *testing.T, w io.Writer) *cache.Invalidator {
	t.Helper()
	c := redis.NewClient(&redis.Options{Addr: closedPort, MaxRetries: -1, DialTimeout: testRedisTimeout})
	t.Cleanup(func() { _ = c.Close() })
	return cache.NewInvalidator(c, testRedisTimeout, slog.New(slog.NewJSONHandler(w, nil)))
}

// setCampaignSQL changes the campaign row by SQL and deletes the cached
// campaign body, as a switch command or an exhausting award would. Tests that
// set flags or the budget this way would otherwise leave a stale body.
func setCampaignSQL(t *testing.T, sql string, args ...any) {
	t.Helper()
	execSQL(t, sql, args...)
	if err := rdb.Del(context.Background(), cache.CampaignKey).Err(); err != nil {
		t.Fatalf("delete campaign key: %v", err)
	}
}

// reset empties every table and the test Redis, restores the real clock, and
// reseeds the campaign with the given budget and spent 0.
func reset(t *testing.T, budget int64) {
	t.Helper()
	ctx := context.Background()
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("reset flush redis: %v", err)
	}
	_, err := pool.Exec(ctx, `TRUNCATE campaigns, user_daily_earnings, payments, redemptions,
		cashback_balances, ledger_entries RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("reset truncate: %v", err)
	}
	restoreClock(t)
	if err := boot.Seed(ctx, pool, budget); err != nil {
		t.Fatalf("reset seed: %v", err)
	}
}

func restoreClock(t *testing.T) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`CREATE OR REPLACE FUNCTION fc_now() RETURNS timestamptz LANGUAGE sql STABLE AS $$ SELECT now() $$`)
	if err != nil {
		t.Fatalf("restore fc_now: %v", err)
	}
}

// setNow replaces fc_now() in the test database with a fixed instant and
// restores the real clock when the test ends.
func setNow(t *testing.T, ts time.Time) {
	t.Helper()
	lit := ts.UTC().Format("2006-01-02 15:04:05.999999")
	_, err := pool.Exec(context.Background(), fmt.Sprintf(
		`CREATE OR REPLACE FUNCTION fc_now() RETURNS timestamptz LANGUAGE sql STABLE AS $$ SELECT '%s+00'::timestamptz $$`, lit))
	if err != nil {
		t.Fatalf("set fc_now: %v", err)
	}
	t.Cleanup(func() { restoreClock(t) })
}

// setClockFrom replaces fc_now() with the transaction start time shifted so
// that the clock reads at now and keeps moving. The offset is at minus the
// database wall clock, read once here. The real clock returns when the test
// ends.
func setClockFrom(t *testing.T, at time.Time) {
	t.Helper()
	ctx := context.Background()
	var dbNow time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&dbNow); err != nil {
		t.Fatalf("read db clock: %v", err)
	}
	offset := at.Sub(dbNow).Microseconds()
	_, err := pool.Exec(ctx, fmt.Sprintf(
		`CREATE OR REPLACE FUNCTION fc_now() RETURNS timestamptz LANGUAGE sql STABLE AS $$ SELECT now() + interval '%d microseconds' $$`, offset))
	if err != nil {
		t.Fatalf("set shifted fc_now: %v", err)
	}
	t.Cleanup(func() { restoreClock(t) })
}

// freshDB creates an empty database on the test server and returns its URL.
// The database is dropped when the test ends.
func freshDB(t *testing.T) string {
	t.Helper()
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("freshDB random: %v", err)
	}
	name := "fc_boot_" + hex.EncodeToString(b[:])
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop database %s: %v", name, err)
		}
	})
	u, err := url.Parse(testURL)
	if err != nil {
		t.Fatalf("parse test url: %v", err)
	}
	u.Path = "/" + strings.TrimPrefix(name, "/")
	return u.String()
}
