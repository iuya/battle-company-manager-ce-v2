package web

import (
	"html"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// hostile is a name that tries every way out of every context at once: a quote of each kind and a backslash (attribute, JS string
// and Alpine expression), angle brackets and a script tag (text), a handler-carrying element, a template action, and a
// closing-and-reopening of an Alpine object.
const hostile = `"'\><img src=x onerror=alert(1)></a><script>alert(7)</script>'+alert(2)+'});alert(3);({{{.}}}`

// taintedFields are the names of the string fields that hold free text: what a player types or a rules file supplies.
var taintedFields = map[string]bool{
	"Name": true, "Owner": true, "Organiser": true, "Description": true, "Note": true, "Text": true, "Label": true, "Stats": true,
	"Trail": true, "DisplayName": true, "Email": true, "Type": true, "Victory": true, "Rule": true, "Unit": true, "CampaignName": true,
}

// taint sets every free-text string field reachable from v (a pointer) to the hostile string.
func taint(v any) { taintValue(reflect.ValueOf(v)) }

func taintValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			taintValue(v.Elem())
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			taintValue(v.Index(i))
		}
	case reflect.Struct:
		for i := range v.NumField() {
			f := v.Field(i)
			if !f.CanSet() {
				continue
			}
			if f.Kind() == reflect.String && taintedFields[v.Type().Field(i).Name] && f.Type() == reflect.TypeFor[string]() {
				f.SetString(hostile)
				continue
			}
			taintValue(f)
		}
	}
}

var (
	tagRe  = regexp.MustCompile(`(?s)<([a-zA-Z][a-zA-Z0-9-]*)((?:\s+[^\s=/>"']+(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>"']+))?)*)\s*/?>`)
	attrRe = regexp.MustCompile(`([^\s=/>"']+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>"']+)))?`)
)

// reporter is the part of testing.T audit needs, so a test can check that the audit itself catches an injection.
type reporter interface {
	Errorf(format string, args ...any)
}

type recorder struct {
	failed bool
	msgs   []string
}

func (r *recorder) Errorf(format string, args ...any) {
	r.failed = true
	r.msgs = append(r.msgs, format)
}

// audit looks at the rendered page the way a browser would and reports every sign that the hostile text escaped its context:
// an element or attribute it created (a script, an image with a handler, an on* attribute), or the text verbatim inside an
// attribute that is run as JavaScript (Alpine's x-*, @*, :* and the on* handlers).
func audit(t reporter, name, body string) {
	if strings.Contains(body, "<img src=x") || strings.Contains(body, "<script>alert") || strings.Contains(body, `onerror=alert`) && !strings.Contains(body, "onerror=alert(1)&gt;") && !strings.Contains(body, "onerror=alert(1)&amp;gt;") && !strings.Contains(body, `onerror=alert(1)>`) {
		t.Errorf("%s: the hostile text appears unescaped in the page", name)
	}
	for _, m := range tagRe.FindAllStringSubmatch(body, -1) {
		tag := strings.ToLower(m[1])
		if tag == "img" && strings.Contains(m[2], "src=x") {
			t.Errorf("%s: an <img src=x> element was created", name)
		}
		if tag == "script" && !strings.Contains(m[2], "src=") {
			t.Errorf("%s: an inline <script> was created: %s", name, m[0])
		}
		for _, a := range attrRe.FindAllStringSubmatch(m[2], -1) {
			attr := strings.ToLower(a[1])
			val := html.UnescapeString(cmp3(a[2], a[3], a[4]))
			switch {
			case attr == "src" && val == "x", attr == "onerror", attr == "alert(1)", attr == "alert(1)>", strings.HasPrefix(attr, "alert"):
				t.Errorf("%s: <%s> got the hostile attribute %q", name, tag, attr)
			case strings.HasPrefix(attr, "on") && attr != "one":
				t.Errorf("%s: <%s> has the event handler attribute %q", name, tag, attr)
			case strings.HasPrefix(attr, "x-") && attr != "x-ref" || strings.HasPrefix(attr, "@") || strings.HasPrefix(attr, ":"):
				if strings.Contains(val, hostile) || strings.Contains(val, `'+alert(2)+'`) || strings.Contains(val, `alert(3);(`) {
					t.Errorf("%s: <%s %s> carries the hostile text verbatim into a JavaScript context: %s", name, tag, attr, val)
				}
			}
		}
	}
}

