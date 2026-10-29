package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // the zones users choose exist even in a minimal container

	"go-simpler.org/env"

	"github.com/iuya/battle-company-manager-ce-v2/internal/perftrace"
	"github.com/iuya/battle-company-manager-ce-v2/internal/postgres"
	"github.com/iuya/battle-company-manager-ce-v2/internal/web"
)

type config struct {
	Port string `env:"PORT" default:"8080"`
	// ListenHost is the address the server listens on; empty is every interface (what a container needs). make run, make dev and the
	// browser tests use 127.0.0.1: a server that only listens on loopback raises no firewall prompt (Windows asks for every new build).
	ListenHost  string `env:"LISTEN_HOST"`
	DatabaseURL string `env:"DATABASE_URL,required"`
	// MigrateOnStart applies the pending migrations when the server starts (the default). Turn it off where the schema is changed
	// only by "server migrate" (a deploy step): the server then refuses to start on a database that is behind.
	MigrateOnStart bool `env:"MIGRATE_ON_START" default:"true"`
	// MaxCachedRuleSets caps the rule sets (hash triples) kept parsed in memory, least recently used dropped first; a set is a few MB.
	MaxCachedRuleSets int `env:"MAX_CACHED_RULESETS" default:"16"`
	// RulesBuildTimeout is the deadline of building a parsed rule set from its three files; a parser stuck on a hostile upload ends with
	// an error (not cached) instead of running for ever.
	RulesBuildTimeout time.Duration `env:"RULES_BUILD_TIMEOUT" default:"60s"`
	// PerfTrace adds a Server-Timing header (time, database statements) to every answer, for tests/e2e/perf.mjs. Development only.
	PerfTrace bool `env:"PERF_TRACE" default:"false"`
	Web       web.Config
}

// validate rejects settings the server cannot run with; the web settings are checked by web.Config.Resolve.
func (c config) validate() error {
	if c.MaxCachedRuleSets < 1 {
		return fmt.Errorf("MAX_CACHED_RULESETS %d: want at least 1", c.MaxCachedRuleSets)
	}
	if c.RulesBuildTimeout < time.Second {
		return fmt.Errorf("RULES_BUILD_TIMEOUT %s: want at least 1s", c.RulesBuildTimeout)
	}
	return nil
}

func main() {
	run := run
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "user":
			run = func() error { return runUser(os.Args[2:]) }
		case "audit":
			run = func() error { return runAudit(os.Args[2:]) }
		case "migrate":
			run = func() error { return runMigrate() }
		case "healthcheck":
			run = runHealthcheck
		case "seed":
			run = func() error { return runSeed(os.Args[2:]) }
		}
	}
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	var cfg config
	if err := env.Load(&cfg, nil); err != nil {
		return err
	}
	if err := cfg.validate(); err != nil {
		return err
	}
	rulesCacheMax = cfg.MaxCachedRuleSets
	rulesBuildTimeout = cfg.RulesBuildTimeout
	if err := cfg.Web.Resolve(); err != nil {
		return err
	}
	for _, w := range cfg.Web.Warnings() {
		slog.Warn(w)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := postgres.Options{SkipMigrations: !cfg.MigrateOnStart}
	var handlerWrap = func(h http.Handler) http.Handler { return h }
	if cfg.PerfTrace {
		opts.Tracer = perftrace.Tracer{}
		handlerWrap = perftrace.Middleware
	}
	store, err := postgres.OpenWith(ctx, cfg.DatabaseURL, opts)
	if err != nil {
		return err
	}
	defer store.Close()

	b, err := newBackend(ctx, store)
	if err != nil {
		return err
	}
	srv, err := web.NewServer(cfg.Web, b.campaigns, b.rulesets, b.glossaries, b.auth)
	if err != nil {
		return err
	}
	srv.SetReadiness(store.Ping)
	srv.SetConcepts(b.concepts)

	httpSrv := &http.Server{Addr: net.JoinHostPort(cfg.ListenHost, cfg.Port), Handler: handlerWrap(srv.Handler()),
		// Slow-client protection. The live stream (SSE) lifts the read deadline and sets a write deadline per write, so it is not cut.
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 120 * time.Second,
		MaxHeaderBytes: 64 << 10}
	httpSrv.RegisterOnShutdown(b.liveHub.Close) // the live streams never end by themselves: Shutdown would wait for them
	go func() {
		if err := b.liveHub.Start(ctx); err != nil {
			slog.Error("live feed stopped", "err", err)
		}
	}()
	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.ListenAndServe() }()
	slog.Info("listening", "addr", httpSrv.Addr)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

// runMigrate applies the pending migrations and exits: the explicit step of a deploy that runs the server with MIGRATE_ON_START=false.
func runMigrate() error {
	var cfg struct {
		DatabaseURL string `env:"DATABASE_URL,required"`
	}
	if err := env.Load(&cfg, nil); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	store.Close()
	slog.Info("migrations applied; the database is current")
	return nil
}

// runHealthcheck is the container's HEALTHCHECK (the image has no curl): it asks the running server for /readyz on its own port.
func runHealthcheck() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/readyz", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("readyz answered %d", resp.StatusCode)
	}
	return nil
}
