//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/riyanpratamap/flash-cashback/backend/internal/boot"
	"github.com/riyanpratamap/flash-cashback/backend/migrations"
)

func TestBootFreshDB(t *testing.T) {
	ctx := context.Background()
	p, err := boot.Connect(ctx, freshDB(t), 5, 5*time.Second)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer p.Close()
	if err := boot.Run(ctx, p, 123456); err != nil {
		t.Fatalf("run: %v", err)
	}
	var n int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM campaigns`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("campaign rows = %d, err %v; want 1", n, err)
	}
	var (
		id, name                        string
		rate                            int
		minPay, cap, budget, spent      int64
		awardsPaused, redemptionsPaused bool
	)
	err = p.QueryRow(ctx, `SELECT id, name, rate_bps, min_payment, daily_cap, budget, spent,
		awards_paused, redemptions_paused FROM campaigns`).
		Scan(&id, &name, &rate, &minPay, &cap, &budget, &spent, &awardsPaused, &redemptionsPaused)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if id != "flash-cashback" || name != "Flash Cashback" || rate != 500 || minPay != 20000 ||
		cap != 50000 || budget != 123456 || spent != 0 || awardsPaused || redemptionsPaused {
		t.Fatalf("campaign = %s %s %d %d %d %d %d %v %v", id, name, rate, minPay, cap, budget, spent,
			awardsPaused, redemptionsPaused)
	}

	// TC10: a re-boot never resets the budget or the spent total.
	if _, err := p.Exec(ctx, `UPDATE campaigns SET spent = 100`); err != nil {
		t.Fatalf("set spent: %v", err)
	}
	if err := boot.Run(ctx, p, 1); err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if err := p.QueryRow(ctx, `SELECT budget, spent FROM campaigns`).Scan(&budget, &spent); err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if budget != 123456 || spent != 100 {
		t.Fatalf("after re-boot budget = %d spent = %d; want 123456 and 100", budget, spent)
	}
}

func TestConnectGivesUpAfterWait(t *testing.T) {
	start := time.Now()
	_, err := boot.Connect(context.Background(), "postgres://flash:secretpw@127.0.0.1:1/flash?sslmode=disable", 2, time.Second)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("connect to a closed port succeeded")
	}
	if elapsed < 900*time.Millisecond || elapsed > 1500*time.Millisecond {
		t.Fatalf("gave up after %v; want about 1s", elapsed)
	}
	if strings.Contains(err.Error(), "secretpw") {
		t.Fatalf("error leaks the password: %v", err)
	}
	if !strings.Contains(err.Error(), "last error: connection refused") {
		t.Fatalf("error carries no cause: %v", err)
	}
}

var schemaConstraints = []string{
	"campaigns_rate_range", "campaigns_min_positive", "campaigns_cap_positive", "campaigns_budget_positive",
	"campaigns_spent_nonneg", "campaigns_spent_within_budget", "campaigns_min_earns",
	"ude_earned_nonneg", "ude_cap_positive", "ude_earned_within_cap",
	"payments_user_format", "payments_hash_len", "payments_amount_range", "payments_status",
	"payments_award_nonneg", "payments_reason_known", "payments_user_key_unique",
	"payments_award_within_rate", "payments_award_within_cap", "payments_reason_matches_award",
	"redemptions_user_format", "redemptions_hash_len", "redemptions_amount_range", "redemptions_status",
	"redemptions_destination", "redemptions_balance_after_nonneg", "redemptions_user_key_unique",
	"balances_user_format", "balances_nonneg",
	"ledger_payment_once", "ledger_redemption_once", "ledger_balance_after_nonneg", "ledger_kind_shape",
}

func TestSchemaConstraintNames(t *testing.T) {
	ctx := context.Background()
	for _, name := range schemaConstraints {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conname = $1`, name).Scan(&n); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if n != 1 {
			t.Errorf("constraint %s: found %d, want 1", name, n)
		}
	}
	for _, name := range []string{"ledger_append_only", "payments_append_only", "redemptions_append_only"} {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_trigger WHERE tgname = $1 AND NOT tgisinternal`, name).Scan(&n); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if n != 1 {
			t.Errorf("trigger %s: found %d, want 1", name, n)
		}
	}
}

func TestAppendOnly(t *testing.T) {
	reset(t, 10_000_000)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // the test never commits; rollback keeps the shared DB clean

	hash := make([]byte, 32)
	var paymentID int64
	err = tx.QueryRow(ctx, `INSERT INTO payments (campaign_id, user_id, idempotency_key, request_hash, amount,
		status, cashback_awarded, cashback_reason, rate_bps, min_payment, daily_cap, campaign_day, created_at)
		VALUES ('flash-cashback', 'u1', gen_random_uuid(), $1, 20000, 'SUCCEEDED', 1000, 'AWARDED', 500, 20000,
		50000, fc_campaign_day(fc_now()), fc_now()) RETURNING id`, hash).Scan(&paymentID)
	if err != nil {
		t.Fatalf("insert payment: %v", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO redemptions (campaign_id, user_id, idempotency_key, request_hash, amount,
		status, destination, balance_after, campaign_day, created_at)
		VALUES ('flash-cashback', 'u1', gen_random_uuid(), $1, 500, 'COMPLETED', 'MAIN_ACCOUNT', 500,
		fc_campaign_day(fc_now()), fc_now())`, hash)
	if err != nil {
		t.Fatalf("insert redemption: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ledger_entries (user_id, kind, amount, payment_id, balance_after, created_at)
		VALUES ('u1', 'AWARD', 1000, $1, 1000, fc_now())`, paymentID); err != nil {
		t.Fatalf("insert ledger: %v", err)
	}

	for _, table := range []string{"ledger_entries", "payments", "redemptions"} {
		for _, stmt := range []string{"UPDATE " + table + " SET user_id = user_id", "DELETE FROM " + table} {
			if _, err := tx.Exec(ctx, "SAVEPOINT attempt"); err != nil {
				t.Fatal(err)
			}
			_, err := tx.Exec(ctx, stmt)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "P0001" {
				t.Errorf("%s: err = %v, want SQLSTATE P0001", stmt, err)
			}
			if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT attempt"); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestSetNow(t *testing.T) {
	wib := time.FixedZone("WIB", 7*3600)
	tests := []struct {
		name     string
		at       time.Time
		wantDay  string
		wantNext string
	}{
		{"last second of the day", time.Date(2026, 3, 10, 23, 59, 59, 0, wib), "2026-03-10", "2026-03-11 00:00:00 +0700"},
		{"first second of the next day", time.Date(2026, 3, 11, 0, 0, 0, 0, wib), "2026-03-11", "2026-03-12 00:00:00 +0700"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setNow(t, tc.at)
			var day time.Time
			var resets time.Time
			err := pool.QueryRow(context.Background(),
				`SELECT fc_campaign_day(fc_now()), fc_day_resets_at(fc_campaign_day(fc_now()))`).Scan(&day, &resets)
			if err != nil {
				t.Fatal(err)
			}
			if got := day.Format("2006-01-02"); got != tc.wantDay {
				t.Errorf("campaign day = %s, want %s", got, tc.wantDay)
			}
			if got := resets.In(wib).Format("2006-01-02 15:04:05 -0700"); got != tc.wantNext {
				t.Errorf("resets at = %s, want %s", got, tc.wantNext)
			}
		})
	}
	// The subtests' cleanups have run: fc_now() is the real clock again.
	var driftSeconds float64
	err := pool.QueryRow(context.Background(), `SELECT abs(extract(epoch FROM fc_now() - now()))`).Scan(&driftSeconds)
	if err != nil {
		t.Fatal(err)
	}
	if driftSeconds > 5 {
		t.Errorf("fc_now() is %.0fs from now() after cleanup; the clock was not restored", driftSeconds)
	}
}

// prepareVersionTable makes goose create its version table (and nothing else).
func prepareVersionTable(t *testing.T, p *pgxpool.Pool) {
	t.Helper()
	db := stdlib.OpenDBFromPool(p)
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatalf("goose provider: %v", err)
	}
	if _, err := provider.HasPending(context.Background()); err != nil {
		t.Fatalf("goose has pending: %v", err)
	}
	var n int
	if err := p.QueryRow(context.Background(), `SELECT count(*) FROM goose_db_version`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("version table rows = %d, err %v; want only the zero row", n, err)
	}
}

// raceBoot boots two pools on one fresh database at once and asserts the
// outcome: no error, one applied version-1 row, one campaign row.
func raceBoot(t *testing.T, prepare bool) {
	url := freshDB(t)
	ctx := context.Background()
	pools := make([]*pgxpool.Pool, 2)
	for i := range pools {
		p, err := boot.Connect(ctx, url, 5, 5*time.Second)
		if err != nil {
			t.Fatalf("connect %d: %v", i, err)
		}
		defer p.Close()
		pools[i] = p
	}
	if prepare {
		prepareVersionTable(t, pools[0])
	}

	start := make(chan struct{})
	errs := make([]error, len(pools))
	var wg sync.WaitGroup
	for i, p := range pools {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = boot.Run(ctx, p, 1_000_000)
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("boot %d: %v", i, err)
		}
	}
	var applied, campaigns int
	err := pools[0].QueryRow(ctx,
		`SELECT count(*) FROM goose_db_version WHERE version_id = 1 AND is_applied`).Scan(&applied)
	if err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if applied != 1 {
		t.Errorf("applied rows for version 1 = %d, want 1", applied)
	}
	if err := pools[0].QueryRow(ctx, `SELECT count(*) FROM campaigns`).Scan(&campaigns); err != nil {
		t.Fatalf("count campaigns: %v", err)
	}
	if campaigns != 1 {
		t.Errorf("campaign rows = %d, want 1", campaigns)
	}
}

// TestRaceBootTwice is the mutation proof of the session lock. goose itself
// retries a concurrent create of its version table after 1 s, which hides a
// missing lock on a fresh database, so the version table is created first and
// the race is on the migration itself.
func TestRaceBootTwice(t *testing.T) { raceBoot(t, true) }

// TestRaceBootTwiceEmpty is AC-53's literal case: two boots on a truly empty
// database. It is outcome-only: goose's retry of the version-table create
// masks the no-locker mutation here, so TestRaceBootTwice is the proof.
func TestRaceBootTwiceEmpty(t *testing.T) { raceBoot(t, false) }
