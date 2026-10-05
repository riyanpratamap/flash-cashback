// Command admin runs the operator switch commands: pause-awards,
// resume-awards, pause-redemptions, resume-redemptions, each with --by.
// Exit 0: done. Exit 1: failed (including "busy, retry"). Exit 2: usage.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/riyanpratamap/flash-cashback/backend/internal/admin"
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
	code := admin.Command(ctx, os.Args[1:], admin.Options{DatabaseURL: cfg.DatabaseURL, ConnectWait: connectWait}, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
