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
	f.camp.EXPECT().Log(gomock.Any(), frodo, int64(7), int64(3), int64(0), liveLogPage).Return(logEntries(2, 5), nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", testMatchPath, nil))).Body.String()
	for _, want := range []string{`id="ato"`, `name="bonus" value="toolKit"`, "Match log"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	// "none", never no trigger at all: htmx defaults a div with hx-get to click, and every click in the card would fetch it again and lose what was ticked.
	if tag := slotTag(t, body, "ato-slot"); !strings.Contains(tag, `hx-trigger="none"`) {
		t.Errorf("the ATO slot fetches neither on load nor on click: %s", tag)
	}
	if strings.Contains(body, "end-checks-slot") {
		t.Error("the End checks card is gone: the End match sheet asks for the objective and the MVP")
	}
	if tag := slotTag(t, body, "live-log-slot"); strings.Contains(tag, `"load`) || !strings.Contains(tag, `hx-trigger="live:match from:body"`) || !strings.Contains(tag, "data-live-match") {
		t.Errorf("the log slot only refreshes on live:match and still names the match for the stream: %s", tag)
	}
}

// The log of an ended match has nothing to refresh: its slot never fetches, a click in it included.
func TestEndedMatchLogSlotDoesNotFetchOnClick(t *testing.T) {
	m := editableMatch()
	m.Ended = true
	f := matchWrite(t, m)
	f.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(core.ATO{}, nil).AnyTimes()
	f.camp.EXPECT().Log(gomock.Any(), frodo, int64(7), int64(3), int64(0), liveLogPage).Return(logEntries(2, 5), nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", testMatchPath, nil))).Body.String()
	if tag := slotTag(t, body, "live-log-slot"); !strings.Contains(tag, `hx-trigger="none"`) {
		t.Errorf("the log slot of an ended match fetches neither on load nor on click: %s", tag)
	}
}

// A card that cannot be built inline stays a lazy slot (fetched on load), the page itself is fine.
func TestMatchPageCardThatFailsStaysLazy(t *testing.T) {
	f := atoFixture(t)
	f.camp.EXPECT().Log(gomock.Any(), frodo, int64(7), int64(3), int64(0), liveLogPage).Return(logEntries(2, 5), nil)
	rec := f.do(f.signedIn(httptest.NewRequest("GET", testMatchPath, nil))) // the ATO card is not stubbed: it errors
	if rec.Code != 200 {
		t.Fatalf("%d", rec.Code)
	}
	if tag := slotTag(t, rec.Body.String(), "ato-slot"); !strings.Contains(tag, `hx-trigger="load"`) {
		t.Errorf("a failed card falls back to the fetch on load: %s", tag)
	}
}

// The page names the state its cards were built from, so that its event stream does not fetch them again.
func TestMatchPageSendsItsGenerationToTheStream(t *testing.T) {
	f := atoFixture(t)
	f.camp.EXPECT().LiveGeneration(gomock.Any(), frodo, int64(7), int64(3)).Return("4.41", nil)
	f.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(atoCard(), nil)
	f.camp.EXPECT().Log(gomock.Any(), frodo, int64(7), int64(3), int64(0), liveLogPage).Return(logEntries(2, 5), nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", testMatchPath, nil))).Body.String()
	if tag := slotTag(t, body, "live-log-slot"); !strings.Contains(tag, `data-live-since="4.41"`) {
		t.Errorf("the log slot carries the generation: %s", tag)
	}
}

// Without a generation (it could not be read) the slot names none, and the stream starts with a full sync.
func TestMatchPageWithoutAGenerationNamesNone(t *testing.T) {
	f := atoFixture(t)
	f.camp.EXPECT().Log(gomock.Any(), frodo, int64(7), int64(3), int64(0), liveLogPage).Return(logEntries(2, 5), nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", testMatchPath, nil))).Body.String()
	if tag := slotTag(t, body, "live-log-slot"); strings.Contains(tag, "data-live-since") {
		t.Errorf("no generation to send: %s", tag)
	}
}
