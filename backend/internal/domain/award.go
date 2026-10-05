// Package domain holds the pure rules: no I/O, no clock, stdlib only.
package domain

import "errors"

// Reason is the outcome code of one award decision.
type Reason string

const (
	ReasonAwarded         Reason = "AWARDED"
	ReasonPartialDailyCap Reason = "PARTIAL_DAILY_CAP"
	ReasonPartialBudget   Reason = "PARTIAL_BUDGET"
	ReasonBelowMinimum    Reason = "BELOW_MINIMUM"
	ReasonCampaignEnded   Reason = "CAMPAIGN_ENDED"
	ReasonCampaignPaused  Reason = "CAMPAIGN_PAUSED"
	ReasonDailyCapReached Reason = "DAILY_CAP_REACHED"
)

// MaxAmount bounds a payment so amount*rate cannot overflow int64.
const MaxAmount int64 = 10_000_000

// ErrOutOfRange reports an amount or rate outside the bounds Award accepts.
var ErrOutOfRange = errors.New("domain: value out of range")

// Rule is the campaign rule an award is decided under. DailyCap is the
// user-day row's cap, not the live campaign cap (AC-71).
type Rule struct {
	RateBPS, MinPayment, DailyCap int64
}

// Award decides the cashback for one payment.
func Award(amount int64, r Rule, earned, budget, spent int64, awardsPaused bool) (int64, Reason, error) {
	if amount < 1 || amount > MaxAmount || r.RateBPS < 1 || r.RateBPS > 10000 {
		return 0, "", ErrOutOfRange
	}
	// Mirrors campaigns_min_positive and campaigns_min_earns; MinPayment is
	// bounded first so the product cannot overflow.
	if r.MinPayment < 1 || r.MinPayment > MaxAmount || r.MinPayment*r.RateBPS < 10000 {
		return 0, "", ErrOutOfRange
	}
	switch {
	case amount < r.MinPayment:
		return 0, ReasonBelowMinimum, nil
	case spent >= budget:
		return 0, ReasonCampaignEnded, nil
	case awardsPaused:
		return 0, ReasonCampaignPaused, nil
	case r.DailyCap-earned <= 0:
		return 0, ReasonDailyCapReached, nil
	}
	full := amount * r.RateBPS / 10000
	capRoom := r.DailyCap - earned
	budgetLeft := budget - spent
	award := min(full, capRoom, budgetLeft)
	switch {
	case award == full:
		return award, ReasonAwarded, nil
	case award == budgetLeft:
		return award, ReasonPartialBudget, nil
	default:
		return award, ReasonPartialDailyCap, nil
	}
}
