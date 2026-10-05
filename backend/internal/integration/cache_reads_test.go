//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/riyanpratamap/flash-cashback/backend/internal/cache"
	"github.com/riyanpratamap/flash-cashback/backend/internal/config"
	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/httpapi"
	"github.com/riyanpratamap/flash-cashback/backend/internal/service"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// Tech-spec §6, the read side (D53): GET /campaign through a ReadCache on the
// test Redis; GET /me/cashback is never cached. The writes in these tests go
// through the real services, whose invalidator deletes the campaign key. The
// routers in the other files pass no cache, so they keep testing PostgreSQL's
// answers.

var quietLog = slog.New(slog.NewJSONHandler(io.Discard, nil))

func readCfg(on bool, addr string, timeout time.Duration) config.Config {
	return config.Config{
		RedisAddr: addr, RedisTimeout: timeout, CacheReads: on,
		CacheCampaignTTL: 5 * time.Second,
	}
}

// testReadCache reads and writes the test Redis.
func testReadCache() *cache.ReadCache {
	return cache.NewReadCache(rdb, readCfg(true, "", testRedisTimeout), slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

// cachedRouter serves the reads through rc and the writes through the real
// services with inv.
func cachedRouter(rc *cache.ReadCache, inv *cache.Invalidator) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	tx := store.TxRunner{Pool: pool, LockTimeoutMS: 2000, StatementTimeoutMS: 5000}
	reads := service.NewReads(pool, rc)
	return httpapi.NewRouter(httpapi.Deps{
		PingPostgres: func(context.Context) error { return nil },
		PingRedis:    func(context.Context) error { return nil },
		Log:          log,
		Reads:        reads,
		History:      reads,
		Payments:     service.NewPayments(tx, inv, log),
		Redemptions:  service.NewRedemptions(tx, log),
	})
}

// serveGet sends one GET through h and returns the status and body.
func serveGet(h http.Handler, path, user string) (int, string) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-User-ID", user)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// mustGet is serveGet for a 200.
func mustGet(t *testing.T, h http.Handler, path, user string) string {
	t.Helper()
	code, body := serveGet(h, path, user)
	if code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", path, code, body)
	}
	return body
}

// cashbackOf decodes a GET /me/cashback body.
func cashbackOf(t *testing.T, body string) domain.CashbackView {
	t.Helper()
	var v domain.CashbackView
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("cashback body %q: %v", body, err)
	}
	return v
}

func campaignOf(t *testing.T, body string) domain.CampaignView {
	t.Helper()
	var v domain.CampaignView
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("campaign body %q: %v", body, err)
	}
	return v
}

const (
	cashbackPath = "/v1/me/cashback"
	campaignPath = "/v1/campaign"
)

const plantedCampaign = `{"id":"flash-cashback","name":"From the cache","status":"ACTIVE","redemption_status":"AVAILABLE",` +
	`"rules":{"rate_bps":500,"min_payment":20000,"daily_cap":50000,"timezone":"Asia/Jakarta"}}`

func plantedCashback(balance int64) string {
	return `{"balance":` + itoa(balance) + `,"today":{"date":"2026-01-02","earned":0,"remaining":50000,` +
		`"resets_at":"2026-01-02T17:00:00Z"}}`
}

func setKey(t *testing.T, key, value string) {
	t.Helper()
	if err := rdb.Set(context.Background(), key, value, time.Minute).Err(); err != nil {
		t.Fatalf("set %s: %v", key, err)
	}
}

// A planted valid campaign body is served: the read goes to the cache first.
func TestReadsServeAValidCachedCampaignBody(t *testing.T) {
	reset(t, 10_000_000)
	h := cachedRouter(testReadCache(), testInvalidator())
	setKey(t, cache.CampaignKey, plantedCampaign)
	if got := campaignOf(t, mustGet(t, h, campaignPath, "user_a")); got.Name != "From the cache" {
		t.Errorf("campaign = %+v, want the cached body", got)
	}
}

// A campaign miss fills the key with the TTL of §6; a cashback read writes
// no key (D53).
func TestCampaignMissFillsTheKeyWithItsTTL(t *testing.T) {
	reset(t, 10_000_000)
	h := cachedRouter(testReadCache(), testInvalidator())
	mustGet(t, h, campaignPath, "user_a")
	mustGet(t, h, cashbackPath, "user_a")
	if ttl := rdb.PTTL(context.Background(), cache.CampaignKey).Val(); ttl <= 0 || ttl > 5*time.Second {
		t.Errorf("campaign TTL = %v, want in (0, 5s]", ttl)
	}
	if keys := redisKeys(t); len(keys) != 1 || keys[0] != cache.CampaignKey {
		t.Errorf("keys = %v, want only %s", keys, cache.CampaignKey)
	}
}

