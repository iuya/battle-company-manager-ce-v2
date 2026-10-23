package postgres

import (
	"context"
	"testing"
)

// Every pooled connection carries the statement and idle-in-transaction limits (a stuck request cannot hold locks for ever),
// and a setting given in the URL wins.
func TestPoolConnectionsCarryTheTimeouts(t *testing.T) {
	ctx := context.Background()
	dsn := scratchDatabase(t)
	for _, tc := range []struct{ name, url, want string }{
		{"default", dsn, "1min"},
		{"url wins", dsn + "&statement_timeout=5000", "5s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Open(ctx, tc.url)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			var got, idle string
			if err := s.pool.QueryRow(ctx, "SHOW statement_timeout").Scan(&got); err != nil {
				t.Fatal(err)
			}
			if err := s.pool.QueryRow(ctx, "SHOW idle_in_transaction_session_timeout").Scan(&idle); err != nil {
				t.Fatal(err)
			}
			if got != tc.want || idle != "1min" {
				t.Errorf("statement_timeout %q (want %q), idle_in_transaction_session_timeout %q", got, tc.want, idle)
			}
		})
	}
}
