package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

const (
	pgHealthTimeout    = time.Second
	redisHealthTimeout = 50 * time.Millisecond
)

type healthResponse struct {
	Status       string            `json:"status"`
	Dependencies map[string]string `json:"dependencies"`
}

// getHealth: PostgreSQL failing is 503; Redis failing is 200 with Redis
// "degraded", because Redis only holds read caches (D06, C14, C16).
func getHealth(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pgErr := probe(r.Context(), pgHealthTimeout, d.PingPostgres)
		redisErr := probe(r.Context(), redisHealthTimeout, d.PingRedis)

		resp := healthResponse{
			Status:       "ok",
			Dependencies: map[string]string{"postgres": "ok", "redis": "ok"},
		}
		code := http.StatusOK
		if redisErr != nil {
			resp.Dependencies["redis"] = "degraded"
		}
		if pgErr != nil {
			resp.Status = "unavailable"
			resp.Dependencies["postgres"] = "down"
			code = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func probe(ctx context.Context, timeout time.Duration, ping func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return ping(ctx)
}
