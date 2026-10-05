package domain

import "testing"

func TestStatusAC41(t *testing.T) {
	const budget = 10_000_000
	tests := []struct {
		name            string
		spent           int64
		awardsPaused    bool
		redemptionPause bool
		status          CampaignStatus
		redemption      RedemptionStatus
	}{
		{"below, running", budget - 1, false, false, CampaignActive, RedemptionAvailable},
		{"below, awards paused", budget - 1, true, false, CampaignPaused, RedemptionAvailable},
		{"equal, running", budget, false, false, CampaignEnded, RedemptionAvailable},
		{"equal, both paused", budget, true, true, CampaignEnded, RedemptionPaused},
		{"below, redemptions paused", budget - 1, false, true, CampaignActive, RedemptionPaused},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Status(tc.spent, budget, tc.awardsPaused); got != tc.status {
				t.Errorf("Status = %q, want %q", got, tc.status)
			}
			if got := RedemptionStatusOf(tc.redemptionPause); got != tc.redemption {
				t.Errorf("RedemptionStatusOf = %q, want %q", got, tc.redemption)
			}
		})
	}
}

func TestTodayRemaining(t *testing.T) {
	tests := []struct {
		name        string
		campaignCap int64
		day         *UserDay
		want        int64
	}{
		{"no row uses campaign cap", 50000, nil, 50000},
		{"earned 47000", 50000, &UserDay{Earned: 47000, DailyCap: 50000}, 3000},
		{"earned at cap", 50000, &UserDay{Earned: 50000, DailyCap: 50000}, 0},
		{"row cap wins over campaign cap", 60000, &UserDay{Earned: 47000, DailyCap: 50000}, 3000},
		{"no row, campaign cap 60000", 60000, nil, 60000},
		{"never negative", 50000, &UserDay{Earned: 60000, DailyCap: 50000}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := TodayRemaining(tc.campaignCap, tc.day); got != tc.want {
				t.Errorf("TodayRemaining = %d, want %d", got, tc.want)
			}
		})
	}
}