// Errors are never cached: with the campaign row not found, both reads fail
// in the service after the cache missed, and no key is left behind.
func TestReadErrorsAreNeverCached(t *testing.T) {
	reset(t, 10_000_000)
	h := cachedRouter(testReadCache(), testInvalidator())
	setCampaignSQL(t, `UPDATE campaigns SET id = 'gone'`)
	for _, path := range []string{campaignPath, cashbackPath} {
		if code, body := serveGet(h, path, "user_a"); code == http.StatusOK {
			t.Errorf("GET %s = 200 with no campaign row: %s", path, body)
		}
	}
	if keys := redisKeys(t); len(keys) != 0 {
		t.Errorf("keys = %v, want none after failed reads", keys)
	}
}

// AC-42: a warm campaign cache never hides a committed change, and
// GET /me/cashback reads PostgreSQL every time: no cashback key is written.
func TestWarmCacheFollowsWritesAC42(t *testing.T) {
	reset(t, 10_000_000)
	h := cachedRouter(testReadCache(), testInvalidator())
	if got := cashbackOf(t, mustGet(t, h, cashbackPath, "user_a")); got.Balance != 0 {
		t.Fatalf("first read = %+v", got)
	}
	if r := pay(t, "user_a", 100000); r.status != http.StatusCreated {
		t.Fatal(r.raw)
	}
	if got := cashbackOf(t, mustGet(t, h, cashbackPath, "user_a")); got.Balance != 5000 {
		t.Fatalf("after the payment balance = %d, want 5000", got.Balance)
	}
	if r := redeem(t, "user_a", 1000); r.status != http.StatusCreated {
		t.Fatal(r.raw)
	}
	if got := cashbackOf(t, mustGet(t, h, cashbackPath, "user_a")); got.Balance != 4000 {
		t.Fatalf("after the redemption balance = %d, want 4000", got.Balance)
	}
	if keys := redisKeys(t); len(keys) != 0 {
		t.Fatalf("keys after cashback reads = %v, want none", keys)
	}
	if got := campaignOf(t, mustGet(t, h, campaignPath, "user_a")); got.Status != domain.CampaignActive {
		t.Fatalf("campaign = %+v", got)
	}
	if !keyExists(t, cache.CampaignKey) {
		t.Fatal("campaign key not warm")
	}
	mustAdmin(t, "pause-awards", "--by", "owner")
	if got := campaignOf(t, mustGet(t, h, campaignPath, "user_a")); got.Status != domain.CampaignPaused {
		t.Fatalf("after pause-awards status = %s, want PAUSED", got.Status)
	}
	mustAdmin(t, "resume-awards", "--by", "owner")
	assertReconciled(t)
}

// AC-42, the last of the budget: the payment deletes the warm campaign key
// and the next read shows ENDED.
func TestWarmCampaignShowsEndedAfterTheLastOfTheBudgetAC42(t *testing.T) {
	reset(t, 5000)
	h := cachedRouter(testReadCache(), testInvalidator())
	if got := campaignOf(t, mustGet(t, h, campaignPath, "user_a")); got.Status != domain.CampaignActive || !keyExists(t, cache.CampaignKey) {
		t.Fatalf("campaign = %+v, key warm %v", got, keyExists(t, cache.CampaignKey))
	}
	if r := pay(t, "user_a", 100000); r.status != http.StatusCreated || r.res.Cashback.Awarded != 5000 {
		t.Fatal(r.raw)
	}
	if got := campaignOf(t, mustGet(t, h, campaignPath, "user_a")); got.Status != domain.CampaignEnded {
		t.Fatalf("after the last of the budget status = %s, want ENDED", got.Status)
	}
	assertReconciled(t)
}

// AC-43: Redis at a closed port is a miss on every read; the answers equal
// PostgreSQL's and the writes are unchanged.
func TestReadsAndWritesWithRedisAtAClosedPortAC43(t *testing.T) {
	reset(t, 10_000_000)
	dead := cache.NewClient(readCfg(true, closedPort, testRedisTimeout))
	t.Cleanup(func() { _ = dead.Close() })
	h := cachedRouter(cache.NewReadCache(dead, readCfg(true, closedPort, testRedisTimeout), quietLog), deadInvalidator(t))
	plain := cachedRouter(nil, testInvalidator())
	for _, path := range []string{campaignPath, cashbackPath} {
		if got, want := mustGet(t, h, path, "user_a"), mustGet(t, plain, path, "user_a"); got != want {
			t.Errorf("GET %s with Redis down = %s, want %s", path, got, want)
		}
	}
	if r, err := doPayment(h, payHeaders("user_a", uuid.NewString()), `{"amount":100000}`); err != nil || r.status != http.StatusCreated {
		t.Fatalf("pay = %+v, %v", r, err)
	}
	if got := cashbackOf(t, mustGet(t, h, cashbackPath, "user_a")); got.Balance != 5000 {
		t.Errorf("balance = %d, want 5000", got.Balance)
	}
	if r, err := doRedeem(h, payHeaders("user_a", uuid.NewString()), `{"amount":1000}`); err != nil || r.status != http.StatusCreated {
		t.Fatalf("redeem = %+v, %v", r, err)
	}
	assertReconciled(t)
}

