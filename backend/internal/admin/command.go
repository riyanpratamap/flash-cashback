// Package admin is the operator command line: the four switch commands
// (tech-spec §4.3) and demo-reset (§9).
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/riyanpratamap/flash-cashback/backend/internal/boot"
	"github.com/riyanpratamap/flash-cashback/backend/internal/cache"
	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/service"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// Exit codes of the command.
const (
	exitOK     = 0
	exitFailed = 1 // including a lock timeout: busy, retry
	exitUsage  = 2
)

const (
	defaultLockTimeoutMS = 5000
	// The statement timeout and the transaction cap sit above the lock
	// timeout, so a lock wait ends as a lock timeout first.
	statementTimeoutMS = 10_000
	txCap              = 10 * time.Second
)

// Options are the command's settings. LockTimeoutMS zero means 5 s (§4.3);
// tests shorten it.
type Options struct {
	DatabaseURL   string
	LockTimeoutMS int64
	ConnectWait   time.Duration
	// Cache deletes the campaign key after a switch commit and every key in
	// demo-reset.
	Cache *cache.Invalidator
	// Demo allows demo-reset (FC_DEMO=1).
	Demo bool
}

// request is a parsed, valid command line.
type request struct {
	sw       domain.Switch
	paused   bool
	operator string
	demo     bool // demo-reset: no switch, no operator
}

// parse reads "<command> --by <operator>" or "demo-reset". Anything else is
// an error.
func parse(args []string) (request, error) {
	if len(args) == 0 {
		return request{}, errors.New("missing command")
	}
	if args[0] == demoCommand {
		if len(args) > 1 {
			return request{}, errors.New("demo-reset takes no arguments")
		}
		return request{demo: true}, nil
	}
	sw, paused, err := domain.ParseSwitchCommand(args[0])
	if err != nil {
		return request{}, err
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	by := fs.String("by", "", "operator name")
	if err := fs.Parse(args[1:]); err != nil {
		return request{}, errors.New("bad flags")
	}
	if fs.NArg() > 0 {
		return request{}, errors.New("unexpected arguments")
	}
	operator := strings.TrimSpace(*by)
	if operator == "" {
		return request{}, errors.New("--by <operator> is required")
	}
	return request{sw: sw, paused: paused, operator: operator}, nil
}

// actionLine is the one JSON line a command prints after its commit.
type actionLine struct {
	Event    string `json:"event"`
	Switch   string `json:"switch"`
	Action   string `json:"action"`
	Operator string `json:"operator"`
	Old      bool   `json:"old"`
	New      bool   `json:"new"`
	Changed  bool   `json:"changed"`
	At       string `json:"at"`
}

const (
	demoCommand = "demo-reset"
	usage       = "usage: admin pause-awards|resume-awards|pause-redemptions|resume-redemptions --by <operator> | admin demo-reset"
)

// failureMessage is the fixed stderr text for a failed Set. The driver text
// may carry SQL, so it is never passed through.
func failureMessage(err error) string {
	switch {
	case errors.Is(err, store.ErrBusy):
		return "admin: busy, retry"
	case errors.Is(err, store.ErrUnknownOutcome):
		return "admin: outcome unknown, check the switch before retrying"
	default:
		return "admin: the switch change failed"
	}
}

// Command runs one switch command and returns the process exit code. A usage
// error returns 2 before any connection and prints nothing on stdout. The
// action line goes to stdout only after the transaction has committed;
// failures go to stderr without SQL or credentials.
func Command(ctx context.Context, args []string, opts Options, stdout, stderr io.Writer) int {
	req, err := parse(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "admin: %v\n%s\n", err, usage) // stderr is the last resort
		return exitUsage
	}
	if req.demo && !opts.Demo {
		_, _ = fmt.Fprintln(stderr, "admin: demo-reset refuses unless FC_DEMO=1") // stderr is the last resort
		return exitUsage
	}
	lockMS := opts.LockTimeoutMS
	if lockMS == 0 {
		lockMS = defaultLockTimeoutMS
	}
	pool, err := boot.Connect(ctx, opts.DatabaseURL, 2, opts.ConnectWait)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "admin:", err) // as above
		return exitFailed
	}
	defer pool.Close()

	tx := store.TxRunner{Pool: pool, LockTimeoutMS: lockMS, StatementTimeoutMS: statementTimeoutMS, Cap: txCap}
	if req.demo {
		return demoReset(ctx, tx, opts, stdout, stderr)
	}
	sws := service.NewSwitches(tx)
	ch, err := sws.Set(ctx, req.sw, req.paused, req.operator)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, failureMessage(err)) // as above
		return exitFailed
	}
	line, err := json.Marshal(actionLine{
		Event: "operator_action", Switch: string(ch.Switch), Action: ch.Action, Operator: ch.Operator,
		Old: ch.Old, New: ch.New, Changed: ch.Changed, At: ch.At,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "admin: the change committed but its line could not be built") // as above
		return exitFailed
	}
	if _, err := fmt.Fprintf(stdout, "%s\n", line); err != nil {
		return exitFailed
	}
	// After the line, whether or not the value changed (§4.3). A Redis
	// failure is logged by the invalidator and does not fail the command.
	opts.Cache.Delete(ctx, cache.CampaignKey)
	return exitOK
}

// demoReset runs demo-reset (AC-57) and prints its one line. The services
// log nothing here: the operator's terminal gets the line and any warning.
func demoReset(ctx context.Context, tx store.TxRunner, opts Options, stdout, stderr io.Writer) int {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	demo := service.NewDemo(tx, service.NewPayments(tx, opts.Cache, quiet), service.NewRedemptions(tx, opts.Cache, quiet), opts.Cache)
	res, err := demo.Reset(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, demoFailureMessage(err)) // stderr is the last resort
		return exitFailed
	}
	if res.CacheErr != nil {
		_, _ = fmt.Fprintln(stderr, "admin: warning: the cache keys were not deleted; they expire on their own") // as above
	}
	line, err := json.Marshal(struct {
		Event string `json:"event"`
		At    string `json:"at"`
	}{"demo_reset", res.At})
	if err != nil {
		return exitFailed
	}
	if _, err := fmt.Fprintf(stdout, "%s\n", line); err != nil {
		return exitFailed
	}
	return exitOK
}

// demoFailureMessage is fixed text: a driver error may carry SQL.
func demoFailureMessage(err error) string {
	switch {
	case errors.Is(err, service.ErrDemoStateNotReached):
		return "admin: demo state not reached (is the budget below 97000?); run demo-reset again"
	case errors.Is(err, store.ErrBusy):
		return "admin: busy, retry"
	default:
		return "admin: demo-reset failed; run demo-reset again"
	}
}
