package domain

import (
	"errors"
	"testing"
)

var stdRule = Rule{RateBPS: 500, MinPayment: 20000, DailyCap: 50000}

func TestAward(t *testing.T) {
	const big = 10_000_000
	tests := []struct {
		name          string
		amount        int64
		rule          Rule
		earned        int64
		budget, spent int64
		paused        bool
		want          int64
		reason        Reason
	}{
		{"below minimum 19999", 19999, stdRule, 0, big, 0, false, 0, ReasonBelowMinimum},
		{"AC-07 below minimum beats daily cap reached", 19999, stdRule, 50000, big, 0, false, 0, ReasonBelowMinimum},
		{"minimum 20000", 20000, stdRule, 0, big, 0, false, 1000, ReasonAwarded},
		{"above minimum 20001", 20001, stdRule, 0, big, 0, false, 1000, ReasonAwarded},
		{"AC-04 39999 rounds down", 39999, stdRule, 0, big, 0, false, 1999, ReasonAwarded},
		{"AC-01 full award under cap", 100000, stdRule, 0, big, 0, false, 5000, ReasonAwarded},
		{"award exactly reaches cap", 100000, stdRule, 45000, big, 0, false, 5000, ReasonAwarded},
		{"partial daily cap", 100000, stdRule, 47000, big, 0, false, 3000, ReasonPartialDailyCap},
		{"daily cap reached", 100000, stdRule, 50000, big, 0, false, 0, ReasonDailyCapReached},
		{"award takes last of budget", 100000, stdRule, 0, 5000, 0, false, 5000, ReasonAwarded},
		{"partial budget", 100000, stdRule, 0, 2000, 0, false, 2000, ReasonPartialBudget},
		{"cap and budget both short, budget smaller", 100000, stdRule, 47000, 2000, 0, false, 2000, ReasonPartialBudget},
		{"cap and budget tie is budget", 100000, stdRule, 48000, 2000, 0, false, 2000, ReasonPartialBudget},
		{"cap shorter than budget", 100000, stdRule, 48000, 3000, 0, false, 2000, ReasonPartialDailyCap},
		{"budget ended", 100000, stdRule, 0, 0, 0, true, 0, ReasonCampaignEnded},
		{"paused", 100000, stdRule, 50000, big, 0, true, 0, ReasonCampaignPaused},
		{"below minimum beats ended and paused", 19999, stdRule, 0, 0, 0, true, 0, ReasonBelowMinimum},
		{"max amount", 10_000_000, stdRule, 0, big, 0, false, 50000, ReasonPartialDailyCap},
		{"nonzero spent, budget left 2000", 100000, stdRule, 0, big, big - 2000, false, 2000, ReasonPartialBudget},
		{"spent equals budget is ended", 100000, stdRule, 0, big, big, false, 0, ReasonCampaignEnded},
		{"spent above budget is ended", 100000, stdRule, 0, big, big + 1, false, 0, ReasonCampaignEnded},
		{"ended beats paused", 100000, stdRule, 0, 0, 0, true, 0, ReasonCampaignEnded},
		{"paused beats daily cap reached", 100000, stdRule, 50000, big, 0, true, 0, ReasonCampaignPaused},
		{"ended beats daily cap reached", 100000, stdRule, 50000, 0, 0, false, 0, ReasonCampaignEnded},
		{"AC-71 row cap 50000 after campaign cap changed", 100000, Rule{500, 20000, 50000}, 47000, big, 0, false, 3000, ReasonPartialDailyCap},
		{"AC-71 row cap lower than standard", 100000, Rule{500, 20000, 30000}, 28000, big, 0, false, 2000, ReasonPartialDailyCap},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, reason, err := Award(tc.amount, tc.rule, tc.earned, tc.budget, tc.spent, tc.paused)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want || reason != tc.reason {
				t.Fatalf("Award = (%d, %s), want (%d, %s)", got, reason, tc.want, tc.reason)
			}
		})
	}
}

func TestAwardOutOfRange(t *testing.T) {
	tests := []struct {
		name   string
		amount int64
		rate   int64
		min    int64
	}{
		{"zero amount", 0, 500, 20000},
		{"negative amount", -1, 500, 20000},
		{"amount above max", MaxAmount + 1, 500, 20000},
		{"max int64 amount", 1<<63 - 1, 500, 20000},
		{"zero rate", 20000, 0, 20000},
		{"negative rate", 20000, -5, 20000},
		{"rate above 10000", 20000, 10001, 20000},
		{"zero minimum", 20000, 500, 0},
		{"negative minimum", 20000, 500, -1},
		{"minimum earns nothing 19 at 500 bps", 20000, 500, 19},
		{"minimum above max amount", 20000, 500, MaxAmount + 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Rule{RateBPS: tc.rate, MinPayment: tc.min, DailyCap: 50000}
			got, reason, err := Award(tc.amount, r, 0, 10_000_000, 0, false)
			if !errors.Is(err, ErrOutOfRange) {
				t.Fatalf("err = %v, want ErrOutOfRange", err)
			}
			if got != 0 || reason != "" {
				t.Fatalf("got (%d, %q) with error, want zero values", got, reason)
			}
		})
	}
}

// TestAwardInvariants is INV-07 over a grid of inputs.
func TestAwardInvariants(t *testing.T) {
	amounts := []int64{1, 19999, 20000, 20001, 39999, 100000, 999999, MaxAmount}
	earneds := []int64{0, 1, 45000, 47000, 48000, 49999, 50000}
	budgetLeft := []int64{0, 1, 999, 2000, 3000, 5000, 49999, 10_000_000}
	rates := []int64{1, 500, 10000}
	const budget = 10_000_000
	for _, amount := range amounts {
		for _, rate := range rates {
			for _, earned := range earneds {
				for _, left := range budgetLeft {
					for _, paused := range []bool{false, true} {
						r := Rule{RateBPS: rate, MinPayment: 20000, DailyCap: 50000}
						spent := int64(budget) - left
						got, reason, err := Award(amount, r, earned, budget, spent, paused)
						if err != nil {
							t.Fatalf("amount=%d rate=%d: %v", amount, rate, err)
						}
						if got < 0 || got*10000 > amount*rate {
							t.Fatalf("amount=%d rate=%d: award %d exceeds full", amount, rate, got)
						}
						if got > r.DailyCap-earned && got > 0 {
							t.Fatalf("award %d exceeds cap room %d", got, r.DailyCap-earned)
						}
						if got > left {
							t.Fatalf("award %d exceeds budget left %d", got, left)
						}
						pays := reason == ReasonAwarded || reason == ReasonPartialDailyCap || reason == ReasonPartialBudget
						if (got > 0) != pays {
							t.Fatalf("amount=%d rate=%d earned=%d left=%d paused=%v: award %d with reason %s",
								amount, rate, earned, left, paused, got, reason)
						}
					}
				}
			}
		}
	}
}
