// Package perftrace is the developer-only measuring aid of the optimisation pass (docs/performance.md). With PERF_TRACE=true the
// server answers every request with a Server-Timing header (total time up to the first byte, database statements and database time),
// which tests/e2e/perf.mjs reads. It is off by default and must stay off in production: it only adds a header, but it is not needed there.
package perftrace

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

type ctxKey struct{}

type counter struct {
	stmts atomic.Int64
	nanos atomic.Int64
}

// Tracer counts the statements (and their time) run under a request context made by Middleware. Pass it as postgres.Options.Tracer.
type Tracer struct{}

type startKey struct{}

func (Tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	if _, ok := ctx.Value(ctxKey{}).(*counter); !ok {
		return ctx
	}
	return context.WithValue(ctx, startKey{}, time.Now())
}

func (Tracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	c, ok := ctx.Value(ctxKey{}).(*counter)
	if !ok {
		return
	}
	c.stmts.Add(1)
	if t, ok := ctx.Value(startKey{}).(time.Time); ok {
		c.nanos.Add(int64(time.Since(t)))
	}
}

func (Tracer) TraceBatchStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceBatchStartData) context.Context {
	if _, ok := ctx.Value(ctxKey{}).(*counter); !ok {
		return ctx
	}
	return context.WithValue(ctx, startKey{}, time.Now())
}

func (Tracer) TraceBatchQuery(ctx context.Context, _ *pgx.Conn, _ pgx.TraceBatchQueryData) {
	if c, ok := ctx.Value(ctxKey{}).(*counter); ok {
		c.stmts.Add(1)
	}
}

func (Tracer) TraceBatchEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceBatchEndData) {
	c, ok := ctx.Value(ctxKey{}).(*counter)
	if !ok {
		return
	}
	if t, ok := ctx.Value(startKey{}).(time.Time); ok {
		c.nanos.Add(int64(time.Since(t)))
	}
}

type writer struct {
	http.ResponseWriter
	c     *counter
	start time.Time
	sent  bool
}

func (w *writer) stamp() {
	if w.sent {
		return
	}
	w.sent = true
	w.Header().Add("Server-Timing", fmt.Sprintf("app;dur=%.2f, db;dur=%.2f;desc=\"%d\"",
		float64(time.Since(w.start))/1e6, float64(w.c.nanos.Load())/1e6, w.c.stmts.Load()))
}

func (w *writer) WriteHeader(code int) { w.stamp(); w.ResponseWriter.WriteHeader(code) }
func (w *writer) Write(b []byte) (int, error) {
	w.stamp()
	return w.ResponseWriter.Write(b)
}
func (w *writer) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Middleware adds the Server-Timing header: app (milliseconds until the response starts), db (milliseconds in the database, the
// description is the number of statements).
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		c := &counter{}
		w := &writer{ResponseWriter: rw, c: c, start: time.Now()}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, c)))
	})
}
