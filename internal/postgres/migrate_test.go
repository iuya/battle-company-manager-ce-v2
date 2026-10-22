package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// scratchDatabase creates an empty database next to the test one and returns its URL; it is dropped with the test.
func scratchDatabase(t *testing.T) string {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("bcm_scratch_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(ctx, base)
		if err != nil {
			return
		}
		defer c.Close(ctx)
		_, _ = c.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}

// Two instances starting together on an empty database both succeed: the advisory lock makes one wait for the other.
func TestConcurrentMigrations(t *testing.T) {
	dsn := scratchDatabase(t)
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := Open(context.Background(), dsn)
			if err == nil {
				s.Close()
			}
			errs[i] = err
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("instance %d: %v", i, err)
		}
	}
}

// With SkipMigrations the server refuses a database that is behind and accepts one that is current.
func TestSkipMigrationsChecksPending(t *testing.T) {
	dsn := scratchDatabase(t)
	ctx := context.Background()
	if _, err := OpenWith(ctx, dsn, Options{SkipMigrations: true}); err == nil || !strings.Contains(err.Error(), "pending migrations") {
		t.Fatalf("empty database: got %v, want the pending migrations error", err)
	}
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = OpenWith(ctx, dsn, Options{SkipMigrations: true})
	if err != nil {
		t.Fatalf("migrated database: %v", err)
	}
	s.Close()
}

// Every migration documents how to undo itself (the only rollback besides a restore, docs/operations.md).
func TestMigrationsHaveDown(t *testing.T) {
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		raw, err := migrations.ReadFile("migrations/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "+goose Down") {
			t.Errorf("%s has no +goose Down section", e.Name())
		}
	}
}
