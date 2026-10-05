//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/riyanpratamap/flash-cashback/backend/internal/cache"
	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/service"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// Tech-spec §6: a write deletes the keys of the views it changes, after
// COMMIT. These tests plant a value under each key, run the write, and look
// at which keys remain. No read uses the cache yet (P4.4).

func plantKeys(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if err := rdb.Set(context.Background(), k, "stale", 0).Err(); err != nil {
			t.Fatalf("plant %s: %v", k, err)
		}
	}
}

func keyExists(t *testing.T, k string) bool {
	t.Helper()
	n, err := rdb.Exists(context.Background(), k).Result()
	if err != nil {
		t.Fatalf("exists %s: %v", k, err)
	}
	return n == 1
}

// wantKeys fails unless exactly the named keys of keys exist.
func wantKeys(t *testing.T, label string, present map[string]bool) {
	t.Helper()
	for k, want := range present {
		if got := keyExists(t, k); got != want {
			t.Errorf("%s: key %s exists = %v, want %v", label, k, got, want)
		}
	}
}

var (
	cashbackA = cache.CashbackKey("user_a")
	cashbackB = cache.CashbackKey("user_b")
)

func payHeaders(user, key string) map[string]string {
	return map[string]string{"X-User-ID": user, "Idempotency-Key": key}
}

func TestPayDeletesOnlyThatUsersCashbackKey(t *testing.T) {
	reset(t, 10_000_000)
	plantKeys(t, cashbackA, cashbackB, cache.CampaignKey)
	if r := pay(t, "user_a", 100000); r.status != http.StatusCreated {
		t.Fatal(r.raw)
	}
	wantKeys(t, "after a payment", map[string]bool{cashbackA: false, cashbackB: true, cache.CampaignKey: true})
	assertReconciled(t)
}

func TestPayThatTakesTheLastOfTheBudgetDeletesCampaignKey(t *testing.T) {
	reset(t, 5000)
	plantKeys(t, cashbackA, cache.CampaignKey)
	r := pay(t, "user_a", 100000)
	if r.status != http.StatusCreated || r.res.Cashback.Awarded != 5000 {
		t.Fatal(r.raw)
	}
	wantKeys(t, "budget exhausted", map[string]bool{cashbackA: false, cache.CampaignKey: false})
	assertReconciled(t)
}

func TestPayWithNoAwardStillDeletesCashbackKeyOnly(t *testing.T) {
	reset(t, 10_000_000)
	plantKeys(t, cashbackA, cache.CampaignKey)
	r := pay(t, "user_a", 19999)
	if r.status != http.StatusCreated || r.res.Cashback.Reason != domain.ReasonBelowMinimum {
		t.Fatal(r.raw)
	}
	wantKeys(t, "below minimum", map[string]bool{cashbackA: false, cache.CampaignKey: true})
}

func TestPayReplayAndConflictDeleteNothing(t *testing.T) {
	reset(t, 10_000_000)
	h := payRouter()
	key := uuid.NewString()
	first, err := doPayment(h, payHeaders("user_a", key), `{"amount":100000}`)
	if err != nil || first.status != http.StatusCreated {
		t.Fatalf("first = %+v, %v", first, err)
	}
	plantKeys(t, cashbackA, cache.CampaignKey)
	replay, err := doPayment(h, payHeaders("user_a", key), `{"amount":100000}`)
	if err != nil || replay.status != http.StatusOK || replay.replayed != "true" {
		t.Fatalf("replay = %+v, %v", replay, err)
	}
	wantKeys(t, "replay", map[string]bool{cashbackA: true, cache.CampaignKey: true})
	conflict, err := doPayment(h, payHeaders("user_a", key), `{"amount":200000}`)
	if err != nil || conflict.status != http.StatusConflict {
		t.Fatalf("conflict = %+v, %v", conflict, err)
	}
	wantKeys(t, "409", map[string]bool{cashbackA: true, cache.CampaignKey: true})
}

