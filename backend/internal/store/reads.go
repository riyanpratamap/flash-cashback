package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
)

// Campaign is the campaign row. Budget and Spent stay in this type: no
// response view carries them.
type Campaign struct {
	ID                string
	Name              string
	RateBps           int
	MinPayment        int64
	DailyCap          int64
	Budget            int64
	Spent             int64
	AwardsPaused      bool
	RedemptionsPaused bool
}

// GetCampaign reads the campaign row without a lock.
func GetCampaign(ctx context.Context, pool *pgxpool.Pool, id string) (Campaign, error) {
	var c Campaign
	err := pool.QueryRow(ctx, `SELECT id, name, rate_bps, min_payment, daily_cap, budget, spent,
			awards_paused, redemptions_paused
		FROM campaigns WHERE id = $1`, id).Scan(&c.ID, &c.Name, &c.RateBps, &c.MinPayment, &c.DailyCap,
		&c.Budget, &c.Spent, &c.AwardsPaused, &c.RedemptionsPaused)
	if err != nil {
		return Campaign{}, fmt.Errorf("get campaign: %w", err)
	}
	return c, nil
}

// TodayRow is the user's balance and today's user-day row.
type TodayRow struct {
	Day         time.Time
	ResetsAt    time.Time
	Balance     int64
	CampaignCap int64
	UserDay     *domain.UserDay // nil when the user has no row for the day
}

// Today reads the campaign day from the database clock, the user's balance
// (0 when there is no row) and the user-day row, in one statement.
func Today(ctx context.Context, pool *pgxpool.Pool, campaignID string, user domain.UserID) (TodayRow, error) {
	var (
		row            TodayRow
		earned, dayCap *int64 // NULL when the user has no row for the day
	)
	err := pool.QueryRow(ctx, `WITH d AS (SELECT fc_campaign_day(fc_now()) AS day)
		SELECT d.day, fc_day_resets_at(d.day),
		       COALESCE((SELECT balance FROM cashback_balances WHERE user_id = $2), 0),
		       u.earned, u.daily_cap, c.daily_cap
		FROM d CROSS JOIN campaigns c
		LEFT JOIN user_daily_earnings u ON u.campaign_id = c.id AND u.user_id = $2 AND u.day = d.day
		WHERE c.id = $1`, campaignID, string(user)).Scan(
		&row.Day, &row.ResetsAt, &row.Balance, &earned, &dayCap, &row.CampaignCap)
	if err != nil {
		return TodayRow{}, fmt.Errorf("read today: %w", err)
	}
	if earned != nil && dayCap != nil {
		row.UserDay = &domain.UserDay{Earned: *earned, DailyCap: *dayCap}
	}
	return row, nil
}
