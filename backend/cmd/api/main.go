// Command api serves the HTTP API; "api healthcheck" probes a running server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/riyanpratamap/flash-cashback/backend/internal/boot"
	"github.com/riyanpratamap/flash-cashback/backend/internal/cache"
	"github.com/riyanpratamap/flash-cashback/backend/internal/config"
	"github.com/riyanpratamap/flash-cashback/backend/internal/httpapi"
	"github.com/riyanpratamap/flash-cashback/backend/internal/service"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

const (
	pgConnectWait = 30 * time.Second
	drainTimeout  = 8 * time.Second
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(healthcheckURL(cfg.HTTPAddr), 2*time.Second))
	}
	os.Exit(serve(cfg))
}

func serve(cfg config.Config) int {
	var level slog.Level
	_ = level.UnmarshalText([]byte(cfg.LogLevel))
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	pool, err := boot.Connect(ctx, cfg.DatabaseURL, int32(cfg.DBMaxConns), pgConnectWait)
	if err != nil {
		log.Error("start failed", "error", err.Error())
		return 1
	}
	defer pool.Close()
	if err := boot.Run(ctx, pool, cfg.CampaignBudget); err != nil {
		log.Error("start failed", "error", err.Error())
		return 1
	}
	// Redis being down is not fatal: the client connects lazily.
	rc := cache.NewClient(cfg)
	defer rc.Close()

	money := store.TxRunner{Pool: pool, LockTimeoutMS: cfg.LockTimeoutMS(), StatementTimeoutMS: cfg.StatementTimeoutMS()}
	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouter(httpapi.Deps{
			PingPostgres: func(ctx context.Context) error { return store.Ping(ctx, pool) },
			PingRedis:    func(ctx context.Context) error { return cache.Ping(ctx, rc) },
			Log:          log,
			Reads:        service.NewReads(pool),
			Payments:     service.NewPayments(money, log),
			Redemptions:  service.NewRedemptions(money, log),
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()
	log.Info("listening", "addr", cfg.HTTPAddr)

	select {
	case err := <-serveErr:
		log.Error("server failed", "error", err.Error())
		return 1
	case <-ctx.Done():
	}
	log.Info("shutting down")
	drainCtx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()
	if err := srv.Shutdown(drainCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("shutdown incomplete", "error", err.Error())
		return 1
	}
	return 0
}
