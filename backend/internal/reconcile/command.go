package reconcile

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/riyanpratamap/flash-cashback/backend/internal/boot"
)

// Exit codes of the command.
const (
	exitOK     = 0 // every check passed
	exitFailed = 1 // at least one check found a violation
	exitCannot = 2 // the checks could not run
)

// Command connects, runs the checks, and returns the process exit code. The
// JSON lines go to stdout; a failure to run goes to stderr without SQL or
// credentials.
func Command(ctx context.Context, databaseURL string, connectWait time.Duration, stdout, stderr io.Writer) int {
	pool, err := boot.Connect(ctx, databaseURL, 2, connectWait)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "reconcile:", err) // stderr is the last resort; nothing else to do on failure
		return exitCannot
	}
	defer pool.Close()
	ok, err := Run(ctx, pool, stdout)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err) // as above
		return exitCannot
	}
	if !ok {
		return exitFailed
	}
	return exitOK
}
