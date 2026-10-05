// Package cache holds the Redis client and the read caches (tech-spec §6).
package cache

import (
	"context"
	"sync"

	"github.com/redis/go-redis/v9"

	"github.com/riyanpratamap/flash-cashback/backend/internal/config"
)

// silent drops go-redis's own log lines: on a failed dial it prints the Redis
// address to stderr. Our code logs the failure itself, without the address.
type silent struct{}

func (silent) Printf(context.Context, string, ...interface{}) {}

var silenceOnce sync.Once

// NewClient builds the Redis client with the options of tech-spec §6. It also
// silences the driver's process-wide logger, once.
func NewClient(cfg config.Config) *redis.Client {
	silenceOnce.Do(func() { redis.SetLogger(silent{}) })
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
