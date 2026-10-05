// Package service holds the use cases: they call the store and the domain.
package service

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// Reads serves the two read endpoints.
type Reads struct{ pool *pgxpool.Pool }

// NewReads builds Reads on the pool.
func NewReads(pool *pgxpool.Pool) *Reads { return &Reads{pool: pool} }

const (
	campaignID = "flash-cashback"
	timezone   = "Asia/Jakarta"
)

// Campaign returns the campaign state and rules.
func (r *Reads) Campaign(ctx context.Context) (domain.CampaignView, error) {
	c, err := store.GetCampaign(ctx, r.pool, campaignID)
	if err != nil {
		return domain.CampaignView{}, err
	}
	return domain.CampaignView{
		ID:               c.ID,
		Name:             c.Name,
		Status:           domain.Status(c.Spent, c.Budget, c.AwardsPaused),
		RedemptionStatus: domain.RedemptionStatusOf(c.RedemptionsPaused),
		Rules: domain.RulesView{
			RateBps: c.RateBps, MinPayment: c.MinPayment, DailyCap: c.DailyCap, Timezone: timezone,
		},
	}, nil
}

// Cashback returns the user's balance and today's progress. A user with no
// rows gets zeros and the campaign's cap.
func (r *Reads) Cashback(ctx context.Context, user domain.UserID) (domain.CashbackView, error) {
	t, err := store.Today(ctx, r.pool, campaignID, user)
	if err != nil {
		return domain.CashbackView{}, err
	}
	var earned int64
	if t.UserDay != nil {
		earned = t.UserDay.Earned
	}
	return domain.CashbackView{
		Balance: t.Balance,
		Today: domain.TodayView{
			Date:      t.Day.Format("2006-01-02"),
			Earned:    earned,
			Remaining: domain.TodayRemaining(t.CampaignCap, t.UserDay),
			ResetsAt:  domain.FormatTime(t.ResetsAt),
		},
	}, nil
}
