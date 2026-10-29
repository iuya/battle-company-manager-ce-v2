package main

import (
	"context"
	"os"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/postgres"
)

// TestSeedRunsTwice builds the real backend over an empty scratch database and seeds it: the second run changes nothing.
func TestSeedRunsTwice(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	url = freshDatabase(ctx, t, url) // empty, whatever the other tests left in the test database
	store, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	b, err := newBackend(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := seed(ctx, b, "demo-password"); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
}
