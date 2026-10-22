package web

import (
	"context"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// Times are stored in UTC and shown in the zone of the viewer (docs/admin.md "Timezone for displaying times"): the saved zone of the
// account, else the browser's (the tz cookie that static/js/tz.js sets), else APP_TIMEZONE, else UTC. The session middleware works the zone
// out once per request; the templates' fmtTime, fmtDate and fmtDateTime use it.

const tzCookie = "tz"

type locKey struct{}

// locationFrom is the zone to show the request's times in.
func locationFrom(ctx context.Context) *time.Location {
	if l, ok := ctx.Value(locKey{}).(*time.Location); ok {
		return l
	}
	return time.UTC
}

// withLocation works the viewer's zone out and makes it available to the handlers (context) and to the renderer (the response writer).
func (s *Server) withLocation(w http.ResponseWriter, r *http.Request) *http.Request {
	browser := ""
	browser = tzCookieValue(r)
	loc := core.UserLocation(userFrom(r.Context()), browser, s.cfg.AppTimezone)
	for rw := w; rw != nil; {
		if rec, ok := rw.(*statusRecorder); ok {
			rec.loc = loc
			break
		}
		u, ok := rw.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			break
		}
		rw = u.Unwrap()
	}
	return r.WithContext(context.WithValue(r.Context(), locKey{}, loc))
}

// writerLocation is the zone withLocation set on the response writer (UTC when none).
func writerLocation(w http.ResponseWriter) *time.Location {
	for rw := w; rw != nil; {
		if rec, ok := rw.(*statusRecorder); ok && rec.loc != nil {
			return rec.loc
		}
		u, ok := rw.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			break
		}
		rw = u.Unwrap()
	}
	return time.UTC
}

// timeFuncs are the template functions that show a time (a time.Time or a *time.Time; "" for none) in loc.
func timeFuncs(loc *time.Location) template.FuncMap {
	show := func(layout string) func(v any) string {
		return func(v any) string {
			switch t := v.(type) {
			case time.Time:
				return t.In(loc).Format(layout)
			case *time.Time:
				if t != nil {
					return t.In(loc).Format(layout)
				}
			}
			return ""
		}
	}
	return template.FuncMap{
		"fmtDate":      show("2 January 2006"),
		"fmtDateShort": show("2 Jan 2006"),
		"fmtDateTime":  show("2 January 2006, 15:04"),
	}
}

// pageIn is the template set of a page with its times shown in loc. Sets are cloned from a pristine one (a template that has been
// executed cannot be cloned) and kept; the cache is emptied when it grows past maxLocationSets, so a visitor who sends many zone names
// cannot grow memory without bound.
const maxLocationSets = 64

type locationSets struct {
	mu   sync.Mutex
	sets map[string]*template.Template
}

func (r *renderer) pageIn(name string, loc *time.Location) (*template.Template, bool) {
	t, ok := r.pages[name]
	if !ok || loc == time.UTC || loc.String() == "UTC" {
		return t, ok
	}
	key := name + "|" + loc.String()
	r.loc.mu.Lock()
	defer r.loc.mu.Unlock()
	if c, ok := r.loc.sets[key]; ok {
		return c, true
	}
	clone, err := r.pristine[name].Clone()
	if err != nil {
		return t, true
	}
	clone.Funcs(timeFuncs(loc))
	if len(r.loc.sets) >= maxLocationSets {
		r.loc.sets = map[string]*template.Template{}
	}
	r.loc.sets[key] = clone
	return clone, true
}
