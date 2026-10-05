package cache

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
)

// Cache keys (tech-spec §6).
const (
	keyPrefix = "fc:v1:"
	// CampaignKey holds the GET /campaign body.
	CampaignKey = keyPrefix + "campaign"
)

// CashbackKey is the key of one user's GET /me/cashback body.
func CashbackKey(u domain.UserID) string { return keyPrefix + "cashback:" + string(u) }

// deleteAllCap bounds DeleteAll: a SCAN walk takes more than one round trip.
const deleteAllCap = 2 * time.Second

// Invalidator deletes cache keys after a commit. It has no read side: the
// write services hold it and nothing else of the cache (INV-11).
type Invalidator struct {
	c       *redis.Client
	timeout time.Duration
	log     *slog.Logger
}

// NewInvalidator builds an Invalidator whose every delete is bounded by
// timeout. A nil logger uses the default one.
func NewInvalidator(c *redis.Client, timeout time.Duration, log *slog.Logger) *Invalidator {
	if log == nil {
		log = slog.Default()
	}
	return &Invalidator{c: c, timeout: timeout, log: log}
}

// Delete removes keys on a fresh context with its own deadline: the request
// context may already be cancelled once the commit is done (KP). A failure is
// logged and never returned: the response stays what the commit made it, and
// the key's TTL bounds the staleness (§4.4, §6). The log carries the keys,
// never the Redis address.
func (i *Invalidator) Delete(ctx context.Context, keys ...string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), i.timeout)
	defer cancel()
	if err := i.c.Del(ctx, keys...).Err(); err != nil {
		i.log.Warn("cache delete failed", "keys", keys, "error", "redis unavailable")
	}
}

// DeleteAll removes every fc:v1:* key. Only demo-reset uses it: the truncate
// makes every user's cashback body stale, not only the three demo users'.
func (i *Invalidator) DeleteAll(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), deleteAllCap)
	defer cancel()
	it := i.c.Scan(ctx, 0, keyPrefix+"*", 100).Iterator()
	for it.Next(ctx) {
		if err := i.c.Del(ctx, it.Val()).Err(); err != nil {
			return fmt.Errorf("cache delete all: %w", err)
		}
	}
	if err := it.Err(); err != nil {
		return fmt.Errorf("cache delete all: %w", err)
	}
	return nil
}
