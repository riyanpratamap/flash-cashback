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

// PaymentStatusSucceeded is the only payment status the schema allows.
const PaymentStatusSucceeded = "SUCCEEDED"

// PaymentResult is the body of POST /payments.
type PaymentResult struct {
	Payment  PaymentView `json:"payment"`
	Cashback AwardView   `json:"cashback"`
}

// PaymentView is the payment itself.
type PaymentView struct {
	ID        int64  `json:"id"`
	Reference string `json:"reference"`
	Amount    int64  `json:"amount"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// AwardView is the cashback decision for the payment.
type AwardView struct {
	Awarded int64  `json:"awarded"`
	Reason  Reason `json:"reason"`
}

// Redemption status and destination: the only values the schema allows a
// completed redemption (the payout is a stub, TC13).
const (
	RedemptionCompleted    = "COMPLETED"
	DestinationMainAccount = "MAIN_ACCOUNT"
)

// RedemptionResult is the body of POST /redemptions. BalanceAfter is stored
// with the redemption, so a replay returns the original value.
type RedemptionResult struct {
	Redemption   RedemptionView `json:"redemption"`
	BalanceAfter int64          `json:"balance_after"`
}

// RedemptionView is the redemption itself.
type RedemptionView struct {
	ID          int64  `json:"id"`
	Reference   string `json:"reference"`
	Amount      int64  `json:"amount"`
	Status      string `json:"status"`
	Destination string `json:"destination"`
	CreatedAt   string `json:"created_at"`
}

// History item types.
const (
	HistoryPayment    = "PAYMENT"
	HistoryRedemption = "REDEMPTION"
)

// HistoryView is the body of GET /me/history.
type HistoryView struct {
	Items []HistoryItem `json:"items"`
}

// HistoryItem is one payment or redemption row. Cashback is set on payments
// only and Destination on redemptions only.
type HistoryItem struct {
	Type        string     `json:"type"`
	ID          int64      `json:"id"`
	Reference   string     `json:"reference"`
	Amount      int64      `json:"amount"`
	Status      string     `json:"status"`
	Destination string     `json:"destination,omitempty"`
	CreatedAt   string     `json:"created_at"`
	Cashback    *AwardView `json:"cashback,omitempty"`
}
