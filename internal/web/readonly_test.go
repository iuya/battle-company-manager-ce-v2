package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// readOnly hands the handler a context that already has a read scope: opening one again changes nothing (WithReadScope returns the same
// context), and a plain request context has none, so WithReadScope makes a new one.
func TestReadOnlyOpensAReadScope(t *testing.T) {
	var seen context.Context
	h := readOnly(func(w http.ResponseWriter, r *http.Request) { seen = r.Context() })
	r := httptest.NewRequest("GET", "/", nil)
	h(httptest.NewRecorder(), r)
	if seen == nil {
		t.Fatal("the handler was not called")
	}
	if core.WithReadScope(seen) != seen {
		t.Error("the handler's context has no read scope")
	}
	if core.WithReadScope(r.Context()) == r.Context() {
		t.Error("a plain request context should not have a read scope")
	}
}
