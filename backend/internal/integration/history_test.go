//go:build integration

package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/service"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
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
	if string(raw) != `{"items":[],"next_cursor":null}`+"\n" {
		t.Errorf("body = %q, want an empty list", raw)
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
	if string(raw) != `{"items":[],"next_cursor":null}`+"\n" {
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
				setCampaignSQL(t, sql)
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

// ac47aFixture is user_a's 45 items, numbered newest first as the AC does
// (item 1 is the newest). Item 21 (redemption) and item 20 (payment) share an
// instant and an id; item 40 (redemption) and item 39 (payment) share an
// instant and the payment's id is the lower. Every other item has its own
// second.
type ac47aFixture struct {
	items [46]struct { // index 1..45
		typ string
		id  int64
		at  time.Time
	}
	start time.Time
}

func (f *ac47aFixture) wantPage(from, to int) []string {
	var out []string
	for n := from; n <= to; n++ {
		out = append(out, fmt.Sprintf("%s-%d", f.items[n].typ, f.items[n].id))
	}
	return out
}

func pageKeys(v domain.HistoryView) []string {
	out := make([]string, 0, len(v.Items))
	for _, it := range v.Items {
		out = append(out, fmt.Sprintf("%s-%d", it.Type, it.ID))
	}
	return out
}

// buildAC47a creates the fixture through the services, oldest item first.
func buildAC47a(t *testing.T) *ac47aFixture {
	t.Helper()
	f := &ac47aFixture{start: time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)}
	step := func(n int, typ string) {
		// Items 21/20 and 40/39 share the instant of the older item of the pair.
		at := f.start.Add(time.Duration(46-n) * time.Second)
		if n == 20 {
			at = f.items[21].at
		}
		if n == 39 { // created before item 40, so item 40's own second
			at = f.start.Add(6 * time.Second)
		}
		setNow(t, at)
		var id int64
		switch typ {
		case domain.HistoryPayment:
			r := pay(t, "user_a", 20000)
			if r.status != http.StatusCreated {
				t.Fatalf("item %d: %s", n, r.raw)
			}
			id = r.res.Payment.ID
		default:
			r := redeem(t, "user_a", 1000)
			if r.status != http.StatusCreated {
				t.Fatalf("item %d: %s", n, r.raw)
			}
			id = r.res.Redemption.ID
		}
		f.items[n].typ, f.items[n].id, f.items[n].at = typ, id, at
	}
	for n := 45; n >= 41; n-- {
		step(n, domain.HistoryPayment)
	}
	// Pair 40/39: the payment is created first, so its id is the lower.
	execSQL(t, `SELECT setval('redemptions_id_seq', 100)`)
	step(39, domain.HistoryPayment)
	step(40, domain.HistoryRedemption)
	for n := 38; n >= 22; n-- {
		step(n, domain.HistoryPayment)
	}
	// Pair 21/20: the payment gets the redemption's id.
	step(21, domain.HistoryRedemption)
	execSQL(t, `SELECT setval('payments_id_seq', $1)`, f.items[21].id-1)
	step(20, domain.HistoryPayment)
	for n := 19; n >= 1; n-- {
		step(n, domain.HistoryPayment)
	}
	if f.items[20].id != f.items[21].id || f.items[39].id >= f.items[40].id {
		t.Fatalf("fixture ids: %+v %+v %+v %+v", f.items[20], f.items[21], f.items[39], f.items[40])
	}
	return f
}

func historyPage(t *testing.T, user string, limit int, c *domain.Cursor) (domain.HistoryView, *domain.Cursor) {
	t.Helper()
	v, err := service.NewReads(pool, nil).History(context.Background(), domain.UserID(user), limit, c)
	if err != nil {
		t.Fatal(err)
	}
	if v.NextCursor == nil {
		return v, nil
	}
	next, err := domain.DecodeCursor(*v.NextCursor)
	if err != nil {
		t.Fatalf("next_cursor %q: %v", *v.NextCursor, err)
	}
	return v, &next
}

// AC-47a: walking the pages by next_cursor gives every item once, in order,
// across two equal-time pairs that sit on the page boundaries.
func TestHistoryCursorWalkAC47a(t *testing.T) {
	reset(t, 10_000_000)
	f := buildAC47a(t)

	p1, c1 := historyPage(t, "user_a", 20, nil)
	if got, want := pageKeys(p1), f.wantPage(1, 20); !reflect.DeepEqual(got, want) {
		t.Fatalf("page 1 = %v\nwant     %v", got, want)
	}
	if c1 == nil || c1.Type != domain.CursorPayment || c1.ID != f.items[20].id || !c1.T.Equal(f.items[20].at) {
		t.Fatalf("page 1 cursor = %+v, want the payment of item 20 at %v", c1, f.items[20].at)
	}

	// A payment at a later instant than every item, after the first fetch.
	setNow(t, f.start.Add(time.Hour))
	if r := pay(t, "user_a", 20000); r.status != http.StatusCreated {
		t.Fatal(r.raw)
	}

	p2, c2 := historyPage(t, "user_a", 20, c1)
	if got, want := pageKeys(p2), f.wantPage(21, 40); !reflect.DeepEqual(got, want) {
		t.Fatalf("page 2 = %v\nwant     %v", got, want)
	}
	if c2 == nil || c2.Type != domain.CursorRedemption || c2.ID != f.items[40].id || !c2.T.Equal(f.items[40].at) {
		t.Fatalf("page 2 cursor = %+v, want the redemption of item 40 at %v", c2, f.items[40].at)
	}
	p3, c3 := historyPage(t, "user_a", 20, c2)
	if got, want := pageKeys(p3), f.wantPage(41, 45); !reflect.DeepEqual(got, want) {
		t.Fatalf("page 3 = %v\nwant     %v", got, want)
	}
	if c3 != nil || p3.NextCursor != nil {
		t.Errorf("last page next_cursor = %v, want nil", p3.NextCursor)
	}
	assertReconciled(t)
}

// AC-47: next_cursor is nil when exactly limit items remain and set when one
// more exists (45 items: limit 44, 45, 46).
func TestHistoryCursorLimitBoundaryAC47(t *testing.T) {
	reset(t, 10_000_000)
	f := buildAC47a(t)

	v, c := historyPage(t, "user_a", 44, nil)
	if len(v.Items) != 44 || c == nil || c.ID != f.items[44].id {
		t.Fatalf("limit 44: %d items, cursor %+v, want 44 items and the cursor of item 44", len(v.Items), c)
	}
	for _, limit := range []int{45, 46} {
		v, c := historyPage(t, "user_a", limit, nil)
		if len(v.Items) != 45 || c != nil || v.NextCursor != nil {
			t.Errorf("limit %d: %d items, next_cursor %v, want 45 items and nil", limit, len(v.Items), v.NextCursor)
		}
	}
	// From item 40's position exactly 5 items remain.
	_, c40 := historyPage(t, "user_a", 40, nil)
	if c40 == nil {
		t.Fatal("limit 40: next_cursor is nil, want the cursor of item 40")
	}
	v, c = historyPage(t, "user_a", 5, c40)
	if len(v.Items) != 5 || c != nil {
		t.Errorf("limit 5 with 5 left: %d items, cursor %+v, want 5 items and nil", len(v.Items), c)
	}
	v, c = historyPage(t, "user_a", 4, c40)
	if len(v.Items) != 4 || c == nil {
		t.Errorf("limit 4 with 5 left: %d items, cursor %+v, want 4 items and a cursor", len(v.Items), c)
	}
	assertReconciled(t)
}

// AC-47: a limit below 1 is an error, not a panic.
func TestHistoryRejectsNonPositiveLimit(t *testing.T) {
	for _, limit := range []int{0, -1} {
		if _, err := service.NewReads(pool, nil).History(context.Background(), "user_a", limit, nil); err == nil {
			t.Errorf("limit %d: error is nil", limit)
		}
	}
}

// AC-47a: user_b with user_a's cursor gets only user_b's rows older than that
// position, newer rows and user_a's rows never.
func TestHistoryCursorScopedToUserAC47a(t *testing.T) {
	reset(t, 10_000_000)
	f := buildAC47a(t)
	_, c1 := historyPage(t, "user_a", 20, nil)

	// user_b: two rows older than item 20 and one newer.
	setNow(t, f.items[30].at.Add(500*time.Millisecond))
	older1 := pay(t, "user_b", 20000)
	setNow(t, f.items[30].at.Add(700*time.Millisecond))
	older2 := redeem(t, "user_b", 1000)
	setNow(t, f.items[10].at.Add(500*time.Millisecond))
	newer := pay(t, "user_b", 20000)
	for _, r := range []int{older1.status, older2.status, newer.status} {
		if r != http.StatusCreated {
			t.Fatal(older1.raw, older2.raw, newer.raw)
		}
	}

	v, next := historyPage(t, "user_b", 20, c1)
	want := []string{
		fmt.Sprintf("REDEMPTION-%d", older2.res.Redemption.ID),
		fmt.Sprintf("PAYMENT-%d", older1.res.Payment.ID),
	}
	if got := pageKeys(v); !reflect.DeepEqual(got, want) {
		t.Errorf("user_b page = %v, want %v", got, want)
	}
	if next != nil {
		t.Errorf("user_b next = %+v, want nil", next)
	}
	assertReconciled(t)
}

// KP: each UNION branch reads its own user index in order and stops at its
// limit, with no sort below the branch LIMIT. enable_seqscan is off because
// 45 rows would otherwise plan as a sequential scan.
func TestHistoryCursorQueryUsesIndexesAC47a(t *testing.T) {
	reset(t, 10_000_000)
	buildAC47a(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	err = tx.QueryRow(ctx, `EXPLAIN (FORMAT JSON) `+store.HistoryCursorSQL,
		"user_a", 21, time.Date(2026, 10, 3, 7, 0, 25, 0, time.UTC), int64(100), int64(100)).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	var plans []struct {
		Plan planNode `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &plans); err != nil {
		t.Fatalf("plan: %v: %s", err, raw)
	}
	found := map[string]bool{}
	var walk func(n planNode, parent string)
	walk = func(n planNode, parent string) {
		if n.IndexName != "" {
			found[n.IndexName] = true
			if n.NodeType != "Index Scan" || parent != "Limit" {
				t.Errorf("%s: node %q under %q, want Index Scan directly under the branch Limit", n.IndexName, n.NodeType, parent)
			}
			// The row comparison must bound the index, not filter its rows.
			if !strings.Contains(n.IndexCond, "(created_at, id) <") {
				t.Errorf("%s: Index Cond %q lacks the (created_at, id) row comparison", n.IndexName, n.IndexCond)
			}
			if n.Filter != "" {
				t.Errorf("%s: Filter %q, want none", n.IndexName, n.Filter)
			}
		}
		for _, c := range n.Plans {
			walk(c, n.NodeType)
		}
	}
	walk(plans[0].Plan, "")
	for _, idx := range []string{"payments_user_newest", "redemptions_user_newest"} {
		if !found[idx] {
			t.Errorf("plan does not use %s:\n%s", idx, raw)
		}
	}
	assertReconciled(t)
}

type planNode struct {
	NodeType  string     `json:"Node Type"`
	IndexName string     `json:"Index Name"`
	IndexCond string     `json:"Index Cond"`
	Filter    string     `json:"Filter"`
	Plans     []planNode `json:"Plans"`
}

// AC-47a over HTTP: following next_cursor from the query string walks the same
// 20/20/5 pages as the service walk, and the last page's next_cursor is null.
func TestHistoryCursorHTTPWalkAC47a(t *testing.T) {
	reset(t, 10_000_000)
	f := buildAC47a(t)

	var got []string
	query, pages := "", 0
	for {
		v := historyOf(t, "user_a", query)
		pages++
		if pages > 3 {
			t.Fatalf("more than 3 pages, last query %q", query)
		}
		got = append(got, pageKeys(v)...)
		if v.NextCursor == nil {
			break
		}
		query = "?limit=20&cursor=" + url.QueryEscape(*v.NextCursor)
	}
	if want := f.wantPage(1, 45); !reflect.DeepEqual(got, want) || pages != 3 {
		t.Fatalf("pages %d, items\n%v\nwant\n%v", pages, got, want)
	}
	assertReconciled(t)
}

// AC-47a: an empty, undecodable, or wrong-version cursor is 400 over HTTP.
func TestHistoryMalformedCursorHTTPAC47a(t *testing.T) {
	reset(t, 10_000_000)
	v9 := base64.RawURLEncoding.EncodeToString([]byte("v9|x"))
	for _, q := range []string{"?cursor=", "?cursor=abc", "?cursor=!!!", "?cursor=" + v9} {
		req := httptest.NewRequest(http.MethodGet, "/v1/me/history"+q, nil)
		req.Header.Set("X-User-ID", "user_a")
		rec := httptest.NewRecorder()
		readsRouter().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"MALFORMED_REQUEST"`) {
			t.Errorf("%s = %d: %s", q, rec.Code, rec.Body)
		}
	}
}