func cmp3(a, b, c string) string {
	if a != "" {
		return a
	}
	if b != "" {
		return b
	}
	return c
}

func TestAuditCatchesInjection(t *testing.T) {
	good := `<div x-data="{ n: &#34;\&#34;&#39;\\\u003e&#34; }" title="` + html.EscapeString(hostile) + `">` + html.EscapeString(hostile) + `</div>`
	r := &recorder{}
	audit(r, "good", good)
	if r.failed {
		t.Errorf("the audit rejected escaped text: %v", r.msgs)
	}
	for name, bad := range map[string]string{
		"alpine":  `<div x-data="{ n: '` + html.EscapeString(hostile) + `' }">`,
		"element": `<div>` + hostile + `</div>`,
		"handler": `<div title="x" onclick="alert(1)">`,
		"script":  `<script>alert(1)</script>`,
		"image":   `<img src=x onerror=alert(1)>`,
	} {
		r := &recorder{}
		audit(r, name, bad)
		if !r.failed {
			t.Errorf("%s: the audit missed an injection", name)
		}
	}
}

// hostileViewer is the signed-in user, with a hostile name and address.
var hostileViewer = core.User{ID: 1, Email: hostile, DisplayName: hostile, Role: core.RoleSuperuser, CanCreateCampaigns: true}

