// Package postgres is the persistence adapter. Its methods satisfy the repository interfaces declared by the core.
package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	pool *pgxpool.Pool
	url  string // kept for connections outside the pool (the live feed)

	// hashes remembers each campaign's rules files and settings (campaignStore.Hashes): they are set when the campaign is created and
	// never change, and campaigns are never deleted, so the answer is read once per campaign and process.
	hashes     sync.Map // campaign id -> hashesEntry
	hashesSize atomic.Int64
}

// Options tune Open.
type Options struct {
	// SkipMigrations connects without applying migrations and fails when some are pending: the schema is then changed only by the
	// explicit "server migrate" command (docs/architecture.md, "Deploying").
	SkipMigrations bool
	// Tracer, when set, sees every query (pgx.QueryTracer, and pgx.BatchTracer for batches): the tests count the queries a page makes.
	Tracer pgx.QueryTracer
}

// Open connects to the database and applies any pending migrations.
func Open(ctx context.Context, url string) (*Store, error) { return OpenWith(ctx, url, Options{}) }

// OpenWith is Open with options.
func OpenWith(ctx context.Context, url string, opt Options) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parsing the database url: %w", err)
	}
	cfg.ConnConfig.Tracer = opt.Tracer
	cfg.MaxConns = poolSize(url, cfg.MaxConns)
	var guarded atomic.Bool
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error { return guard(ctx, c, &guarded) }
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}
	s := &Store{pool: pool, url: url}
	if err := s.migrate(ctx, opt.SkipMigrations); err != nil {
		pool.Close()
		return nil, err
	}
	// The migrations may run longer than the limits below; only connections opened after this have them.
	guarded.Store(true)
	pool.Reset()
	return s, nil
}

// minPoolConns is the pool size when DATABASE_URL does not set pool_max_conns and the machine has fewer CPUs: pgxpool's own default is the
// number of CPUs (at least 4), which on a small host is fewer connections than the cards of one match page need side by side. Measured
// with 160 match pages requested at once (BenchmarkPages/match-parallel): 4 connections 2.8 ms per page, 10 connections 2.0 ms, 20 connections 1.8 ms.
const minPoolConns = 10

// poolSize is the connection limit: the one in DATABASE_URL (pool_max_conns=N) when it has one, else the larger of pgxpool's default and minPoolConns.
func poolSize(url string, def int32) int32 {
	if strings.Contains(url, "pool_max_conns") {
		return def
	}
	return max(def, minPoolConns)
}

// Limits on what one request can hold in the database (the values are a judgement call): a statement (a lock wait included) that runs longer is
// cancelled, and a transaction left open and idle is ended and its locks released, so a stuck request cannot wedge the match row
// or exhaust the pool. Settings given in DATABASE_URL (statement_timeout=...) win.
const (
	statementTimeout       = "60s"
	idleInTransactionLimit = "60s"
)

func guard(ctx context.Context, c *pgx.Conn, on *atomic.Bool) error {
	if !on.Load() {
		return nil
	}
	rp := c.Config().RuntimeParams
	for name, v := range map[string]string{"statement_timeout": statementTimeout, "idle_in_transaction_session_timeout": idleInTransactionLimit} {
		if _, set := rp[name]; set {
			continue
		}
		if _, err := c.Exec(ctx, "SELECT set_config($1, $2, false)", name, v); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Close() { s.pool.Close() }

// Ping reports whether the database is reachable; used by the health check.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// migrate applies the pending migrations under a Postgres advisory lock (goose's session locker), so two instances that start
// together run each migration once: the second waits for the first and then finds nothing to do. With check set it only
// verifies that nothing is pending.
func (s *Store) migrate(ctx context.Context, check bool) error {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	db := stdlib.OpenDBFromPool(s.pool)
	defer db.Close()
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, sub, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("preparing migrations: %w", err)
	}
	if check {
		pending, err := p.HasPending(ctx)
		if err != nil {
			return fmt.Errorf("checking migrations: %w", err)
		}
		if pending {
			return fmt.Errorf("the database has pending migrations: run \"server migrate\" first")
		}
		return nil
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	return nil
}
