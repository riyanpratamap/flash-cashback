// Package store holds the PostgreSQL access code.
package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Ping checks that PostgreSQL answers a query.
func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, "SELECT 1")
	return err
}
