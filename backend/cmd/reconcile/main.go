// Command reconcile checks the money invariants and prints one JSON line per
// check. Exit 0: all pass. Exit 1: a check failed. Exit 2: it could not run.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/riyanpratamap/flash-cashback/backend/internal/config"
	"github.com/riyanpratamap/flash-cashback/backend/internal/reconcile"
)

const connectWait = 5 * time.Second

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "reconcile:", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	code := reconcile.Command(ctx, cfg.DatabaseURL, connectWait, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