// AC-44: a Redis that stalls for 500 ms costs a read one deadline of 50 ms
// (the GET; a failed GET skips the SET), not the stall; a payment is
// unaffected.
func TestStalledRedisCostsAReadItsDeadlineAC44(t *testing.T) {
	reset(t, 10_000_000)
	cfg := readCfg(true, testRedisAddr(), 50*time.Millisecond)
	c := cache.NewClient(cfg) // the production options, deadline 50 ms
	t.Cleanup(func() { _ = c.Close() })
	h := cachedRouter(cache.NewReadCache(c, cfg, quietLog), testInvalidator())
	mustGet(t, h, campaignPath, "user_b") // connections up before the stall

	ctx := context.Background()
	if err := rdb.Do(ctx, "CLIENT", "PAUSE", "500", "ALL").Err(); err != nil {
		t.Fatalf("client pause: %v", err)
	}
	stalled := time.Now()
	t.Cleanup(func() { time.Sleep(time.Until(stalled.Add(700 * time.Millisecond))) })

	start := time.Now()
	code, body := serveGet(h, cashbackPath, "user_a")
	took := time.Since(start)
	if code != http.StatusOK || cashbackOf(t, body).Balance != 0 {
		t.Fatalf("read = %d %s", code, body)
	}
	if took > 90*time.Millisecond {
		t.Errorf("read took %v during a 500 ms Redis stall, want under 90 ms (one deadline)", took)
	}
	if r := pay(t, "user_a", 100000); r.status != http.StatusCreated {
		t.Errorf("pay during the stall = %d: %s", r.status, r.raw)
	}
}

// AC-45: a value that is not a campaign body is a miss: the answer is
// PostgreSQL's and the key is replaced by the good body.
func TestInvalidCachedCampaignIsAMissAC45(t *testing.T) {
	t.Run("not json", func(t *testing.T) {
		reset(t, 10_000_000)
		h := cachedRouter(testReadCache(), testInvalidator())
		setKey(t, cache.CampaignKey, "stale")
		if got := campaignOf(t, mustGet(t, h, campaignPath, "user_a")); got.Name == "From the cache" || got.Rules.DailyCap != 50000 {
			t.Fatalf("campaign = %+v, want PostgreSQL's answer", got)
		}
		stored, err := rdb.Get(context.Background(), cache.CampaignKey).Result()
		if err != nil || campaignOf(t, stored).Rules.DailyCap != 50000 {
			t.Errorf("key after the read = %q, %v; want the good body", stored, err)
		}
	})
	t.Run("campaign missing rules", func(t *testing.T) {
		reset(t, 10_000_000)
		h := cachedRouter(testReadCache(), testInvalidator())
		setKey(t, cache.CampaignKey, `{"id":"flash-cashback","name":"From the cache","status":"ACTIVE","redemption_status":"AVAILABLE"}`)
		if got := campaignOf(t, mustGet(t, h, campaignPath, "user_a")); got.Name == "From the cache" || got.Rules.DailyCap != 50000 {
			t.Fatalf("campaign = %+v, want PostgreSQL's answer", got)
		}
	})
	t.Run("campaign unknown status", func(t *testing.T) {
		reset(t, 10_000_000)
		h := cachedRouter(testReadCache(), testInvalidator())
		setKey(t, cache.CampaignKey, strings.Replace(plantedCampaign, `"ACTIVE"`, `"DONE"`, 1))
		if got := campaignOf(t, mustGet(t, h, campaignPath, "user_a")); got.Status != domain.CampaignActive || got.Name == "From the cache" {
			t.Fatalf("campaign = %+v, want PostgreSQL's answer", got)
		}
	})
}

// AC-45 (second half): a cashback key left by an older build (balance 999999)
// is never read: GET /me/cashback shows PostgreSQL's 18000, and a redemption
// of 20000 is 422 (INV-11).
func TestStaleCashbackKeyIsNeverReadAC45(t *testing.T) {
	reset(t, 10_000_000)
	fund(t, "user_a", 18000)
	h := cachedRouter(testReadCache(), testInvalidator())
	setKey(t, cashbackA, plantedCashback(999999))
	if got := cashbackOf(t, mustGet(t, h, cashbackPath, "user_a")); got.Balance != 18000 {
		t.Fatalf("shown balance = %d, want PostgreSQL's 18000", got.Balance)
	}
	r := redeem(t, "user_a", 20000)
	if r.status != http.StatusUnprocessableEntity {
		t.Fatalf("redeem 20000 = %d: %s", r.status, r.raw)
	}
	if got := balanceOf(t, "user_a"); got != 18000 {
		t.Errorf("balance = %d, want 18000", got)
	}
	assertReconciled(t)
}

