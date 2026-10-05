// Command admin runs the operator switch commands: pause-awards,
// resume-awards, pause-redemptions, resume-redemptions, each with --by, and
// demo-reset (only with FC_DEMO=1).
// Exit 0: done. Exit 1: failed (including "busy, retry"). Exit 2: usage.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/riyanpratamap/flash-cashback/backend/internal/admin"
	"github.com/riyanpratamap/flash-cashback/backend/internal/cache"
	"github.com/riyanpratamap/flash-cashback/backend/internal/config"
)

const connectWait = 5 * time.Second

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "admin:", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	// The client connects lazily, so a usage error still never connects.
	rc := cache.NewClient(cfg)
	opts := admin.Options{
		DatabaseURL: cfg.DatabaseURL, ConnectWait: connectWait, Demo: cfg.Demo,
		Cache: cache.NewInvalidator(rc, cfg.RedisTimeout, slog.Default()),
	}
	code := admin.Command(ctx, os.Args[1:], opts, os.Stdout, os.Stderr)
	_ = rc.Close() // nothing to do on a failed close at exit
	stop()
	os.Exit(code)
}
