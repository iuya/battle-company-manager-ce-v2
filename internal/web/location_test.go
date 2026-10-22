package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func TestTimeFuncsShowTheViewersZoneAcrossADSTChange(t *testing.T) {
	auckland, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Skip("no tzdata")
	}
	f := timeFuncs(auckland)["fmtDateTime"].(func(any) string)
	// 2026-09-26 13:59 UTC is 01:59 NZST (+12) on the 27th; two hours later NZDT (+13) starts: 14:01 UTC is 03:01 NZDT.
	if got := f(time.Date(2026, 9, 26, 13, 59, 0, 0, time.UTC)); got != "27 September 2026, 01:59" {
		t.Errorf("before the change: %q", got)
	}
	if got := f(time.Date(2026, 9, 26, 14, 1, 0, 0, time.UTC)); got != "27 September 2026, 03:01" {
		t.Errorf("after the change: %q", got)
	}
	d := timeFuncs(time.UTC)["fmtDate"].(func(any) string)
	if d(time.Time{}) == "" || d((*time.Time)(nil)) != "" || d("x") != "" {
		t.Error("a nil pointer or another type is empty")
	}
}

func TestPagesShowTimesInTheAccountsZoneAndOthersAreUnaffected(t *testing.T) {
	m := editableMatch()
	m.PlayedOn = time.Date(2026, 9, 26, 23, 30, 0, 0, time.UTC) // already the 27th in Auckland
	when := func(u core.User) string {
		f := matchWrite(t, m)
		f.camp.EXPECT().Scenarios(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		r := f.signedInAs(u, httptest.NewRequest("GET", matchPath, nil))
		return f.do(r).Body.String()
	}
	nz := frodo
	nz.Timezone = "Pacific/Auckland"
	if body := when(nz); !strings.Contains(body, "27 September 2026") {
		t.Error("Auckland sees the 27th")
	}
	if body := when(frodo); !strings.Contains(body, "26 September 2026") || strings.Contains(body, "27 September 2026") {
		t.Error("others see UTC")
	}
	// the browser's zone applies when the account has none
	f := matchWrite(t, m)
	f.camp.EXPECT().Scenarios(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	r := f.signedIn(httptest.NewRequest("GET", matchPath, nil))
	r.AddCookie(&http.Cookie{Name: "tz", Value: "Pacific/Auckland"})
	if body := f.do(r).Body.String(); !strings.Contains(body, "27 September 2026") {
		t.Error("the tz cookie is used")
	}
}

func TestAccountTimezoneIsSavedOrRefused(t *testing.T) {
	f := newFixture(t, Config{})
	f.auth.EXPECT().SetTimezone(gomock.Any(), frodo.ID, "Europe/Paris").Return(nil)
	rec := f.do(f.signedIn(form("/account/timezone", url.Values{"zone": {"Europe/Paris"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Your time zone is saved.") || !strings.Contains(rec.Body.String(), `value="Europe/Paris"`) {
		t.Errorf("saved: %d", rec.Code)
	}
	f.auth.EXPECT().SetTimezone(gomock.Any(), frodo.ID, "Mars/Olympus").Return(core.ErrInvalidUser)
	rec = f.do(f.signedIn(form("/account/timezone", url.Values{"zone": {"Mars/Olympus"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "not a time zone name") {
		t.Errorf("refused: %d", rec.Code)
	}
}
