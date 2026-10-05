//go:build integration

package integration

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/riyanpratamap/flash-cashback/backend/internal/boot"
	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// AC-13: the campaign day is the WIB calendar day. At 23:59:59 WIB the cap is
// used up; one second later the day is new and the same payment earns again.
func TestPayCampaignDayAC13(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T16:59:59Z")

	first := pay(t, "user_c", 1_000_000)
	if first.status != http.StatusCreated || first.res.Cashback.Awarded != 50000 ||
		first.res.Cashback.Reason != domain.ReasonAwarded {
		t.Errorf("first = %d: %s, want 201 50000 AWARDED", first.status, first.raw)
	}
	capped := pay(t, "user_c", 100000)
	if capped.status != http.StatusCreated || capped.res.Cashback.Awarded != 0 ||
		capped.res.Cashback.Reason != domain.ReasonDailyCapReached ||
		!strings.HasPrefix(capped.res.Payment.Reference, "PAY-20261003-") {
		t.Errorf("capped = %d: %s, want 201 0 DAILY_CAP_REACHED PAY-20261003-", capped.status, capped.raw)
	}

	mustSetNow(t, "2026-10-03T17:00:00Z")
	next := pay(t, "user_c", 100000)
	if next.status != http.StatusCreated || next.res.Cashback.Awarded != 5000 ||
		next.res.Cashback.Reason != domain.ReasonAwarded ||
		!strings.HasPrefix(next.res.Payment.Reference, "PAY-20261004-") {
		t.Errorf("next day = %d: %s, want 201 5000 AWARDED PAY-20261004-", next.status, next.raw)
	}

	_, got := getBody(t, "/v1/me/cashback", "user_c")
	today, _ := got["today"].(map[string]any)
	if got["balance"] != 55000.0 || today["date"] != "2026-10-04" || today["earned"] != 5000.0 ||
		today["resets_at"] != "2026-10-05T00:00:00+07:00" {
		t.Errorf("GET /me/cashback = %v, want balance 55000, date 2026-10-04, earned 5000, resets_at 2026-10-05T00:00:00+07:00", got)
	}
	assertReconciled(t)
}

// AC-14: a payment that starts before midnight WIB and waits on the campaign
// lock past midnight belongs to the day it started on.
func TestRaceMidnightAC14(t *testing.T) {
	reset(t, 10_000_000)
	wib := time.FixedZone("WIB", 7*3600)
	midnight := time.Date(2026, 10, 4, 0, 0, 0, 0, wib)
	setClockFrom(t, time.Date(2026, 10, 3, 23, 59, 59, 400_000_000, wib))
	h := payRouter()

	release, holderPid := holdCampaignLock(t)
	var (
		reply payReply
		perr  error
	)
	done := make(chan struct{})
	go func() {
		defer close(done)
		reply, perr = doPayment(h, map[string]string{
			"X-User-ID": "user_a", "Idempotency-Key": uuid.NewString(),
		}, `{"amount":100000}`)
	}()

	waiting, pollErr := waitBlockedBy(holderPid)
	if !waiting {
		release()
		<-done
		if pollErr != nil {
			t.Fatalf("poll query failed: %v", pollErr)
		}
		t.Fatal("the payment never waited on the campaign lock")
	}
	// The shifted wall clock: fc_now() is statement start plus the offset, so
	// adding the time elapsed in the statement gives clock_timestamp() plus it.
	const shifted = `SELECT fc_now() + (clock_timestamp() - now())`
	started := time.Now()
	waitFor(10*time.Second, func() bool {
		var at time.Time
		err := pool.QueryRow(context.Background(), shifted).Scan(&at)
		return err == nil && at.After(midnight) && time.Since(started) >= time.Second
	})
	var releaseAt time.Time
	if err := pool.QueryRow(context.Background(), shifted).Scan(&releaseAt); err != nil {
		t.Errorf("read shifted clock: %v", err)
	}
	release()
	<-done

	if perr != nil {
		t.Errorf("payment: %v", perr)
	}
	if reply.status != http.StatusCreated {
		t.Errorf("status = %d: %s, want 201", reply.status, reply.raw)
	}
	if !releaseAt.After(midnight) {
		t.Errorf("released at %v, want after midnight %v: the test did not cross the day", releaseAt, midnight)
	}
	var createdAt time.Time
	var day, fnDay string
	err := pool.QueryRow(context.Background(), `SELECT COALESCE(max(created_at), 'epoch'),
		COALESCE(max(campaign_day)::text, ''), COALESCE(max(fc_campaign_day(created_at))::text, '')
		FROM payments`).Scan(&createdAt, &day, &fnDay)
	if err != nil {
		t.Fatalf("read payment: %v", err)
	}
	if !createdAt.Before(releaseAt) || !createdAt.Before(midnight) {
		t.Errorf("created_at = %v, want before release %v and before midnight %v", createdAt, releaseAt, midnight)
	}
	if day != "2026-10-03" || fnDay != day {
		t.Errorf("campaign_day = %q, fc_campaign_day(created_at) = %q, want 2026-10-03 both", day, fnDay)
	}
	if !strings.HasPrefix(reply.res.Payment.Reference, "PAY-20261003-") {
		t.Errorf("reference = %q, want PAY-20261003-", reply.res.Payment.Reference)
	}
	if n := queryInt(t, `SELECT COALESCE((SELECT earned FROM user_daily_earnings
		WHERE user_id = 'user_a' AND day = '2026-10-03'), -1)`); n != 5000 {
		t.Errorf("earned on 2026-10-03 = %d, want 5000", n)
	}

	_, got := getBody(t, "/v1/me/cashback", "user_a")
	today, _ := got["today"].(map[string]any)
	if got["balance"] != 5000.0 || today["date"] != "2026-10-04" || today["earned"] != 0.0 {
		t.Errorf("GET /me/cashback = %v, want balance 5000, date 2026-10-04, earned 0", got)
	}
	assertReconciled(t)
}

