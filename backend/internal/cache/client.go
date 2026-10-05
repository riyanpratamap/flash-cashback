// Package cache holds the Redis client and the read caches (tech-spec §6).
package cache

import (
	"context"

	"github.com/redis/go-redis/v9"

	"github.com/riyanpratamap/flash-cashback/backend/internal/config"
)

// NewClient builds the Redis client with the options of tech-spec §6.
func NewClient(cfg config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
		// Without this go-redis ignores a context deadline (KP).
		ContextTimeoutEnabled: true,
		DialTimeout:           cfg.RedisTimeout,
		ReadTimeout:           cfg.RedisTimeout,
		WriteTimeout:          cfg.RedisTimeout,
		PoolTimeout:           cfg.RedisTimeout,
		// -1 disables retries; 0 would mean the default of 3.
		MaxRetries: -1,
	})
}

// Ping checks that Redis answers PING.
func Ping(ctx context.Context, c *redis.Client) error {
	return c.Ping(ctx).Err()
}
