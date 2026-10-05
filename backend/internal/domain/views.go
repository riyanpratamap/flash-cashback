package domain

// CampaignView is the body of GET /campaign. It has no budget or spent field:
// budget figures never leave the server (INV-10).
type CampaignView struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Status           CampaignStatus   `json:"status"`
	RedemptionStatus RedemptionStatus `json:"redemption_status"`
	Rules            RulesView        `json:"rules"`
}

// RulesView is the served rule set.
type RulesView struct {
	RateBps    int    `json:"rate_bps"`
	MinPayment int64  `json:"min_payment"`
	DailyCap   int64  `json:"daily_cap"`
	Timezone   string `json:"timezone"`
}

// CashbackView is the body of GET /me/cashback.
type CashbackView struct {
	Balance int64     `json:"balance"`
	Today   TodayView `json:"today"`
}

// TodayView is the current campaign day for one user.
type TodayView struct {
	Date      string `json:"date"`
	Earned    int64  `json:"earned"`
	Remaining int64  `json:"remaining"`
	ResetsAt  string `json:"resets_at"`
}
