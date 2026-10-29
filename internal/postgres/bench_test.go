package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// BenchmarkDecodeUnits times decodeUnits on the units JSON of every company of a loaded demo world, and BenchmarkReadRoster the whole
// roster read of the biggest campaign (statement and decode). Point WORLD_DATABASE_URL at a scratch copy of the world:
//
//	WORLD_DATABASE_URL=postgres://bcm:bcm@localhost:5432/bcm_perf?sslmode=disable go test ./internal/postgres -run xxx -bench 'Decode|ReadRoster' -benchmem
func BenchmarkDecodeUnits(b *testing.B) {
	url := os.Getenv("WORLD_DATABASE_URL")
	if url == "" {
		b.Skip("WORLD_DATABASE_URL is not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, `SELECT `+unitsJSON+` FROM companies c`)
	if err != nil {
		b.Fatal(err)
	}
	var all [][]byte
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			b.Fatal(err)
		}
		all = append(all, raw)
	}
	rows.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, raw := range all {
			if _, err := decodeUnits(raw); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkReadRoster(b *testing.B) {
	url := os.Getenv("WORLD_DATABASE_URL")
	if url == "" {
		b.Skip("WORLD_DATABASE_URL is not set")
	}
	ctx := context.Background()
	s, err := OpenWith(ctx, url, Options{SkipMigrations: true})
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	var campaign int64
	if err := s.pool.QueryRow(ctx, `SELECT campaign_id FROM companies GROUP BY campaign_id ORDER BY count(*) DESC, campaign_id LIMIT 1`).Scan(&campaign); err != nil {
		b.Fatal(err)
	}
	cs := companyStore{s}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := cs.ReadRoster(ctx, campaign); err != nil {
			b.Fatal(err)
		}
	}
}