// The client hangs up while the transaction runs: the request context is
// cancelled, the commit still happens (InTx detaches), and the delete must
// still run (KP). The hook cancels after the step-5 read.
func TestPayDeleteSurvivesCancelledRequestContext(t *testing.T) {
	reset(t, 10_000_000)
	plantKeys(t, cashbackA)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := service.NewPayments(store.TxRunner{Pool: pool, LockTimeoutMS: 2000, StatementTimeoutMS: 5000,
		Hooks: store.Hooks{AfterCampaignRead: cancel}}, testInvalidator(), nil)
	_, replayed, err := svc.Pay(ctx, domain.MoneyCommand{
		UserID: "user_a", Key: uuid.New(), Amount: 100000, Hash: domain.RequestHash(100000), RequestID: "r1"})
	if err != nil || replayed {
		t.Fatalf("Pay = replayed %v, err %v", replayed, err)
	}
	if n := queryInt(t, `SELECT count(*) FROM payments`); n != 1 {
		t.Fatalf("payments = %d, want 1 committed", n)
	}
	if ctx.Err() == nil {
		t.Fatal("the hook did not cancel the request context")
	}
	wantKeys(t, "cancelled mid-transaction", map[string]bool{cashbackA: false})
}

func TestRedeemDeletesCashbackKeyOnlyWhenItCommits(t *testing.T) {
	reset(t, 10_000_000)
	fund(t, "user_a", 5000)
	h := payRouter()

	plantKeys(t, cashbackA, cashbackB, cache.CampaignKey)
	if r := redeem(t, "user_a", 6000); r.status != http.StatusUnprocessableEntity {
		t.Fatalf("over balance = %d: %s", r.status, r.raw)
	}
	wantKeys(t, "422", map[string]bool{cashbackA: true})

	key := uuid.NewString()
	first, err := doRedeem(h, payHeaders("user_a", key), `{"amount":2000}`)
	if err != nil || first.status != http.StatusCreated {
		t.Fatalf("redeem = %+v, %v", first, err)
	}
	wantKeys(t, "201", map[string]bool{cashbackA: false, cashbackB: true, cache.CampaignKey: true})

	plantKeys(t, cashbackA)
	replay, err := doRedeem(h, payHeaders("user_a", key), `{"amount":2000}`)
	if err != nil || replay.status != http.StatusOK || replay.replayed != "true" {
		t.Fatalf("replay = %+v, %v", replay, err)
	}
	wantKeys(t, "replay", map[string]bool{cashbackA: true})
	conflict, err := doRedeem(h, payHeaders("user_a", key), `{"amount":1000}`)
	if err != nil || conflict.status != http.StatusConflict {
		t.Fatalf("conflict = %+v, %v", conflict, err)
	}
	wantKeys(t, "key reuse 409", map[string]bool{cashbackA: true})

	setCampaignSQL(t, `UPDATE campaigns SET redemptions_paused = true`)
	plantKeys(t, cashbackA)
	if r := redeem(t, "user_a", 1000); r.status != http.StatusConflict {
		t.Fatalf("paused = %d: %s", r.status, r.raw)
	}
	wantKeys(t, "paused 409", map[string]bool{cashbackA: true})
	assertReconciled(t)
}

func TestEverySwitchCommandDeletesCampaignKey(t *testing.T) {
	reset(t, 10_000_000)
	for _, cmd := range []string{"pause-awards", "pause-awards", "resume-awards", "pause-redemptions", "resume-redemptions"} {
		plantKeys(t, cache.CampaignKey, cashbackA)
		mustAdmin(t, cmd, "--by", "owner")
		wantKeys(t, cmd, map[string]bool{cache.CampaignKey: false, cashbackA: true})
	}
}

