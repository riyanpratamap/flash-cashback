package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type healthBody struct {
	Status       string            `json:"status"`
	Dependencies map[string]string `json:"dependencies"`
}

func ok(context.Context) error   { return nil }
func fail(context.Context) error { return errors.New("down") }

func TestHealthz(t *testing.T) {
	tests := []struct {
		name       string
		pg, redis  func(context.Context) error
		wantCode   int
		wantStatus string
		wantPG     string
		wantRedis  string
	}{
		{"both up", ok, ok, 200, "ok", "ok", "ok"},
		{"redis down is degraded", ok, fail, 200, "ok", "ok", "degraded"},
		{"postgres down is 503", fail, ok, 503, "unavailable", "down", "ok"},
		{"both down is 503", fail, fail, 503, "unavailable", "down", "degraded"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := NewRouter(Deps{PingPostgres: tc.pg, PingRedis: tc.redis})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/healthz", nil))
			if rec.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d", rec.Code, tc.wantCode)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q", ct)
			}
			var got healthBody
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("body %q: %v", rec.Body.String(), err)
			}
			if got.Status != tc.wantStatus || got.Dependencies["postgres"] != tc.wantPG || got.Dependencies["redis"] != tc.wantRedis {
				t.Errorf("body = %+v", got)
			}
		})
	}
}

func TestHealthzDeadlines(t *testing.T) {
	var pgLeft, redisLeft time.Duration
	probe := func(dst *time.Duration) func(context.Context) error {
		return func(ctx context.Context) error {
			dl, has := ctx.Deadline()
			if !has {
				return errors.New("no deadline")
			}
			*dst = time.Until(dl)
			return nil
		}
	}
	h := NewRouter(Deps{PingPostgres: probe(&pgLeft), PingRedis: probe(&redisLeft)})
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/healthz", nil))
	if pgLeft <= 500*time.Millisecond || pgLeft > time.Second {
		t.Errorf("postgres deadline left = %v, want about 1s", pgLeft)
	}
	if redisLeft <= 0 || redisLeft > 50*time.Millisecond {
		t.Errorf("redis deadline left = %v, want at most 50ms", redisLeft)
	}
}
