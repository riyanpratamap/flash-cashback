package httpapi

import (
	"fmt"
	"strings"
	"testing"

	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// AC-27, AC-55: store outcomes map to one status and code each, and the
// envelope never carries driver text.
func TestStoreErrorMapping(t *testing.T) {
	deadlock := fmt.Errorf("%w: %w", store.ErrBusy, store.ErrDeadlock)
	cases := []struct {
		name      string
		err       error
		status    int
		code      string
		wantLog   string // substring that must be in the log, "" for none
		wantLevel string
	}{
		{"busy", store.ErrBusy, 503, "SERVICE_BUSY", "", ""},
		{"deadlock", deadlock, 503, "SERVICE_BUSY", "deadlock", "ERROR"},
		{"invariant", store.ErrInvariant, 500, "INTERNAL_ERROR", "invariant_violation", "ERROR"},
		{"unknown outcome", fmt.Errorf("%w: commit", store.ErrUnknownOutcome), 500, "INTERNAL_ERROR", "", "ERROR"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig()
			r.fake.err = c.err
			rec := r.do("POST", "/v1/payments", reqOpt{user: str("user_a"), key: str(goodKey), body: `{"amount":20000}`})
			r.wantError(t, rec, c.status, c.code)
			if c.code == "SERVICE_BUSY" {
				want := "A lock wait timed out; nothing was committed; retry with the same key"
				if got := decodeEnvelope(t, rec).Error.Message; got != want {
					t.Errorf("message = %q, want %q", got, want)
				}
			}
			logs := r.logs.String()
			if c.wantLog != "" && !strings.Contains(logs, c.wantLog) {
				t.Errorf("log lacks %q: %s", c.wantLog, logs)
			}
			if c.wantLevel != "" && !strings.Contains(logs, `"level":"`+c.wantLevel+`"`) {
				t.Errorf("log lacks level %s: %s", c.wantLevel, logs)
			}
			if c.wantLevel == "" && logs != "" && strings.Contains(logs, `"level":"ERROR"`) {
				t.Errorf("plain busy must not log at error: %s", logs)
			}
		})
	}
}
