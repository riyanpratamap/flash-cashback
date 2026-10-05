package config

import (
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	got, err := Load(env(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		HTTPAddr:         ":8080",
		DatabaseURL:      "postgres://flash:flash@postgres:5432/flash?sslmode=disable",
		DBMaxConns:       20,
		RedisAddr:        "redis:6379",
		RedisTimeout:     50 * time.Millisecond,
		CacheCampaignTTL: 5 * time.Second,
		CacheReads:       true,
		LockTimeout:      2 * time.Second,
		StatementTimeout: 5 * time.Second,
		CampaignBudget:   10000000,
		LogLevel:         "info",
		Demo:             false,
	}
	if got != want {
		t.Fatalf("defaults:\n got %+v\nwant %+v", got, want)
	}
	if got.LockTimeoutMS() != 2000 || got.StatementTimeoutMS() != 5000 {
		t.Fatalf("ms: lock %d statement %d", got.LockTimeoutMS(), got.StatementTimeoutMS())
	}
}

func TestLoadOverrides(t *testing.T) {
	got, err := Load(env(map[string]string{
		"HTTP_ADDR":          "0.0.0.0:9000",
		"DATABASE_URL":       "postgres://u:p@localhost:55432/db",
		"DB_MAX_CONNS":       "5",
		"REDIS_ADDR":         "localhost:56379",
		"REDIS_TIMEOUT":      "100ms",
		"CACHE_CAMPAIGN_TTL": "1s",
		"CACHE_READS":        "off",
		"LOCK_TIMEOUT":       "1500ms",
		"STATEMENT_TIMEOUT":  "7s",
		"CAMPAIGN_BUDGET":    "500000",
		"LOG_LEVEL":          "debug",
		"FC_DEMO":            "1",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		HTTPAddr:         "0.0.0.0:9000",
		DatabaseURL:      "postgres://u:p@localhost:55432/db",
		DBMaxConns:       5,
		RedisAddr:        "localhost:56379",
		RedisTimeout:     100 * time.Millisecond,
		CacheCampaignTTL: time.Second,
		LockTimeout:      1500 * time.Millisecond,
		StatementTimeout: 7 * time.Second,
		CampaignBudget:   500000,
		LogLevel:         "debug",
		Demo:             true,
	}
	if got != want {
		t.Fatalf("overrides:\n got %+v\nwant %+v", got, want)
	}
	if got.LockTimeoutMS() != 1500 || got.StatementTimeoutMS() != 7000 {
		t.Fatalf("ms: lock %d statement %d", got.LockTimeoutMS(), got.StatementTimeoutMS())
	}
}

func TestLoadCacheReadsOff(t *testing.T) {
	got, err := Load(env(map[string]string{"CACHE_READS": "off"}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.CacheReads {
		t.Fatal("CACHE_READS=off left CacheReads true")
	}
	got, err = Load(env(map[string]string{"CACHE_READS": "on"}))
	if err != nil || !got.CacheReads {
		t.Fatalf("CACHE_READS=on: %+v, %v", got, err)
	}
}

func TestLoadDemoOnlyExactlyOne(t *testing.T) {
	for _, v := range []string{"", "0", "true", "yes", "11", " 1"} {
		got, err := Load(env(map[string]string{"FC_DEMO": v}))
		if err != nil {
			t.Fatalf("FC_DEMO=%q: %v", v, err)
		}
		if got.Demo {
			t.Errorf("FC_DEMO=%q enabled demo", v)
		}
	}
}

func TestLoadRejects(t *testing.T) {
	cases := []struct {
		name, key, value string
	}{
		{"lock not a duration", "LOCK_TIMEOUT", "abc"},
		{"lock zero", "LOCK_TIMEOUT", "0s"},
		{"lock negative", "LOCK_TIMEOUT", "-1s"},
		{"lock sub-millisecond", "LOCK_TIMEOUT", "1500us"},
		{"statement sub-millisecond", "STATEMENT_TIMEOUT", "1500us"},
		{"statement zero", "STATEMENT_TIMEOUT", "0s"},
		{"redis timeout zero", "REDIS_TIMEOUT", "0s"},
		{"redis timeout negative", "REDIS_TIMEOUT", "-5ms"},
		{"campaign ttl bad", "CACHE_CAMPAIGN_TTL", "5"},
		{"max conns not a number", "DB_MAX_CONNS", "x"},
		{"max conns zero", "DB_MAX_CONNS", "0"},
		{"max conns negative", "DB_MAX_CONNS", "-3"},
		{"budget zero", "CAMPAIGN_BUDGET", "0"},
		{"budget negative", "CAMPAIGN_BUDGET", "-1"},
		{"budget fractional", "CAMPAIGN_BUDGET", "1.5"},
		{"budget not a number", "CAMPAIGN_BUDGET", "ten"},
		{"log level unknown", "LOG_LEVEL", "trace"},
		{"log level case", "LOG_LEVEL", "INFO"},
		{"cache reads unknown", "CACHE_READS", "maybe"},
		{"cache reads case", "CACHE_READS", "OFF"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(env(map[string]string{tc.key: tc.value}))
			if err == nil {
				t.Fatalf("%s=%q accepted", tc.key, tc.value)
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("error %q does not name %s", err, tc.key)
			}
		})
	}
}
