package admin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

func TestParse(t *testing.T) {
	ok := map[string]struct {
		args   []string
		sw     domain.Switch
		paused bool
		by     string
	}{
		"pause awards":        {[]string{"pause-awards", "--by", "owner"}, domain.SwitchAwards, true, "owner"},
		"resume awards":       {[]string{"resume-awards", "--by", "owner"}, domain.SwitchAwards, false, "owner"},
		"pause redemptions":   {[]string{"pause-redemptions", "--by", "owner"}, domain.SwitchRedemptions, true, "owner"},
		"resume redemptions":  {[]string{"resume-redemptions", "--by=riyan"}, domain.SwitchRedemptions, false, "riyan"},
		"operator is trimmed": {[]string{"pause-awards", "--by", "  owner "}, domain.SwitchAwards, true, "owner"},
	}
	for name, c := range ok {
		t.Run(name, func(t *testing.T) {
			got, err := parse(c.args)
			if err != nil {
				t.Fatal(err)
			}
			if got.sw != c.sw || got.paused != c.paused || got.operator != c.by {
				t.Errorf("parse = %+v, want %s %v %q", got, c.sw, c.paused, c.by)
			}
		})
	}
	bad := map[string][]string{
		"no args":             nil,
		"unknown command":     {"drop-awards", "--by", "owner"},
		"missing by":          {"pause-awards"},
		"empty by":            {"pause-awards", "--by", ""},
		"blank by":            {"pause-awards", "--by", "   "},
		"by without value":    {"pause-awards", "--by"},
		"extra argument":      {"pause-awards", "--by", "owner", "now"},
		"operator positional": {"pause-awards", "owner"},
		"unknown flag":        {"pause-awards", "--by", "owner", "--force"},
	}
	for name, args := range bad {
		t.Run("usage error: "+name, func(t *testing.T) {
			if _, err := parse(args); err == nil {
				t.Errorf("parse(%v) accepted", args)
			}
		})
	}
}

// A usage error exits 2 before any connection: the URL here would fail to
// connect, and stdout stays empty.
func TestCommandUsageExitsTwoBeforeConnecting(t *testing.T) {
	var out, errOut bytes.Buffer
	opts := Options{DatabaseURL: "postgres://nobody:secret@127.0.0.1:1/none", ConnectWait: time.Minute}
	start := time.Now()
	code := Command(context.Background(), []string{"pause-awards"}, opts, &out, &errOut)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty", out.String())
	}
	if !strings.Contains(errOut.String(), "--by") || strings.Contains(errOut.String(), "secret") {
		t.Errorf("stderr = %q", errOut.String())
	}
	if time.Since(start) > time.Second {
		t.Error("usage error waited for a connection")
	}
}

// A database that cannot be reached exits 1 with nothing on stdout, and the
// message carries neither the URL nor the password.
func TestCommandConnectFailureExitsOne(t *testing.T) {
	var out, errOut bytes.Buffer
	opts := Options{DatabaseURL: "postgres://nobody:s3cr3tpw@127.0.0.1:1/none", ConnectWait: 200 * time.Millisecond}
	code := Command(context.Background(), []string{"pause-awards", "--by", "owner"}, opts, &out, &errOut)
	if code != 1 {
		t.Errorf("exit = %d, want 1 (stderr %q)", code, errOut.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty", out.String())
	}
	if errOut.Len() == 0 {
		t.Error("stderr is empty, want a message")
	}
	for _, leak := range []string{"s3cr3tpw", "postgres://", "127.0.0.1"} {
		if strings.Contains(errOut.String(), leak) {
			t.Errorf("stderr %q leaks %q", errOut.String(), leak)
		}
	}
}

func TestFailureMessage(t *testing.T) {
	driver := errors.New(`ERROR: relation "campaigns" does not exist (SQLSTATE 42P01)`)
	for name, c := range map[string]struct {
		err  error
		want string
	}{
		"busy":            {fmt.Errorf("%w: lock", store.ErrBusy), "admin: busy, retry"},
		"unknown outcome": {fmt.Errorf("%w: commit", store.ErrUnknownOutcome), "admin: outcome unknown, check the switch before retrying"},
		"other":           {driver, "admin: the switch change failed"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := failureMessage(c.err); got != c.want {
				t.Errorf("message = %q, want %q", got, c.want)
			}
		})
	}
}