// hostileWorld stubs the services with hostile data on every free-text field and returns the pages to render.
func hostileWorld(t *testing.T) (*fixture, []string) {
	t.Helper()
	f := newFixture(t, Config{StyleGuide: true})
	anyCtx := gomock.Any()

	d, detail := mineDetail()
	d.OrganiserID = 99
	d.Rulesets = []core.RulesetInfo{{Kind: core.KindBattleCompanies, Hash: "abcdef0123456789", Name: "x", Version: "y"}}
	detail.Roster = append(detail.Roster, core.CompanyUnit{ID: 6, Name: "n", Type: "Captain", Role: core.RoleLeader, Kind: core.KindInfantry, Points: 70,
		Stats:     []core.Attribute{{Abbr: "Mv", Name: "Movement", Value: 6, Trail: `"`}},
		Equipment: []core.Equipment{{Name: "Sword", Class: core.EquipMelee}, {Name: "Bow", Class: core.EquipBow, Bought: true, Stats: `24" · S2`, Description: "Can shoot."}},
		Rules:     []core.UnitRule{{Name: "Bodyguard", Description: "Protects.", Source: core.RuleFromUnit}},
		Path:      &core.UnitPath{Name: "Path", HeroicActions: []core.HeroicAction{{Name: "Heroic Strike", Source: core.ActionFromUnit}}, Rules: []core.UnitRule{{Name: "Rally", Description: "Cheers.", Source: core.RuleFromPath}}}})
	detail.Limits = []core.CompanyLimit{{Label: "Bows", Used: 1, Max: 2}}
	detail.Perks = []core.Perk{{ID: 1, MatchID: 3, Effect: "nextGame", Name: "p", Text: "d"}}
	other := core.CampaignCompany{ID: 2, Name: "n", Owner: "o", Faction: "Gondor", OwnerID: 50, Points: 10, Units: 3}
	otherDetail := core.CompanyDetail{CampaignCompany: other, Roster: []core.CompanyUnit{{ID: 9, Name: "n", Type: "Captain", Role: core.RoleLeader, Points: 60}}}
	taint(&d)
	taint(&detail)
	taint(&other)
	taint(&otherDetail)
	d.Companies[0].Faction, detail.Faction = "Gondor", "Gondor"
	hostileCompany := d.Companies[0]

	match := core.Match{ID: 3, Number: 3, Scenario: &core.MatchScenario{Kind: core.ScenarioCustom, Name: "n", Description: "d", Victory: "v"},
		Result: core.MatchResult{Decided: true, Winner: 1, Note: "n", Sides: make([]core.SideResult, 2)},
		Sides: []core.MatchSide{
			{Name: "n", Companies: []core.CampaignCompany{{ID: 1, Name: "n", OwnerID: 1, Owner: "o"}}},
			{Name: "n", Companies: []core.CampaignCompany{{ID: 2, Name: "n", OwnerID: 50, Owner: "o"}}}}}
	taint(&match)
	match.Result.Note = hostile

	summaries := []core.CampaignSummary{{ID: 7, Name: "n", Description: "d", Status: core.CampaignActive, Round: 1, Rounds: 8, Matches: 1, Companies: []core.CampaignCompany{hostileCompany}}}
	taint(&summaries)
	scenarios := []core.ScenarioInfo{{Name: "Seize Ground", Scoring: []string{hostile}, About: core.ScenarioAbout{}}}
	profiles := []core.CompanyProfile{{BCCompany: core.BCCompany{Name: "Minas Tirith", Alignment: "good", MaxModels: 15, ReinforcementCost: 3}, Description: hostile,
		Oath:         core.Oath{Text: hostile + " {name}", Vow: hostile},
		Start:        []core.StartUnit{{Count: 2, Unit: "Warrior of Minas Tirith", Wargear: []string{hostile}}},
		Paths:        []core.PathProgression{{Name: hostile}},
		HeroUpgrades: []core.HeroUpgrade{{Name: hostile, Rule: hostile, Text: hostile}}}}
	options := []core.CampaignOption{{Key: "k", Scope: "rules", Label: hostile, Description: hostile}}
	usage := []core.RulesetUsage{{RulesetInfo: core.RulesetInfo{Kind: core.KindBattleCompanies, Hash: "abcdef0123456789", Name: hostile, Version: hostile}, Size: 10, UploadedBy: &hostileViewer, CreatedAt: time.Now(), Campaigns: []core.CampaignRef{{ID: 7, Name: hostile}}}}
	admin := []core.AdminUser{{User: hostileViewer, Campaigns: 1, CreatedAt: time.Now(), Organises: []core.OrganisedCampaign{{ID: 7, Name: hostile, Status: core.CampaignActive}}}}

	f.auth.EXPECT().Authenticate(anyCtx, "tok").Return(hostileViewer, time.Time{}, nil).AnyTimes()
	f.auth.EXPECT().ListUsers(anyCtx, anyCtx, anyCtx, anyCtx, anyCtx).Return(admin, 1, nil).AnyTimes()
	f.rs.EXPECT().ListWithUsage(anyCtx, anyCtx).Return(usage, nil).AnyTimes()
	f.rs.EXPECT().Default(anyCtx, anyCtx).Return(core.RulesetInfo{Name: hostile, Version: hostile, Hash: "abcdef0123456789"}, nil).AnyTimes()
	f.rs.EXPECT().Lookup(anyCtx, anyCtx).Return(core.RulesetInfo{Name: hostile, Version: hostile, Hash: "abcdef0123456789"}, nil).AnyTimes()
	f.camp.EXPECT().List(anyCtx, anyCtx, anyCtx).Return(summaries, nil).AnyTimes()
	f.nav = []core.CampaignNav{{ID: 7, Name: hostile, Status: core.CampaignActive}}
	f.camp.EXPECT().Get(anyCtx, anyCtx, anyCtx).Return(d, nil).AnyTimes()
	f.camp.EXPECT().Matches(anyCtx, anyCtx).Return([]core.Match{match}, nil).AnyTimes()
	f.camp.EXPECT().Match(anyCtx, anyCtx, anyCtx).Return(match, nil).AnyTimes()
	f.camp.EXPECT().Companies(anyCtx, anyCtx).Return([]core.CampaignCompany{hostileCompany, other}, nil).AnyTimes()
	f.camp.EXPECT().Members(anyCtx, anyCtx).Return([]core.CampaignMember{{UserID: 1, Name: hostile, Organiser: true}, {UserID: 2, Name: hostile, Companies: []core.MemberCompany{{ID: 2, Name: hostile}}}}, nil).AnyTimes()
	f.camp.EXPECT().Company(anyCtx, anyCtx, int64(2)).Return(other, nil).AnyTimes()
	f.camp.EXPECT().Company(anyCtx, anyCtx, int64(1)).Return(hostileCompany, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(anyCtx, anyCtx, int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(anyCtx, anyCtx, int64(2)).Return(otherDetail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(anyCtx, anyCtx, anyCtx, anyCtx).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(anyCtx, anyCtx, anyCtx, anyCtx).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(anyCtx, anyCtx, anyCtx, anyCtx).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().Scenarios(anyCtx, anyCtx).Return(scenarios, nil).AnyTimes()
	f.camp.EXPECT().CompanyProfiles(anyCtx, anyCtx).Return(profiles, nil).AnyTimes()
	f.camp.EXPECT().ArmouryEntries(anyCtx, anyCtx, anyCtx).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().ArmouryCreatures(anyCtx, anyCtx, anyCtx).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().ArmouryWanderers(anyCtx, anyCtx, anyCtx).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().Preview(anyCtx, anyCtx, anyCtx).Return(core.InvitePreview{CampaignID: 7, CampaignName: hostile}, nil).AnyTimes()
	am := core.Aftermath{MatchID: 3, CompanyID: 1, Status: core.AftermathInjuries, Outcome: "win", Access: core.AftermathEdit, WaivedNote: hostile,
		Casualties: []core.Casualty{{UnitID: 5, Name: "n", Hero: true, Rolled: true, Dice: []int{3, 4}, Total: 7, ResultTxt: "n", Text: "n"}},
		Lines:      []core.AftermathLine{{Kind: "note", Text: "n"}}}
	taint(&am)
	rules := core.AftermathRules{Gains: []core.XPGain{{Key: "k", Label: hostile, Amount: 1}},
		Bonuses: []core.OddsBonus{{Key: "k", Name: hostile, Text: hostile}},
		Warrior: []core.InjuryRow{{Min: 1, Max: 6, Key: "k", Name: hostile, Text: hostile}}, Hero: []core.InjuryRow{{Min: 2, Max: 12, Key: "k", Name: hostile, Text: hostile}}}
	f.camp.EXPECT().AftermathRules(anyCtx, anyCtx).Return(rules, nil).AnyTimes()
	f.camp.EXPECT().Aftermath(anyCtx, anyCtx, anyCtx, anyCtx, anyCtx).Return(am, nil).AnyTimes()
	f.camp.EXPECT().Live(anyCtx, anyCtx, anyCtx, anyCtx).Return(core.LiveCompany{}, core.ErrNotFound).AnyTimes()
	f.rs.EXPECT().CampaignOptions(anyCtx).Return(options, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(anyCtx, anyCtx).Return(core.Glossary{}, nil).AnyTimes()
	f.gl.EXPECT().Default(anyCtx).Return(core.Glossary{}, nil).AnyTimes()

	return f, []string{
		"/", "/?finished=1", "/campaigns/new", "/campaigns/7", "/campaigns/7/companies", "/campaigns/7/rankings", "/campaigns/7/matches",
		"/campaigns/7/my-company", "/campaigns/7/my-company/recruit", "/campaigns/7/companies/2", "/campaigns/7/armory",
		"/campaigns/7/company/new", "/campaigns/7/company/info?c=Minas+Tirith", "/campaigns/7/matches/new", "/campaigns/7/matches/3",
		"/campaigns/7/matches/3/scenario", "/campaigns/7/glossary", "/glossary", "/account", "/admin", "/admin/users", "/admin/rulesets",
		"/join/sometoken", "/nav/campaigns", "/styleguide", "/campaigns/7/matches/3/aftermath/1",
	}
}

// noHostileText lists the pages that show no free text from the stubs (the style guide, menus, fragments of fixed words).
var noHostileText = map[string]bool{"/styleguide": true}

// Every page type renders hostile names, descriptions and rules-file text without letting them out of their context: in text,
// in plain attributes and in the attributes Alpine evaluates as JavaScript.
func TestHostileStringsAreEscapedOnEveryPage(t *testing.T) {
	f, pages := hostileWorld(t)
	for _, path := range pages {
		for _, htmx := range []bool{false, true} {
			name := path
			r := httptest.NewRequest("GET", path, nil)
			if htmx {
				r.Header.Set("HX-Request", "true")
				name += " (htmx)"
			}
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
			rec := f.do(r)
			if rec.Code >= 500 {
				t.Errorf("%s: %d", name, rec.Code)
				continue
			}
			audit(t, name, rec.Body.String())
			if rec.Code == 200 && !strings.Contains(rec.Body.String(), "alert(2)") && !noHostileText[path] {
				t.Errorf("%s: the page shows none of the hostile text, so it proves nothing: feed it some or list it in noHostileText", name)
			}
		}
	}
}
