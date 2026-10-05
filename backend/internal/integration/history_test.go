//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
)

// historyOf reads GET /v1/me/history for user, with an optional query.
func historyOf(t *testing.T, user, query string) domain.HistoryView {
	t.Helper()
	raw, _ := getBody(t, "/v1/me/history"+query, user)
	var v domain.HistoryView
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("history body: %v: %s", err, raw)
	}
	return v
}

func references(v domain.HistoryView) []string {
	out := make([]string, 0, len(v.Items))
	for _, it := range v.Items {
		out = append(out, it.Reference)
	}
	return out
}

func startClock(t *testing.T, rfc3339 string) {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		t.Fatal(err)
	}
	setClockFrom(t, ts)
}

// AC-47, AC-02: payments (Rp0 ones too) and redemptions in one list, newest
// first, each with only its own fields.
func TestHistoryItemsNewestFirstAC47AC02(t *testing.T) {
	reset(t, 10_000_000)
	startClock(t, "2026-10-03T07:00:00Z")
	if r := pay(t, "user_a", 100000); r.status != http.StatusCreated {
		t.Fatal(r.raw)
	}
	if r := redeem(t, "user_a", 2000); r.status != http.StatusCreated {
		t.Fatal(r.raw)
	}
	if r := pay(t, "user_a", 19999); r.status != http.StatusCreated || r.res.Cashback.Reason != domain.ReasonBelowMinimum {
		t.Fatal(r.raw)
	}
	if r := redeem(t, "user_a", 1000); r.status != http.StatusCreated {
		t.Fatal(r.raw)
	}

	raw, _ := getBody(t, "/v1/me/history", "user_a")
	var got struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	wantTime := regexp.MustCompile(`^2026-10-03T14:\d\d:\d\d\+07:00$`)
	for _, it := range got.Items {
		ts, _ := it["created_at"].(string)
		if !wantTime.MatchString(ts) {
			t.Errorf("created_at = %q, want a WIB time on 2026-10-03", ts)
		}
		delete(it, "created_at")
	}
	want := []map[string]any{
		{"type": "REDEMPTION", "id": 2.0, "reference": "RDM-20261003-000002", "amount": 1000.0,
			"status": "COMPLETED", "destination": "MAIN_ACCOUNT"},
		{"type": "PAYMENT", "id": 2.0, "reference": "PAY-20261003-000002", "amount": 19999.0,
			"status": "SUCCEEDED", "cashback": map[string]any{"awarded": 0.0, "reason": "BELOW_MINIMUM"}},
		{"type": "REDEMPTION", "id": 1.0, "reference": "RDM-20261003-000001", "amount": 2000.0,
			"status": "COMPLETED", "destination": "MAIN_ACCOUNT"},
		{"type": "PAYMENT", "id": 1.0, "reference": "PAY-20261003-000001", "amount": 100000.0,
			"status": "SUCCEEDED", "cashback": map[string]any{"awarded": 5000.0, "reason": "AWARDED"}},
	}
	if !reflect.DeepEqual(got.Items, want) {
		t.Errorf("items = %v\nwant    %v", got.Items, want)
	}
	assertReconciled(t)
}

// AC-47: default 20, maximum 50, the newest ones of both tables.
func TestHistoryLimitAC47(t *testing.T) {
	reset(t, 10_000_000)
	startClock(t, "2026-10-03T07:00:00Z")
	var newest []string // newest first
	for i := 1; i <= 30; i++ {
		if r := pay(t, "user_a", 20000); r.status != http.StatusCreated || r.res.Cashback.Awarded != 1000 {
			t.Fatal(r.raw)
		}
		if r := redeem(t, "user_a", 1000); r.status != http.StatusCreated {
			t.Fatal(r.raw)
		}
		newest = append([]string{
			domain.Reference(domain.RefRedemption, time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), int64(i)),
			domain.Reference(domain.RefPayment, time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), int64(i)),
		}, newest...)
	}
	for _, tc := range []struct {
		query string
		n     int
	}{{"", 20}, {"?limit=50", 50}, {"?limit=1", 1}} {
		if got := references(historyOf(t, "user_a", tc.query)); !reflect.DeepEqual(got, newest[:tc.n]) {
			t.Errorf("limit %q = %v\nwant %v", tc.query, got, newest[:tc.n])
		}
	}
	assertReconciled(t)
}

