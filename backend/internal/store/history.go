package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
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
	TypeRank        int // 1 for a payment, 0 for a redemption
}

// The two page queries are static texts. Each UNION branch takes its own
// limit rows from its (user_id, created_at DESC, id DESC) index, so a long
// history is never read whole, and the outer sort merges them. Both columns
// are DESC, so the row comparison below matches the index. type_rank breaks a
// tie between a payment and a redemption at one instant (payment first).
// $2 is the limit plus one: the extra row says another page exists.
const (
	// HistoryFirstSQL is the first page: $1 user, $2 limit + 1.
	HistoryFirstSQL = `
		(SELECT 'PAYMENT' AS type, id, amount, status, NULL::text AS destination,
		        cashback_awarded, cashback_reason, campaign_day, created_at, 1 AS type_rank
		   FROM payments WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2)
		UNION ALL
		(SELECT 'REDEMPTION', id, amount, status, destination,
		        NULL::bigint, NULL::text, campaign_day, created_at, 0
		   FROM redemptions WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2)
		ORDER BY created_at DESC, type_rank DESC, id DESC LIMIT $2`

	// HistoryCursorSQL is a later page: $1 user, $2 limit + 1, $3 cursor time,
	// $4 payment id bound, $5 redemption id bound (domain.BranchBound).
	HistoryCursorSQL = `
		(SELECT 'PAYMENT' AS type, id, amount, status, NULL::text AS destination,
		        cashback_awarded, cashback_reason, campaign_day, created_at, 1 AS type_rank
		   FROM payments WHERE user_id = $1 AND (created_at, id) < ($3, $4)
		  ORDER BY created_at DESC, id DESC LIMIT $2)
		UNION ALL
		(SELECT 'REDEMPTION', id, amount, status, destination,
		        NULL::bigint, NULL::text, campaign_day, created_at, 0
		   FROM redemptions WHERE user_id = $1 AND (created_at, id) < ($3, $5)
		  ORDER BY created_at DESC, id DESC LIMIT $2)
		ORDER BY created_at DESC, type_rank DESC, id DESC LIMIT $2`
)

// HistoryPage returns the user's payments and redemptions together, newest
// first: at most limit + 1 rows, the position after cursor (nil is the first
// page). The caller drops the extra row.
func HistoryPage(ctx context.Context, pool *pgxpool.Pool, user domain.UserID, limit int, cursor *domain.Cursor) ([]HistoryRow, error) {
	var rows pgx.Rows
	var err error
	if cursor == nil {
		rows, err = pool.Query(ctx, HistoryFirstSQL, string(user), limit+1)
	} else {
		rows, err = pool.Query(ctx, HistoryCursorSQL, string(user), limit+1, cursor.T,
			domain.BranchBound(*cursor, domain.CursorPayment), domain.BranchBound(*cursor, domain.CursorRedemption))
	}
	if err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}
	defer rows.Close()
	var out []HistoryRow
	for rows.Next() {
		var r HistoryRow
		if err := rows.Scan(&r.Type, &r.ID, &r.Amount, &r.Status, &r.Destination,
			&r.CashbackAwarded, &r.CashbackReason, &r.Day, &r.CreatedAt, &r.TypeRank); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read history rows: %w", err)
	}
	return out, nil
}
