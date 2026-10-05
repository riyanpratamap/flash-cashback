//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/riyanpratamap/flash-cashback/backend/internal/cache"
	"github.com/riyanpratamap/flash-cashback/backend/internal/config"
	"github.com/riyanpratamap/flash-cashback/backend/internal/httpapi"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

const (
	defaultTestRedisAddr = "localhost:56379"
	closedPort           = "127.0.0.1:1"
)

func testRedisAddr() string {
	if v := os.Getenv("FC_TEST_REDIS_ADDR"); v != "" {
		return v
	}
	return defaultTestRedisAddr
}

type healthResult struct {
	code   int
	status string
	deps   map[string]string
}

// getHealth serves the real router with real pings against the given
// PostgreSQL pool and Redis address.
func getHealth(t *testing.T, pg *pgxpool.Pool, redisAddr string) healthResult {
	t.Helper()
	cfg, err := config.Load(func(k string) string {
		switch k {
		case "REDIS_ADDR":
			return redisAddr
		case "REDIS_TIMEOUT":
			return "50ms"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	rc := cache.NewClient(cfg)
	t.Cleanup(func() { _ = rc.Close() })
	h := httpapi.NewRouter(httpapi.Deps{
		PingPostgres: func(ctx context.Context) error { return store.Ping(ctx, pg) },
		PingRedis:    func(ctx context.Context) error { return cache.Ping(ctx, rc) },
	})
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/healthz")
	if err != nil {
		t.Fatalf("GET healthz: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		Status       string            `json:"status"`
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return healthResult{resp.StatusCode, body.Status, body.Dependencies}
}

func TestHealthBothUp(t *testing.T) {
	got := getHealth(t, pool, testRedisAddr())
	if got.code != 200 || got.status != "ok" || got.deps["postgres"] != "ok" || got.deps["redis"] != "ok" {
		t.Fatalf("got %+v", got)
	}
}

func TestHealthRedisDownIsDegraded(t *testing.T) {
	start := time.Now()
	got := getHealth(t, pool, closedPort)
	if got.code != 200 || got.status != "ok" || got.deps["postgres"] != "ok" || got.deps["redis"] != "degraded" {
		t.Fatalf("got %+v", got)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("a down Redis cost %v, want well under 500ms", d)
	}
}

func TestHealthPostgresDownIs503(t *testing.T) {
	// pgxpool.New does not connect, so this pool points at a closed port.
	dead, err := pgxpool.New(context.Background(), "postgres://flash:flash@127.0.0.1:1/flash?sslmode=disable")
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(dead.Close)
	got := getHealth(t, dead, testRedisAddr())
	if got.code != 503 || got.status != "unavailable" || got.deps["postgres"] != "down" || got.deps["redis"] != "ok" {
		t.Fatalf("got %+v", got)
	}
}
