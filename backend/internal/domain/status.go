package domain

// CampaignStatus is the public campaign state.
type CampaignStatus string

const (
	CampaignActive CampaignStatus = "ACTIVE"
	CampaignPaused CampaignStatus = "PAUSED"
	CampaignEnded  CampaignStatus = "ENDED"
)

// RedemptionStatus is the public redemption state.
type RedemptionStatus string

const (
	RedemptionAvailable RedemptionStatus = "AVAILABLE"
	RedemptionPaused    RedemptionStatus = "PAUSED"
)

// Status derives the campaign status: ended wins over paused.
func Status(spent, budget int64, awardsPaused bool) CampaignStatus {
	switch {
	case spent >= budget:
		return CampaignEnded
	case awardsPaused:
		return CampaignPaused
	default:
		return CampaignActive
	}
}

// RedemptionStatusOf maps the redemptions-paused flag.
func RedemptionStatusOf(paused bool) RedemptionStatus {
	if paused {
		return RedemptionPaused
	}
	return RedemptionAvailable
}

// UserDay is today's user-day row.
type UserDay struct{ Earned, DailyCap int64 }

// TodayRemaining is max(cap-earned, 0); the cap is the row's when it exists,
// else the campaign's.
func TodayRemaining(campaignCap int64, day *UserDay) int64 {
	limit, earned := campaignCap, int64(0)
	if day != nil {
		limit, earned = day.DailyCap, day.Earned
	}
	return max(limit-earned, 0)
}
