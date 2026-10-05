// Package service holds the use cases: they call the store and the domain.
package service

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/riyanpratamap/flash-cashback/backend/internal/cache"
	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// Reads serves the read endpoints; only GET /campaign goes through the cache.
type Reads struct {
	pool  *pgxpool.Pool
	cache *cache.ReadCache // nil: caching is off (AC-76)
}

// NewReads builds Reads on the pool. A nil rc turns the read cache off.
func NewReads(pool *pgxpool.Pool, rc *cache.ReadCache) *Reads { return &Reads{pool: pool, cache: rc} }

const (
	campaignID = "flash-cashback"
	timezone   = "Asia/Jakarta"
)

// Campaign returns the campaign state and rules, from the cache when it holds
// a valid body. Only a successful read is stored (§6).
func (r *Reads) Campaign(ctx context.Context) (domain.CampaignView, error) {
	lookup := cache.Unavailable // no cache: nothing to fill
	if r.cache != nil {
		var v domain.CampaignView
		if v, lookup = r.cache.Campaign(ctx); lookup == cache.Hit {
			return v, nil
		}
	}
	v, err := r.campaignFromDB(ctx)
	if err != nil {
		return domain.CampaignView{}, err
	}
	// A failed Redis is not asked again in this request (AC-44).
	if r.cache != nil && lookup == cache.Miss {
		r.cache.SetCampaign(ctx, v)
	}
	return v, nil
}

func (r *Reads) campaignFromDB(ctx context.Context) (domain.CampaignView, error) {
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

// Cashback returns the user's balance and today's progress from PostgreSQL;
// it is never cached (D53). A user with no rows gets zeros and the
// campaign's cap.
func (r *Reads) Cashback(ctx context.Context, user domain.UserID) (domain.CashbackView, error) {
	t, err := store.Today(ctx, r.pool, campaignID, user)
	if err != nil {
		return domain.CashbackView{}, err
	}
	return cashbackView(t), nil
}

func cashbackView(t store.TodayRow) domain.CashbackView {
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
	}
}

// History returns the user's newest payments and redemptions, newest first.
// A user with no rows gets an empty list, not null.
func (r *Reads) History(ctx context.Context, user domain.UserID, limit int) (domain.HistoryView, error) {
	rows, err := store.Newest(ctx, r.pool, user, limit)
	if err != nil {
		return domain.HistoryView{}, err
	}
	items := make([]domain.HistoryItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, historyItem(row))
	}
	return domain.HistoryView{Items: items}, nil
}

func historyItem(row store.HistoryRow) domain.HistoryItem {
	item := domain.HistoryItem{
		Type: row.Type, ID: row.ID, Amount: row.Amount, Status: row.Status,
		CreatedAt: domain.FormatTime(row.CreatedAt),
	}
	if row.Type == domain.HistoryPayment {
		item.Reference = domain.Reference(domain.RefPayment, row.Day, row.ID)
		if row.CashbackAwarded != nil && row.CashbackReason != nil {
			item.Cashback = &domain.AwardView{Awarded: *row.CashbackAwarded, Reason: domain.Reason(*row.CashbackReason)}
		}
		return item
	}
	item.Reference = domain.Reference(domain.RefRedemption, row.Day, row.ID)
	if row.Destination != nil {
		item.Destination = *row.Destination
	}
	return item
}