// AC-76: with CACHE_READS=off the campaign key is neither read nor written;
// the delete after the last-of-budget commit still runs.
func TestCacheReadsOffAC76(t *testing.T) {
	reset(t, 5000)
	rc := cache.NewReadCache(rdb, readCfg(false, "", testRedisTimeout), quietLog)
	h := cachedRouter(rc, testInvalidator())
	mustGet(t, h, campaignPath, "user_a")
	mustGet(t, h, cashbackPath, "user_a")
	if keys := redisKeys(t); len(keys) != 0 {
		t.Fatalf("keys with the cache off = %v, want none", keys)
	}
	// A planted body is not served either.
	setKey(t, cache.CampaignKey, plantedCampaign)
	if got := campaignOf(t, mustGet(t, h, campaignPath, "user_a")); got.Name == "From the cache" {
		t.Fatalf("campaign = %+v, want PostgreSQL's answer", got)
	}
	if r := pay(t, "user_a", 100000); r.status != http.StatusCreated {
		t.Fatal(r.raw)
	}
	if keyExists(t, cache.CampaignKey) {
		t.Error("the last-of-budget payment did not delete the key set by hand")
	}
	assertReconciled(t)
}

// Failures log once per 10 s per operation, with the key and no address.
func TestReadWarningsAreThrottledAndHideTheAddress(t *testing.T) {
	reset(t, 10_000_000)
	var buf bytes.Buffer
	cfg := readCfg(true, closedPort, 100*time.Millisecond)
	dead := cache.NewClient(cfg)
	t.Cleanup(func() { _ = dead.Close() })
	h := cachedRouter(cache.NewReadCache(dead, cfg, slog.New(slog.NewJSONHandler(&buf, nil))), testInvalidator())
	for range 5 {
		mustGet(t, h, campaignPath, "user_a")
		mustGet(t, h, cashbackPath, "user_a")
	}
	out := buf.String()
	if n := strings.Count(out, `"op":"get"`); n != 1 {
		t.Errorf("get warnings = %d, want 1:\n%s", n, out)
	}
	if n := strings.Count(out, `"op":"set"`); n != 0 {
		t.Errorf("set warnings = %d, want 0 (a failed GET skips the SET):\n%s", n, out)
	}
	if strings.Contains(out, "127.0.0.1") || strings.Contains(out, "connection refused") {
		t.Errorf("a warning carries the address or the driver text:\n%s", out)
	}
}

// The read deadline is the context's, not the client's: with client timeouts
// of 500 ms a stalled Redis still costs a read its own 50 ms deadline. This
// is what ContextTimeoutEnabled buys (KP).
func TestReadDeadlineIsTheContextsNotTheClientsAC44(t *testing.T) {
	reset(t, 10_000_000)
	c := cache.NewClient(readCfg(true, testRedisAddr(), 500*time.Millisecond))
	t.Cleanup(func() { _ = c.Close() })
	h := cachedRouter(cache.NewReadCache(c, readCfg(true, testRedisAddr(), 50*time.Millisecond), quietLog), testInvalidator())
	mustGet(t, h, campaignPath, "user_b") // connections up before the stall

	if err := rdb.Do(context.Background(), "CLIENT", "PAUSE", "500", "ALL").Err(); err != nil {
		t.Fatalf("client pause: %v", err)
	}
	stalled := time.Now()
	t.Cleanup(func() { time.Sleep(time.Until(stalled.Add(700 * time.Millisecond))) })

	start := time.Now()
	if code, body := serveGet(h, cashbackPath, "user_a"); code != http.StatusOK || cashbackOf(t, body).Balance != 0 {
		t.Fatalf("read = %d %s", code, body)
	}
	if took := time.Since(start); took > 90*time.Millisecond {
		t.Errorf("read took %v with a 50 ms read deadline, want under 90 ms", took)
	}
}

// A read whose request context is cancelled (a client that went away) is a
// silent miss: Redis is healthy, so no warning.
func TestCancelledRequestIsASilentMissNoWarn(t *testing.T) {
	reset(t, 10_000_000)
	var buf bytes.Buffer
	rc := cache.NewReadCache(rdb, readCfg(true, "", testRedisTimeout), slog.New(slog.NewJSONHandler(&buf, nil)))
	reads := service.NewReads(pool, rc)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = reads.Cashback(ctx, "user_a")
	_, _ = reads.Campaign(ctx)
	if buf.Len() != 0 {
		t.Errorf("a cancelled request logged:\n%s", buf.String())
	}
}
