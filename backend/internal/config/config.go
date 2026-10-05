// Package config reads the process configuration from the environment
// (tech-spec §8). Every variable has a working default and is validated here,
// so the rest of the code relies on typed values.
package config

import (
	"fmt"
	"strconv"
	"time"
)

// Config is the process configuration.
type Config struct {
	HTTPAddr         string
	DatabaseURL      string
	DBMaxConns       int
	RedisAddr        string
	RedisTimeout     time.Duration
	CacheCampaignTTL time.Duration
	CacheCashbackTTL time.Duration
	// CacheReads is false only when CACHE_READS is "off" (D51, AC-76).
	CacheReads bool
	// LockTimeout and StatementTimeout are whole milliseconds: they are
	// formatted into SET LOCAL, which takes no bind parameters.
	LockTimeout      time.Duration
	StatementTimeout time.Duration
	CampaignBudget   int64
	LogLevel         string
	// Demo is true only when FC_DEMO is exactly "1".
	Demo bool
}

// LockTimeoutMS is LockTimeout in whole milliseconds.
func (c Config) LockTimeoutMS() int64 { return c.LockTimeout.Milliseconds() }

// StatementTimeoutMS is StatementTimeout in whole milliseconds.
func (c Config) StatementTimeoutMS() int64 { return c.StatementTimeout.Milliseconds() }

// Load reads the configuration through getenv; an empty value means the
// default. The error names the offending variable.
func Load(getenv func(string) string) (Config, error) {
	l := loader{getenv: getenv}
	c := Config{
		HTTPAddr:         l.str("HTTP_ADDR", ":8080"),
		DatabaseURL:      l.str("DATABASE_URL", "postgres://flash:flash@postgres:5432/flash?sslmode=disable"),
		DBMaxConns:       int(l.positiveInt("DB_MAX_CONNS", 20, 1<<31-1)),
		RedisAddr:        l.str("REDIS_ADDR", "redis:6379"),
		RedisTimeout:     l.duration("REDIS_TIMEOUT", 50*time.Millisecond, false),
		CacheCampaignTTL: l.duration("CACHE_CAMPAIGN_TTL", 5*time.Second, false),
		CacheCashbackTTL: l.duration("CACHE_CASHBACK_TTL", 60*time.Second, false),
		CacheReads:       l.onOff("CACHE_READS", true),
		LockTimeout:      l.duration("LOCK_TIMEOUT", 2*time.Second, true),
		StatementTimeout: l.duration("STATEMENT_TIMEOUT", 5*time.Second, true),
		CampaignBudget:   l.positiveInt("CAMPAIGN_BUDGET", 10000000, 1<<62),
		LogLevel:         l.logLevel("LOG_LEVEL", "info"),
		Demo:             getenv("FC_DEMO") == "1",
	}
	if l.err != nil {
		return Config{}, l.err
	}
	return c, nil
}

// loader keeps the first error so Load reads as a flat list.
type loader struct {
	getenv func(string) string
	err    error
}

func (l *loader) fail(key string, format string, args ...any) {
	if l.err == nil {
		l.err = fmt.Errorf("config %s: %s", key, fmt.Sprintf(format, args...))
	}
}

func (l *loader) str(key, def string) string {
	if v := l.getenv(key); v != "" {
		return v
	}
	return def
}

func (l *loader) positiveInt(key string, def, max int64) int64 {
	v := l.getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		l.fail(key, "%q is not an integer", v)
		return def
	}
	if n <= 0 || n > max {
		l.fail(key, "%d is outside 1..%d", n, max)
		return def
	}
	return n
}

func (l *loader) duration(key string, def time.Duration, wholeMS bool) time.Duration {
	v := l.getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.fail(key, "%q is not a duration such as 500ms or 2s", v)
		return def
	}
	if d <= 0 {
		l.fail(key, "%q must be greater than zero", v)
		return def
	}
	if wholeMS && d%time.Millisecond != 0 {
		l.fail(key, "%q must be a whole number of milliseconds", v)
		return def
	}
	return d
}

func (l *loader) logLevel(key, def string) string {
	v := l.str(key, def)
	switch v {
	case "debug", "info", "warn", "error":
		return v
	}
	l.fail(key, "%q is not one of debug, info, warn, error", v)
	return def
}

func (l *loader) onOff(key string, def bool) bool {
	switch v := l.getenv(key); v {
	case "":
		return def
	case "on":
		return true
	case "off":
		return false
	default:
		l.fail(key, "%q is not one of on, off", v)
		return def
	}
}
