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