// Redis down: the commit stands and the answer is unchanged (§4.4).
func TestWritesSucceedWithRedisAtAClosedPort(t *testing.T) {
	reset(t, 10_000_000)
	h := payRouterInv(store.TxRunner{Pool: pool, LockTimeoutMS: 2000, StatementTimeoutMS: 5000}, deadInvalidator(t), io.Discard)
	if r, err := doPayment(h, payHeaders("user_a", uuid.NewString()), `{"amount":100000}`); err != nil || r.status != http.StatusCreated {
		t.Fatalf("pay = %+v, %v", r, err)
	}
	if r, err := doRedeem(h, payHeaders("user_a", uuid.NewString()), `{"amount":1000}`); err != nil || r.status != http.StatusCreated {
		t.Fatalf("redeem = %+v, %v", r, err)
	}
	opts := adminOptions(0)
	opts.Cache = deadInvalidator(t)
	if code, _, errOut := runAdminWith(t, opts, "pause-awards", "--by", "owner"); code != 0 {
		t.Fatalf("pause-awards = exit %d, stderr %q", code, errOut)
	}
	assertReconciled(t)
}

// A failed delete is a warning with the keys, never the Redis address.
func TestFailedDeleteWarnsWithoutTheAddress(t *testing.T) {
	var logs bytes.Buffer
	deadInvalidatorLog(t, &logs).Delete(context.Background(), cashbackA)
	out := logs.String()
	if !strings.Contains(out, `"level":"WARN"`) || !strings.Contains(out, "cache delete failed") || !strings.Contains(out, cashbackA) {
		t.Errorf("log = %q, want a warning naming the key", out)
	}
	if strings.Contains(out, "127.0.0.1") || strings.Contains(out, "refused") {
		t.Errorf("log leaks the Redis address: %q", out)
	}
}

// failCommitOn makes every insert into table fail at COMMIT: a deferred
// constraint trigger raises when the transaction commits. The trigger is
// dropped when the test ends.
func failCommitOn(t *testing.T, table string) {
	t.Helper()
	execSQL(t, `CREATE FUNCTION fc_test_fail_commit() RETURNS trigger LANGUAGE plpgsql
		AS $$ BEGIN RAISE EXCEPTION 'test: commit refused'; END $$`)
	t.Cleanup(func() { execSQL(t, `DROP FUNCTION fc_test_fail_commit() CASCADE`) })
	execSQL(t, `CREATE CONSTRAINT TRIGGER fc_test_fail_commit AFTER INSERT ON `+table+`
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fc_test_fail_commit()`)
}

// Tech-spec §6: when COMMIT fails the outcome is unknown, so the cashback key
// is deleted (safe either way). The campaign key is not: Exhausted is unknown.
func TestPayDeletesCashbackKeyWhenCommitOutcomeUnknown(t *testing.T) {
	reset(t, 10_000_000)
	plantKeys(t, cashbackA, cashbackB, cache.CampaignKey)
	failCommitOn(t, "payments")
	svc := service.NewPayments(store.TxRunner{Pool: pool, LockTimeoutMS: 2000, StatementTimeoutMS: 5000}, testInvalidator(), nil)
	_, _, err := svc.Pay(context.Background(), domain.MoneyCommand{
		UserID: "user_a", Key: uuid.New(), Amount: 100000, Hash: domain.RequestHash(100000), RequestID: "r1"})
	if !errors.Is(err, store.ErrUnknownOutcome) {
		t.Fatalf("Pay err = %v, want ErrUnknownOutcome", err)
	}
	wantKeys(t, "unknown outcome", map[string]bool{cashbackA: false, cashbackB: true, cache.CampaignKey: true})
}

func TestRedeemDeletesCashbackKeyWhenCommitOutcomeUnknown(t *testing.T) {
	reset(t, 10_000_000)
	fund(t, "user_a", 5000)
	plantKeys(t, cashbackA, cashbackB, cache.CampaignKey)
	failCommitOn(t, "redemptions")
	svc := service.NewRedemptions(store.TxRunner{Pool: pool, LockTimeoutMS: 2000, StatementTimeoutMS: 5000}, testInvalidator(), nil)
	_, _, err := svc.Redeem(context.Background(), domain.MoneyCommand{
		UserID: "user_a", Key: uuid.New(), Amount: 2000, Hash: domain.RequestHash(2000), RequestID: "r1"})
	if !errors.Is(err, store.ErrUnknownOutcome) {
		t.Fatalf("Redeem err = %v, want ErrUnknownOutcome", err)
	}
	wantKeys(t, "unknown outcome", map[string]bool{cashbackA: false, cashbackB: true, cache.CampaignKey: true})
}
