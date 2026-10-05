package cache

import (
	"testing"
	"time"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
)

func TestThrottleAllowsOncePerWindowPerOp(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	th := newThrottle(10*time.Second, func() time.Time { return now })
	steps := []struct {
		name string
		op   string
		step time.Duration // clock advance before the call
		want bool
	}{
		{"first get", "get", 0, true},
		{"second get at once", "get", 0, false},
		{"set is its own op", "set", 0, true},
		{"get just inside the window", "get", 9 * time.Second, false},
		{"get at the window edge", "get", time.Second, true},
		{"get again at once", "get", 0, false},
		{"set after a long gap", "set", time.Minute, true},
	}
	for _, s := range steps {
		now = now.Add(s.step)
		if got := th.allow(s.op); got != s.want {
			t.Errorf("%s: allow(%q) = %v, want %v", s.name, s.op, got, s.want)
		}
	}
}

func TestDecodeCampaign(t *testing.T) {
	const good = `{"id":"flash-cashback","name":"Flash","status":"ACTIVE","redemption_status":"AVAILABLE",` +
		`"rules":{"rate_bps":500,"min_payment":20000,"daily_cap":50000,"timezone":"Asia/Jakarta"}}`
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"valid", good, true},
		{"not json", `stale`, false},
		{"empty", ``, false},
		{"wrong type", `{"id":1}`, false},
		{"unknown field", `{"id":"x","name":"n","status":"ACTIVE","redemption_status":"AVAILABLE","budget":9,` +
			`"rules":{"rate_bps":500,"min_payment":20000,"daily_cap":50000,"timezone":"Asia/Jakarta"}}`, false},
		{"trailing value", good + good, false},
		{"unknown status", `{"id":"x","name":"n","status":"DONE","redemption_status":"AVAILABLE",` +
			`"rules":{"rate_bps":500,"min_payment":20000,"daily_cap":50000,"timezone":"Asia/Jakarta"}}`, false},
		{"unknown redemption status", `{"id":"x","name":"n","status":"ACTIVE","redemption_status":"X",` +
			`"rules":{"rate_bps":500,"min_payment":20000,"daily_cap":50000,"timezone":"Asia/Jakarta"}}`, false},
		{"negative cap", `{"id":"x","name":"n","status":"ACTIVE","redemption_status":"AVAILABLE",` +
			`"rules":{"rate_bps":500,"min_payment":20000,"daily_cap":-1,"timezone":"Asia/Jakarta"}}`, false},
		{"missing rules", `{"id":"x","name":"n","status":"ACTIVE","redemption_status":"AVAILABLE"}`, false},
		{"missing rate_bps", `{"id":"x","name":"n","status":"ACTIVE","redemption_status":"AVAILABLE",` +
			`"rules":{"min_payment":20000,"daily_cap":50000,"timezone":"Asia/Jakarta"}}`, false},
		{"missing name", `{"id":"x","status":"ACTIVE","redemption_status":"AVAILABLE",` +
			`"rules":{"rate_bps":500,"min_payment":20000,"daily_cap":50000,"timezone":"Asia/Jakarta"}}`, false},
		{"empty id", `{"id":"","name":"n","status":"ACTIVE","redemption_status":"AVAILABLE",` +
			`"rules":{"rate_bps":500,"min_payment":20000,"daily_cap":1,"timezone":"Asia/Jakarta"}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, ok := decodeCampaign([]byte(tc.in))
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (view %+v)", ok, tc.ok, v)
			}
			if !ok && v != (domain.CampaignView{}) {
				t.Fatalf("a rejected value returned %+v", v)
			}
		})
	}
}

func TestDecodeCashback(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"valid", `{"balance":18000,"today":{"date":"2026-01-02","earned":5000,"remaining":45000,"resets_at":"2026-01-02T17:00:00Z"}}`, true},
		{"not json", `stale`, false},
		{"wrong type", `{"balance":"18000"}`, false},
		{"unknown field", `{"balance":1,"extra":1,"today":{"date":"2026-01-02","earned":0,"remaining":0,"resets_at":"2026-01-02T17:00:00Z"}}`, false},
		{"negative balance", `{"balance":-1,"today":{"date":"2026-01-02","earned":0,"remaining":0,"resets_at":"2026-01-02T17:00:00Z"}}`, false},
		{"negative earned", `{"balance":1,"today":{"date":"2026-01-02","earned":-1,"remaining":0,"resets_at":"2026-01-02T17:00:00Z"}}`, false},
		{"bad date", `{"balance":1,"today":{"date":"yesterday","earned":0,"remaining":0,"resets_at":"2026-01-02T17:00:00Z"}}`, false},
		{"bad resets_at", `{"balance":1,"today":{"date":"2026-01-02","earned":0,"remaining":0,"resets_at":"soon"}}`, false},
		{"empty object", `{}`, false},
		{"missing balance", `{"today":{"date":"2026-01-02","earned":0,"remaining":0,"resets_at":"2026-01-02T17:00:00Z"}}`, false},
		{"missing today", `{"balance":1}`, false},
		{"missing earned", `{"balance":1,"today":{"date":"2026-01-02","remaining":0,"resets_at":"2026-01-02T17:00:00Z"}}`, false},
		{"missing remaining", `{"balance":1,"today":{"date":"2026-01-02","earned":0,"resets_at":"2026-01-02T17:00:00Z"}}`, false},
		{"zero amounts are present", `{"balance":0,"today":{"date":"2026-01-02","earned":0,"remaining":0,"resets_at":"2026-01-02T17:00:00Z"}}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := decodeCashback([]byte(tc.in)); ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
		})
	}
}

func TestCashbackTTL(t *testing.T) {
	cases := []struct {
		name                string
		max, until, elapsed time.Duration
		want                time.Duration
		ok                  bool
	}{
		{"far from the reset: the configured TTL", 60 * time.Second, 5 * time.Hour, 40 * time.Millisecond, 60 * time.Second, true},
		{"reset sooner: capped at the reset", 60 * time.Second, 30 * time.Second, 0, 30 * time.Second, true},
		{"elapsed time since the database clock is taken off", 60 * time.Second, 30 * time.Second, 40 * time.Millisecond, 29960 * time.Millisecond, true},
		{"the configured TTL still caps after the elapsed time is taken off", 60 * time.Second, 60*time.Second + 100*time.Millisecond, 40 * time.Millisecond, 60 * time.Second, true},
		{"exactly one second left is cached", 60 * time.Second, time.Second, 0, time.Second, true},
		{"one second less the elapsed time is not cached", 60 * time.Second, time.Second, time.Millisecond, 0, false},
		{"just under one second is not cached", 60 * time.Second, 999 * time.Millisecond, 0, 0, false},
		{"the reset has passed", 60 * time.Second, -time.Second, 0, 0, false},
		{"elapsed past the reset", 60 * time.Second, time.Second, 5 * time.Second, 0, false},
		{"configured TTL under one second", 500 * time.Millisecond, time.Hour, 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := cashbackTTL(tc.max, tc.until, tc.elapsed)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("cashbackTTL(%v, %v, %v) = %v, %v; want %v, %v", tc.max, tc.until, tc.elapsed, got, ok, tc.want, tc.ok)
			}
		})
	}
}
