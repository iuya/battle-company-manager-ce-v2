package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// slotTag returns the opening tag of the element with the id.
func slotTag(t *testing.T, body, id string) string {
	t.Helper()
	i := strings.Index(body, `id="`+id+`"`)
	if i < 0 {
		t.Fatalf("no %s in the page", id)
	}
	i = strings.LastIndex(body[:i], "<")
	return body[i : i+strings.Index(body[i:], ">")+1]
}

// The full match page draws its cards inline (one round trip fewer); the slots keep only the refresh on
// live:match, so nothing fetches on load.
func TestMatchPageDrawsItsCardsInline(t *testing.T) {
	f := atoFixture(t)
	f.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(atoCard(), nil)
	f.camp.EXPECT().EndChecks(gomock.Any(), frodo, int64(7), int64(3)).Return(core.EndChecks{}, nil)
	f.camp.EXPECT().Log(gomock.Any(), frodo, int64(7), int64(3), int64(0), liveLogPage).Return(logEntries(2, 5), nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, want := range []string{`id="ato"`, `name="bonus" value="toolKit"`, "Match log"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if tag := slotTag(t, body, "ato-slot"); strings.Contains(tag, "hx-trigger") {
		t.Errorf("the ATO slot does not fetch on load: %s", tag)
	}
	if tag := slotTag(t, body, "end-checks-slot"); strings.Contains(tag, `"load`) || !strings.Contains(tag, `hx-trigger="live:match from:body"`) {
		t.Errorf("the end checks slot only refreshes on live:match: %s", tag)
	}
	if tag := slotTag(t, body, "live-log-slot"); strings.Contains(tag, `"load`) || !strings.Contains(tag, `hx-trigger="live:match from:body"`) || !strings.Contains(tag, "data-live-match") {
		t.Errorf("the log slot only refreshes on live:match and still names the match for the stream: %s", tag)
	}
}

// A card that cannot be built inline stays a lazy slot (fetched on load), the page itself is fine.
func TestMatchPageCardThatFailsStaysLazy(t *testing.T) {
	f := atoFixture(t)
	f.camp.EXPECT().EndChecks(gomock.Any(), frodo, int64(7), int64(3)).Return(core.EndChecks{}, nil)
	f.camp.EXPECT().Log(gomock.Any(), frodo, int64(7), int64(3), int64(0), liveLogPage).Return(logEntries(2, 5), nil)
	rec := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))) // the ATO card is not stubbed: it errors
	if rec.Code != 200 {
		t.Fatalf("%d", rec.Code)
	}
	if tag := slotTag(t, rec.Body.String(), "ato-slot"); !strings.Contains(tag, `hx-trigger="load"`) {
		t.Errorf("a failed card falls back to the fetch on load: %s", tag)
	}
}
