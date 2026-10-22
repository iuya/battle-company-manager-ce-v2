// Package postgres is the persistence adapter. Its methods satisfy the repository interfaces declared by the core.
package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

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
	return s, nil
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
