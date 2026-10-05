package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
)

// HistoryRow is one payment or redemption. The columns that exist in only
// one table are NULL in the other, so they scan into pointers.
type HistoryRow struct {
	Type            string
	ID              int64
	Amount          int64
	Status          string
	Destination     *string
	CashbackAwarded *int64
	CashbackReason  *string
	Day             time.Time
	CreatedAt       time.Time
}

// Newest returns the user's newest payments and redemptions together, newest
// first, at most limit rows. Each branch takes its own limit rows from its
// (user_id, created_at DESC, id DESC) index, so a long history is never read
// whole; the outer sort and limit then merge the two.
func Newest(ctx context.Context, pool *pgxpool.Pool, user domain.UserID, limit int) ([]HistoryRow, error) {
	rows, err := pool.Query(ctx, `
		(SELECT 'PAYMENT' AS type, id, amount, status, NULL::text AS destination,
		        cashback_awarded, cashback_reason, campaign_day, created_at
		   FROM payments WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2)
		UNION ALL
		(SELECT 'REDEMPTION', id, amount, status, destination,
		        NULL::bigint, NULL::text, campaign_day, created_at
		   FROM redemptions WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2)
		ORDER BY created_at DESC, id DESC LIMIT $2`, string(user), limit)
	if err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}
	defer rows.Close()
	var out []HistoryRow
	for rows.Next() {
		var r HistoryRow
		if err := rows.Scan(&r.Type, &r.ID, &r.Amount, &r.Status, &r.Destination,
			&r.CashbackAwarded, &r.CashbackReason, &r.Day, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read history rows: %w", err)
	}
	return out, nil
}
