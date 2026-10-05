package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/riyanpratamap/flash-cashback/backend/internal/config"
	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
)

// warnEvery is the shortest gap between two warnings of the same operation
// (tech-spec §6: at most one per 10 s per operation).
const warnEvery = 10 * time.Second

// ReadCache is the read side of the cache (tech-spec §6): GET /campaign and
// GET /me/cashback bodies. It is a separate type from Invalidator, and only
// service.Reads holds it: no money path reads the cache (INV-11). Every Redis
// error, timeout, or invalid value is a miss; nothing here returns an error.
type ReadCache struct {
	c           *redis.Client
	timeout     time.Duration
	campaignTTL time.Duration
	cashbackTTL time.Duration
	log         *slog.Logger
	warn        *throttle
}

// NewReadCache builds a ReadCache on c. It returns nil when CACHE_READS is
// off (D51, AC-76): service.Reads treats a nil cache as no cache. A nil logger
// uses the default one.
func NewReadCache(c *redis.Client, cfg config.Config, log *slog.Logger) *ReadCache {
	if !cfg.CacheReads {
		return nil
	}
	if log == nil {
		log = slog.Default()
	}
	return &ReadCache{
		c: c, timeout: cfg.RedisTimeout, campaignTTL: cfg.CacheCampaignTTL, cashbackTTL: cfg.CacheCashbackTTL, log: log,
		warn: newThrottle(warnEvery, time.Now),
	}
}

// Lookup is the outcome of a cache read.
type Lookup int

const (
	// Miss: Redis answered and has no usable body (absent or invalid). The
	// caller reads PostgreSQL and refills the key.
	Miss Lookup = iota
	// Hit: a valid body was returned.
	Hit
	// Unavailable: Redis failed or the request was cancelled. The caller reads
	// PostgreSQL and does not SET, so one read pays at most one Redis deadline
	// (AC-44).
	Unavailable
)

// Campaign returns the cached GET /campaign body; the view is valid only on Hit.
func (r *ReadCache) Campaign(ctx context.Context) (domain.CampaignView, Lookup) {
	b, l := r.get(ctx, CampaignKey)
	if l != Hit {
		return domain.CampaignView{}, l
	}
	v, ok := decodeCampaign(b)
	if !ok {
		r.warnOp("decode", CampaignKey)
		return domain.CampaignView{}, Miss
	}
	return v, Hit
}

// SetCampaign stores the body for the campaign TTL.
func (r *ReadCache) SetCampaign(ctx context.Context, v domain.CampaignView) {
	r.set(ctx, CampaignKey, v, r.campaignTTL)
}

// Cashback returns the user's cached GET /me/cashback body; the view is valid
// only on Hit.
func (r *ReadCache) Cashback(ctx context.Context, user domain.UserID) (domain.CashbackView, Lookup) {
	key := CashbackKey(user)
	b, l := r.get(ctx, key)
	if l != Hit {
		return domain.CashbackView{}, l
	}
	v, ok := decodeCashback(b)
	if !ok {
		r.warnOp("decode", key)
		return domain.CashbackView{}, Miss
	}
	return v, Hit
}

// SetCashback stores the body for min(CACHE_CASHBACK_TTL, untilReset minus
// the time since started), where untilReset is the database clock's time to
// the end of the campaign day at the statement that read it and started is
// taken before that statement: Redis counts the TTL from the SET, so the
// entry must not outlive 00:00:00 (AC-46). A body with under a second left
// is not stored (§6).
func (r *ReadCache) SetCashback(ctx context.Context, user domain.UserID, v domain.CashbackView, untilReset time.Duration, started time.Time) {
	ttl, ok := cashbackTTL(r.cashbackTTL, untilReset, time.Since(started))
	if !ok {
		return
	}
	r.set(ctx, CashbackKey(user), v, ttl)
}

// get reads key on the read deadline. A cancelled request is a silent
// Unavailable: Redis is not at fault.
func (r *ReadCache) get(ctx context.Context, key string) ([]byte, Lookup) {
	parent := ctx
	ctx, cancel := context.WithTimeout(parent, r.timeout)
	defer cancel()
	b, err := r.c.Get(ctx, key).Bytes()
	switch {
	case err == nil:
		return b, Hit
	case errors.Is(err, redis.Nil):
		return nil, Miss
	case parent.Err() != nil && errors.Is(err, context.Canceled):
		return nil, Unavailable
	}
	r.warnOp("get", key)
	return nil, Unavailable
}

// set runs on a context detached from the request: the response is already
// decided, and a cancelled request must not skip a good fill.
func (r *ReadCache) set(ctx context.Context, key string, v any, ttl time.Duration) {
	b, err := json.Marshal(v)
	if err != nil {
		r.warnOp("set", key)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.timeout)
	defer cancel()
	if err := r.c.Set(ctx, key, b, ttl).Err(); err != nil {
		r.warnOp("set", key)
	}
}

