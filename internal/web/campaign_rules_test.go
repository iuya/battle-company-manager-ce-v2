package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func rulesUpdateFor(clean bool) core.RulesUpdate {
	file := func(k core.RulesetKind, from, to string, differs bool) core.RulesFile {
		return core.RulesFile{Kind: k, Pinned: core.RulesetInfo{Kind: k, Hash: from + "0000000000", Name: "BC", Version: "1"}, Current: core.RulesetInfo{Kind: k, Hash: to + "0000000000", Name: "BC", Version: "2", Default: true}, Differs: differs}
	}
	u := core.RulesUpdate{CampaignID: 7, Name: "Siege", Differs: true, Files: []core.RulesFile{file(core.KindVanilla, "aaaaaaaa", "aaaaaaaa", false), file(core.KindBattleCompanies, "bbbbbbbb", "cccccccc", true), file(core.KindGlossary, "dddddddd", "dddddddd", false)},
		Companies: []core.RulesUpdateCompany{{ID: 1, Name: "Grey", Points: [2]int{100, 101}, Gold: [2]int{4, 5}, Changes: []string{"Gold 4 → 5."}}}}
	if !clean {
		u.Companies = append(u.Companies, core.RulesUpdateCompany{ID: 2, Name: "Shire", Points: [2]int{90, 90}, Problems: []string{"Pippin: \"Lost Blade\" is not in the new files."}})
		u.Blockers = []string{"1 match is still open: finish or delete it first."}
	}
	return u
}

func TestRulesUpdateCheckShowsTheReport(t *testing.T) {
	f := organiserFixture(t, gandalf, core.CampaignActive)
	f.camp.EXPECT().RulesUpdatePreview(gomock.Any(), gandalf, int64(7)).Return(rulesUpdateFor(true), nil)
	rec := f.do(f.signedInAs(gandalf, httpGet("/campaigns/7/rules-update")))
	body := rec.Body.String()
	for _, want := range []string{`id="rules-update"`, "data-clean", "Works", "Gold 4 → 5.", "bbbbbbbb", "cccccccc", `hx-post="/campaigns/7/rules-update"`, "Move to the current files"} {
		if !strings.Contains(body, want) {
			t.Errorf("a clean report lacks %q", want)
		}
	}
	if strings.Contains(body, "switch_custom") {
		t.Error("no custom switch for an older edition")
	}

	f = organiserFixture(t, gandalf, core.CampaignActive)
	f.camp.EXPECT().RulesUpdatePreview(gomock.Any(), gandalf, int64(7)).Return(rulesUpdateFor(false), nil)
	body = f.do(f.signedInAs(gandalf, httpGet("/campaigns/7/rules-update"))).Body.String()
	if !strings.Contains(body, "Lost Blade") || !strings.Contains(body, "data-blocker") || !strings.Contains(body, "1 problem") || strings.Contains(body, "Move to the current files") {
		t.Errorf("a blocked report names what blocks and offers no move: %s", body)
	}

	u := rulesUpdateFor(true)
	u.Custom = true
	f = organiserFixture(t, gandalf, core.CampaignActive)
	f.camp.EXPECT().RulesUpdatePreview(gomock.Any(), gandalf, int64(7)).Return(u, nil)
	if body = f.do(f.signedInAs(gandalf, httpGet("/campaigns/7/rules-update"))).Body.String(); !strings.Contains(body, `name="switch_custom" required`) {
		t.Error("a custom file asks for the explicit switch")
	}
}

func TestRulesUpdateMovesOrShowsTheRefusal(t *testing.T) {
	f := organiserFixture(t, gandalf, core.CampaignActive)
	f.camp.EXPECT().UpdateRules(gomock.Any(), gandalf, int64(7), false).Return(nil)
	r := f.signedInAs(gandalf, form("/campaigns/7/rules-update", url.Values{}))
	r.Header.Set("HX-Request", "true")
	if rec := f.do(r); rec.Header().Get("HX-Redirect") != "/campaigns/7" {
		t.Errorf("redirect: %d %v", rec.Code, rec.Header())
	}
	f.camp.EXPECT().UpdateRules(gomock.Any(), gandalf, int64(7), true).Return(core.ErrRulesUpdateBlocked)
	f.camp.EXPECT().RulesUpdatePreview(gomock.Any(), gandalf, int64(7)).Return(rulesUpdateFor(false), nil)
	rec := f.do(f.signedInAs(gandalf, form("/campaigns/7/rules-update", url.Values{"switch_custom": {"on"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "cannot be moved to the current files") {
		t.Errorf("refusal: %d %.300s", rec.Code, rec.Body.String())
	}
	g := organiserFixture(t, frodo, core.CampaignActive)
	if rec := g.do(g.signedIn(httpGet("/campaigns/7/rules-update"))); rec.Code != http.StatusForbidden {
		t.Errorf("a player checks: %d", rec.Code)
	}
	if rec := g.do(g.signedIn(form("/campaigns/7/rules-update", url.Values{}))); rec.Code != http.StatusForbidden {
		t.Errorf("a player moves: %d", rec.Code)
	}
}

func TestRulesCardOfTheOverview(t *testing.T) {
	overview := func(u core.User, status core.CampaignStatus, current bool) string {
		f := newFixture(t, Config{})
		d := campaignDetail()
		d.OrganiserID, d.Status = gandalf.ID, status
		d.Rulesets = []core.RulesetInfo{{Kind: core.KindVanilla, Hash: "aaaaaaaa11", Name: "V", Default: true}, {Kind: core.KindBattleCompanies, Hash: "bbbbbbbb11", Name: "BC", Version: "1", Default: current}, {Kind: core.KindGlossary, Hash: "cccccccc11", Name: "G", Default: true}}
		f.camp.EXPECT().Get(gomock.Any(), u.ID, int64(7)).Return(d, nil).AnyTimes()
		f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return(nil, nil).AnyTimes()
		f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return(nil, nil).AnyTimes()
		f.camp.EXPECT().Members(gomock.Any(), int64(7)).Return(nil, nil).AnyTimes()
		return f.do(f.signedInAs(u, httpGet("/campaigns/7"))).Body.String()
	}
	if body := overview(gandalf, core.CampaignActive, false); !strings.Contains(body, "data-rules-update") || !strings.Contains(body, "Check the update") {
		t.Error("the organiser of a campaign on an older file is offered the check")
	}
	if body := overview(sauron, core.CampaignActive, false); !strings.Contains(body, "Check the update") {
		t.Error("an administrator is offered the check too")
	}
	if body := overview(gandalf, core.CampaignActive, true); strings.Contains(body, "Check the update") {
		t.Error("nothing to offer when every file is current")
	}
	if body := overview(gandalf, core.CampaignFinished, false); strings.Contains(body, "Check the update") {
		t.Error("an ended campaign never changes")
	}
}
