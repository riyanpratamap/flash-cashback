package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
)

type fakeReads struct {
	campaignCalls int
	cashbackUsers []domain.UserID
	err           error
}

func (f *fakeReads) Campaign(context.Context) (domain.CampaignView, error) {
	f.campaignCalls++
	return domain.CampaignView{
		ID: "flash-cashback", Name: "Flash Cashback",
		Status: domain.CampaignActive, RedemptionStatus: domain.RedemptionAvailable,
		Rules: domain.RulesView{RateBps: 500, MinPayment: 20000, DailyCap: 50000, Timezone: "Asia/Jakarta"},
	}, f.err
}

func (f *fakeReads) Cashback(_ context.Context, u domain.UserID) (domain.CashbackView, error) {
	f.cashbackUsers = append(f.cashbackUsers, u)
	return domain.CashbackView{Balance: 15000, Today: domain.TodayView{
		Date: "2026-10-03", Earned: 47000, Remaining: 3000, ResetsAt: "2026-10-04T00:00:00+07:00",
	}}, f.err
}

func (f *fakeReads) calls() int { return f.campaignCalls + len(f.cashbackUsers) }

func readsRig(f *fakeReads) http.Handler {
	return NewRouter(Deps{PingPostgres: ok, PingRedis: ok, Reads: f})
}

func get(h http.Handler, path string, user *string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if user != nil {
		req.Header.Set("X-User-ID", *user)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestReadsUserValidatedBeforeService(t *testing.T) {
	for _, path := range []string{"/v1/campaign", "/v1/me/cashback"} {
		for name, tc := range map[string]struct {
			user *string
			code string
		}{
			"missing": {nil, "MISSING_USER"},
			"invalid": {str("BAD USER"), "INVALID_USER"},
		} {
			t.Run(path+"/"+name, func(t *testing.T) {
				f := &fakeReads{}
				rec := get(readsRig(f), path, tc.user)
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
				}
				if e := decodeEnvelope(t, rec); e.Error.Code != tc.code {
					t.Fatalf("code = %q, want %q", e.Error.Code, tc.code)
				}
				if f.calls() != 0 {
					t.Fatalf("service called %d times", f.calls())
				}
			})
		}
	}
}

func keysOf(t *testing.T, raw []byte) []string {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	var out []string
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		m, ok := v.(map[string]any)
		if !ok {
			return
		}
		for k, c := range m {
			out = append(out, prefix+k)
			walk(prefix+k+".", c)
		}
	}
	walk("", v)
	sort.Strings(out)
	return out
}

func TestReadsSuccessShape(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)budget|spent`)
	for path, want := range map[string]string{
		"/v1/campaign":    "id name redemption_status rules rules.daily_cap rules.min_payment rules.rate_bps rules.timezone status",
		"/v1/me/cashback": "balance today today.date today.earned today.remaining today.resets_at",
	} {
		t.Run(path, func(t *testing.T) {
			f := &fakeReads{}
			rec := get(readsRig(f), path, str("user_a"))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d (body %s)", rec.Code, rec.Body)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q", ct)
			}
			keys := keysOf(t, rec.Body.Bytes())
			if got := strings.Join(keys, " "); got != want {
				t.Errorf("keys = %q, want %q", got, want)
			}
			for _, k := range keys {
				if forbidden.MatchString(k) {
					t.Errorf("forbidden key %q", k)
				}
			}
		})
	}
}

func TestCashbackPassesUser(t *testing.T) {
	f := &fakeReads{}
	get(readsRig(f), "/v1/me/cashback", str("user_a"))
	if len(f.cashbackUsers) != 1 || f.cashbackUsers[0] != "user_a" {
		t.Fatalf("users = %v", f.cashbackUsers)
	}
}

func TestReadsServiceErrorIs500(t *testing.T) {
	for _, path := range []string{"/v1/campaign", "/v1/me/cashback"} {
		t.Run(path, func(t *testing.T) {
			f := &fakeReads{err: errors.New("boom SELECT secret")}
			rec := get(readsRig(f), path, str("user_a"))
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d", rec.Code)
			}
			if strings.Contains(rec.Body.String(), "secret") {
				t.Errorf("body leaks error text: %s", rec.Body)
			}
		})
	}
}