// AC-52: running the boot again with another budget keeps the stored budget,
// the spent amount, and the single campaign row.
func TestPayRestartKeepsBudgetAC52(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:32:00Z")
	if got := pay(t, "user_a", 100000); got.status != http.StatusCreated || got.res.Cashback.Awarded != 5000 {
		t.Fatalf("payment = %d: %s, want 201 5000", got.status, got.raw)
	}

	if err := boot.Run(context.Background(), pool, 1); err != nil {
		t.Errorf("boot.Run = %v, want nil", err)
	}
	if n := queryInt(t, `SELECT budget FROM campaigns`); n != 10_000_000 {
		t.Errorf("budget = %d, want 10000000", n)
	}
	if n := queryInt(t, `SELECT spent FROM campaigns`); n != 5000 {
		t.Errorf("spent = %d, want 5000", n)
	}
	if n := queryInt(t, `SELECT count(*) FROM campaigns`); n != 1 {
		t.Errorf("campaign rows = %d, want 1", n)
	}
	assertReconciled(t)
}

// AC-55: with PostgreSQL unreachable every request answers 500
// INTERNAL_ERROR, with nothing of the driver or the address in the body, and
// the real database is untouched.
func TestPostgresDownAC55(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:32:00Z")
	down, err := pgxpool.New(context.Background(),
		"postgres://flash:flash@127.0.0.1:1/flash?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer down.Close()
	var logs bytes.Buffer
	h := payRouterTx(store.TxRunner{Pool: down, LockTimeoutMS: 100, StatementTimeoutMS: 100}, &logs)
	before := readCounts(t)

	check := func(name string, rec *httptest.ResponseRecorder) {
		t.Helper()
		body := rec.Body.String()
		if rec.Code != http.StatusInternalServerError || !strings.Contains(body, "INTERNAL_ERROR") {
			t.Errorf("%s = %d: %s, want 500 INTERNAL_ERROR", name, rec.Code, body)
		}
		for _, leak := range []string{"127.0.0.1", "SELECT", "INSERT", "pgx", "connect", "refused"} {
			if strings.Contains(body, leak) {
				t.Errorf("%s body leaks %q: %s", name, leak, body)
			}
		}
	}

	post := httptest.NewRequest(http.MethodPost, "/v1/payments", strings.NewReader(`{"amount":100000}`))
	post.Header.Set("X-User-ID", "user_a")
	post.Header.Set("Idempotency-Key", uuid.NewString())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, post)
	check("POST /v1/payments", rec)

	for _, path := range []string{"/v1/me/cashback", "/v1/campaign"} {
		get := httptest.NewRequest(http.MethodGet, path, nil)
		get.Header.Set("X-User-ID", "user_a")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, get)
		check("GET "+path, rec)
	}

	// Nothing committed holds by construction: the handler only touches the
	// dead pool. The check stays as a guard against a handler that grows a
	// second connection.
	if after := readCounts(t); after != before {
		t.Errorf("counts changed: %+v -> %+v", before, after)
	}

	// The internal log must not carry SQL or a stack trace either. Whether a
	// host may appear in it is the owner's call, so it is reported only.
	lower := strings.ToLower(logs.String())
	for _, leak := range []string{"select", "insert", "from ", "goroutine "} {
		if strings.Contains(lower, leak) {
			t.Errorf("log leaks %q: %s", leak, logs.String())
		}
	}
	t.Logf("log contains 127.0.0.1: %v", strings.Contains(logs.String(), "127.0.0.1"))
	assertReconciled(t)
}
