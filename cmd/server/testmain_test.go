package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	neturl "net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/iuya/battle-company-manager-ce-v2/internal/postgres"
)

// TestMain runs the package from the repository root (definitions/ is read relative to it) and drops the template database at the end.
func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	worldTemplate.drop()
	os.Exit(code)
}

// worldTemplate is the demo world seeded once per test run into a database of its own. The tests that need a seeded world (the two fuzz
// tests) get a copy of it (CREATE DATABASE ... TEMPLATE is a file copy, under a second) instead of seeding their own for about 50 seconds,
// and so can run side by side. TestSeedWorld stays the one test that seeds from an empty database: it is the test of the seed.
var worldTemplate templateDB

type templateDB struct {
	once  sync.Once
	mu    sync.Mutex // copies of the template are made one at a time
	name  string
	base  string
	err   error
	count int
}

// databaseURL is the URL of another database on the server of url.
func databaseURL(url, name string) (string, error) {
	u, err := neturl.Parse(url)
	if err != nil {
		return "", err
	}
	u.Path = "/" + name
	return u.String(), nil
}

// seededDatabase returns the URL of a private copy of the seeded demo world; the copy is dropped when the test ends.
func seededDatabase(ctx context.Context, t *testing.T, base string) string {
	t.Helper()
	worldTemplate.once.Do(func() { worldTemplate.err = buildWorldTemplate(ctx, base) })
	if worldTemplate.err != nil {
		t.Fatalf("seeding the template database: %v", worldTemplate.err)
	}
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	worldTemplate.mu.Lock()
	worldTemplate.count++
	name := fmt.Sprintf("bcm_world_copy_%d_%d_%d", os.Getpid(), worldTemplate.count, time.Now().UnixNano()%1000000)
	_, err = admin.Exec(ctx, "CREATE DATABASE "+name+" TEMPLATE "+worldTemplate.name)
	worldTemplate.mu.Unlock()
	if err != nil {
		admin.Close(ctx)
		t.Fatalf("copying the template database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		admin.Close(ctx)
	})
	dsn, err := databaseURL(base, name)
	if err != nil {
		t.Fatal(err)
	}
	return dsn
}

func buildWorldTemplate(ctx context.Context, base string) error {
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		return err
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("bcm_world_template_%d_%d", os.Getpid(), time.Now().UnixNano()%1000000)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		return err
	}
	worldTemplate.name, worldTemplate.base = name, base
	dsn, err := databaseURL(base, name)
	if err != nil {
		return err
	}
	store, err := postgres.Open(ctx, dsn)
	if err != nil {
		return err
	}
	dice := &demoDice{}
	b, err := newBackendWithDice(ctx, store, dice)
	if err != nil {
		store.Close()
		return err
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		store.Close()
		return err
	}
	exec := func(ctx context.Context, q string, args ...any) error {
		_, err := conn.Exec(ctx, q, args...)
		return err
	}
	var out bytes.Buffer
	err = seedWorld(ctx, b, dice, "demo-password", &out, exec)
	conn.Close(ctx)
	store.Close()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, out.String())
	}
	// a database is copied only while nobody is connected to it
	_, err = admin.Exec(ctx, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()", name)
	return err
}

// drop removes the template at the end of the run.
func (w *templateDB) drop() {
	if w.name == "" {
		return
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, w.base)
	if err != nil {
		return
	}
	defer admin.Close(ctx)
	_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+w.name+" WITH (FORCE)")
}

// logCapture collects what the server logs while tests that read the log (a recovered panic is logged, not answered) run; tests that run
// side by side share it, so a panic in any of them fails all of them.
var logCapture struct {
	mu   sync.Mutex
	n    int
	buf  bytes.Buffer
	prev *slog.Logger
}

type captureWriter struct{}

func (captureWriter) Write(p []byte) (int, error) {
	logCapture.mu.Lock()
	defer logCapture.mu.Unlock()
	return logCapture.buf.Write(p)
}

// captureLogs makes the default logger write to the shared buffer until the test ends and returns a function that reads it.
func captureLogs(t *testing.T) func() string {
	t.Helper()
	logCapture.mu.Lock()
	if logCapture.n == 0 {
		logCapture.buf.Reset()
		logCapture.prev = slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(captureWriter{}, nil)))
	}
	logCapture.n++
	logCapture.mu.Unlock()
	t.Cleanup(func() {
		logCapture.mu.Lock()
		defer logCapture.mu.Unlock()
		if logCapture.n--; logCapture.n == 0 {
			slog.SetDefault(logCapture.prev)
		}
	})
	return func() string {
		logCapture.mu.Lock()
		defer logCapture.mu.Unlock()
		return logCapture.buf.String()
	}
}
