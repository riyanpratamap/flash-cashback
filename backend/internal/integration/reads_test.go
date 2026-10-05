//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/riyanpratamap/flash-cashback/backend/internal/httpapi"
	"github.com/riyanpratamap/flash-cashback/backend/internal/service"
)

var budgetOrSpent = regexp.MustCompile(`(?i)budget|spent`)

func readsRouter() http.Handler {
	return httpapi.NewRouter(httpapi.Deps{
		PingPostgres: func(context.Context) error { return nil },
		PingRedis:    func(context.Context) error { return nil },
		Reads:        service.NewReads(pool),
	})
}

// getBody serves one GET through the real router and returns the raw body
// and its decoded generic form.
func getBody(t *testing.T, path, user string) ([]byte, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-User-ID", user)
	rec := httptest.NewRecorder()
	readsRouter().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", path, rec.Code, rec.Body)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("GET %s body: %v", path, err)
	}
	return rec.Body.Bytes(), m
}

func noBudgetKey(t *testing.T, v any, path string) {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	for k, c := range m {
		if budgetOrSpent.MatchString(k) {
			t.Errorf("key %q in %s", path+k, path)
		}
		noBudgetKey(t, c, path+k+".")
	}
}

func TestCashbackNewUserWIBDay(t *testing.T) {
	for name, tc := range map[string]struct {
		now           string
		date, resetAt string
	}{
		"last second of WIB day": {"2026-10-03T16:59:59Z", "2026-10-03", "2026-10-04T00:00:00+07:00"},
		"first second of next":   {"2026-10-03T17:00:00Z", "2026-10-04", "2026-10-05T00:00:00+07:00"},
	} {
		t.Run(name, func(t *testing.T) {
			reset(t, 10_000_000)
			ts, err := time.Parse(time.RFC3339, tc.now)
			if err != nil {
				t.Fatal(err)
			}
			setNow(t, ts)
			_, got := getBody(t, "/v1/me/cashback", "user_new")
			want := map[string]any{
				"balance": 0.0,
				"today": map[string]any{
					"date": tc.date, "earned": 0.0, "remaining": 50000.0, "resets_at": tc.resetAt,
				},
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("body = %v, want %v", got, want)
			}
		})
	}
}

func TestCampaignStatusRows(t *testing.T) {
	for name, tc := range map[string]struct {
		sql                string
		status, redemption string
	}{
		"row 1 active":             {``, "ACTIVE", "AVAILABLE"},
		"row 2 awards paused":      {`UPDATE campaigns SET awards_paused = true`, "PAUSED", "AVAILABLE"},
		"row 5 redemptions paused": {`UPDATE campaigns SET redemptions_paused = true`, "ACTIVE", "PAUSED"},
	} {
		t.Run(name, func(t *testing.T) {
			reset(t, 10_000_000)
			if tc.sql != "" {
				if _, err := pool.Exec(context.Background(), tc.sql); err != nil {
					t.Fatal(err)
				}
			}
			_, got := getBody(t, "/v1/campaign", "user_new")
			want := map[string]any{
				"id": "flash-cashback", "name": "Flash Cashback",
				"status": tc.status, "redemption_status": tc.redemption,
				"rules": map[string]any{
					"rate_bps": 500.0, "min_payment": 20000.0, "daily_cap": 50000.0, "timezone": "Asia/Jakarta",
				},
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("body = %v, want %v", got, want)
			}
		})
	}
}

func TestReadsNeverExposeBudget(t *testing.T) {
	for name, sql := range map[string]string{
		"active": ``,
		"paused": `UPDATE campaigns SET awards_paused = true, redemptions_paused = true`,
	} {
		t.Run(name, func(t *testing.T) {
			reset(t, 10_000_000)
			if sql != "" {
				if _, err := pool.Exec(context.Background(), sql); err != nil {
					t.Fatal(err)
				}
			}
			for _, path := range []string{"/v1/campaign", "/v1/me/cashback"} {
				_, m := getBody(t, path, "user_new")
				noBudgetKey(t, m, path+" ")
			}
		})
	}
}

func TestCampaignBodyIndependentOfBudget(t *testing.T) {
	var bodies [2][]byte
	for i, budget := range []int64{2000, 9_000_000} {
		reset(t, budget)
		bodies[i], _ = getBody(t, "/v1/campaign", "user_new")
	}
	if string(bodies[0]) != string(bodies[1]) {
		t.Errorf("bodies differ:\n%s\n%s", bodies[0], bodies[1])
	}
}
