package web

import "testing"

// BenchmarkStaticFiles is the start-up cost of the embedded asset table: reading the files, hashing them and the bundle.
//
// go test ./internal/web -run xxx -bench StaticFiles -benchmem -count 6
func BenchmarkStaticFiles(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if len(buildStaticFiles()) == 0 {
			b.Fatal("no static files")
		}
	}
}

// BenchmarkStaticGzip is what making the gzip copy of every text file costs (best compression): done once per process, in the background after
// the start (WarmStatic) or at the first request that needs a copy.
func BenchmarkStaticGzip(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, f := range buildStaticFiles() {
			f.gz()
		}
	}
}