// Two rows at the same instant keep a stable order: the larger id first.
func TestHistorySameInstantLargerIDFirst(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	for i := 0; i < 5; i++ {
		if r := pay(t, "user_a", 20000); r.status != http.StatusCreated {
			t.Fatal(r.raw)
		}
	}
	var ids []int64
	for _, it := range historyOf(t, "user_a", "").Items {
		ids = append(ids, it.ID)
	}
	if want := []int64{5, 4, 3, 2, 1}; !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	assertReconciled(t)
}

// AC-16: a user with no rows gets an empty list, never null or a 404.
func TestHistoryNewUserAC16(t *testing.T) {
	reset(t, 10_000_000)
	raw, _ := getBody(t, "/v1/me/history", "user_new")
	if string(raw) != `{"items":[]}`+"\n" {
		t.Errorf("body = %q, want {\"items\":[]}", raw)
	}
	assertReconciled(t)
}

// AC-48: another user's payments and redemptions never show up.
func TestHistoryAndCashbackScopedToCallerAC48(t *testing.T) {
	reset(t, 10_000_000)
	mustSetNow(t, "2026-10-03T07:00:00Z")
	if r := pay(t, "user_a", 100000); r.status != http.StatusCreated {
		t.Fatal(r.raw)
	}
	if r := redeem(t, "user_a", 2000); r.status != http.StatusCreated {
		t.Fatal(r.raw)
	}
	if n := len(historyOf(t, "user_a", "").Items); n != 2 {
		t.Errorf("user_a items = %d, want 2", n)
	}
	raw, _ := getBody(t, "/v1/me/history", "user_b")
	if string(raw) != `{"items":[]}`+"\n" {
		t.Errorf("user_b history = %s, want none", raw)
	}
	_, m := getBody(t, "/v1/me/cashback", "user_b")
	today := m["today"].(map[string]any)
	if m["balance"] != 0.0 || today["earned"] != 0.0 {
		t.Errorf("user_b cashback = %v, want balance 0 earned 0", m)
	}
	assertReconciled(t)
}

// AC-49: no budget figure in any response, in every campaign state. The byte
// comparison of GET /campaign at budget 2000 and 9000000 is
// TestCampaignBodyIndependentOfBudget.
func TestNoBudgetFigureInAnyResponseAC49(t *testing.T) {
	for name, sql := range map[string]string{
		"active":          ``,
		"both paused":     `UPDATE campaigns SET awards_paused = true, redemptions_paused = true`,
		"partially spent": `UPDATE campaigns SET budget = spent + 2000`,
		"ended":           `UPDATE campaigns SET budget = spent`,
	} {
		t.Run(name, func(t *testing.T) {
			reset(t, 10_000_000)
			mustSetNow(t, "2026-10-03T07:00:00Z")
			fund(t, "user_a", 5000)
			if sql != "" {
				if _, err := pool.Exec(context.Background(), sql); err != nil {
					t.Fatal(err)
				}
			}
			bodies := map[string]string{
				"POST /payments":    pay(t, "user_a", 100000).raw,
				"POST /redemptions": redeem(t, "user_a", 1000).raw,
			}
			for _, path := range []string{"/v1/campaign", "/v1/me/cashback", "/v1/me/history"} {
				raw, _ := getBody(t, path, "user_a")
				bodies["GET "+path] = string(raw)
			}
			for what, raw := range bodies {
				var v any
				if err := json.Unmarshal([]byte(raw), &v); err != nil {
					t.Fatalf("%s body: %v: %s", what, err, raw)
				}
				noBudgetKey(t, v, what+" ")
			}
			if n := len(historyOf(t, "user_a", "").Items); n < 2 {
				t.Errorf("history has %d items, want the funding payment and the next payment", n)
			}
			assertReconciled(t)
		})
	}
}