// warnOp logs the key and the operation, never the Redis address or the
// driver's error text.
func (r *ReadCache) warnOp(op, key string) {
	if r.warn.allow(op) {
		r.log.Warn("cache read unavailable", "op", op, "key", key)
	}
}

// throttle lets one event per operation through every window.
type throttle struct {
	mu    sync.Mutex
	every time.Duration
	now   func() time.Time
	last  map[string]time.Time
}

func newThrottle(every time.Duration, now func() time.Time) *throttle {
	return &throttle{every: every, now: now, last: map[string]time.Time{}}
}

func (t *throttle) allow(op string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if last, seen := t.last[op]; seen && now.Sub(last) < t.every {
		return false
	}
	t.last[op] = now
	return true
}

// strictDecode parses exactly one JSON value into v and rejects unknown
// fields (a cached value is a boundary, KP).
func strictDecode(b []byte, v any) bool {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return false
	}
	return dec.Decode(&struct{}{}) == io.EOF
}

// campaignBody and cashbackBody mirror the views with pointer fields: a
// cached body with a field missing is told from one holding a zero, and is a
// miss. A partial result is never served (KP).
type campaignBody struct {
	ID               *string                  `json:"id"`
	Name             *string                  `json:"name"`
	Status           *domain.CampaignStatus   `json:"status"`
	RedemptionStatus *domain.RedemptionStatus `json:"redemption_status"`
	Rules            *struct {
		RateBps    *int    `json:"rate_bps"`
		MinPayment *int64  `json:"min_payment"`
		DailyCap   *int64  `json:"daily_cap"`
		Timezone   *string `json:"timezone"`
	} `json:"rules"`
}

type cashbackBody struct {
	Balance *int64 `json:"balance"`
	Today   *struct {
		Date      *string `json:"date"`
		Earned    *int64  `json:"earned"`
		Remaining *int64  `json:"remaining"`
		ResetsAt  *string `json:"resets_at"`
	} `json:"today"`
}

func decodeCampaign(b []byte) (domain.CampaignView, bool) {
	var m campaignBody
	if !strictDecode(b, &m) {
		return domain.CampaignView{}, false
	}
	if m.ID == nil || m.Name == nil || m.Status == nil || m.RedemptionStatus == nil || m.Rules == nil {
		return domain.CampaignView{}, false
	}
	rl := m.Rules
	if rl.RateBps == nil || rl.MinPayment == nil || rl.DailyCap == nil || rl.Timezone == nil {
		return domain.CampaignView{}, false
	}
	v := domain.CampaignView{
		ID: *m.ID, Name: *m.Name, Status: *m.Status, RedemptionStatus: *m.RedemptionStatus,
		Rules: domain.RulesView{RateBps: *rl.RateBps, MinPayment: *rl.MinPayment, DailyCap: *rl.DailyCap, Timezone: *rl.Timezone},
	}
	switch v.Status {
	case domain.CampaignActive, domain.CampaignPaused, domain.CampaignEnded:
	default:
		return domain.CampaignView{}, false
	}
	switch v.RedemptionStatus {
	case domain.RedemptionAvailable, domain.RedemptionPaused:
	default:
		return domain.CampaignView{}, false
	}
	r := v.Rules
	if v.ID == "" || r.RateBps < 0 || r.MinPayment < 0 || r.DailyCap < 0 || r.Timezone == "" {
		return domain.CampaignView{}, false
	}
	return v, true
}

func decodeCashback(b []byte) (domain.CashbackView, bool) {
	var m cashbackBody
	if !strictDecode(b, &m) {
		return domain.CashbackView{}, false
	}
	if m.Balance == nil || m.Today == nil {
		return domain.CashbackView{}, false
	}
	t := m.Today
	if t.Date == nil || t.Earned == nil || t.Remaining == nil || t.ResetsAt == nil {
		return domain.CashbackView{}, false
	}
	if *m.Balance < 0 || *t.Earned < 0 || *t.Remaining < 0 {
		return domain.CashbackView{}, false
	}
	if _, err := time.Parse("2006-01-02", *t.Date); err != nil {
		return domain.CashbackView{}, false
	}
	if _, err := time.Parse(time.RFC3339, *t.ResetsAt); err != nil {
		return domain.CashbackView{}, false
	}
	return domain.CashbackView{
		Balance: *m.Balance,
		Today:   domain.TodayView{Date: *t.Date, Earned: *t.Earned, Remaining: *t.Remaining, ResetsAt: *t.ResetsAt},
	}, true
}

// minCashbackTTL is the shortest lifetime worth a SET (§6).
const minCashbackTTL = time.Second

// cashbackTTL is min(configured, untilReset - elapsed): a cashback body never
// outlives the campaign day it describes, counting the time spent since the
// database clock was read. Under one second it is not cached.
func cashbackTTL(configured, untilReset, elapsed time.Duration) (time.Duration, bool) {
	ttl := min(configured, untilReset-elapsed)
	if ttl < minCashbackTTL {
		return 0, false
	}
	return ttl, true
}
