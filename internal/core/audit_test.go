package core

import (
	"log/slog"
	"slices"
	"strings"
	"testing"
)

func TestAuditCoversEveryCompany(t *testing.T) {
	rs := shippedRules(t)
	report := AuditRules(rs, AuditInput{})
	if got, want := len(report.Companies), len(rs.BC.Profiles()); got != want || got < 37 {
		t.Fatalf("audited %d companies, rules have %d (at least 37 expected)", got, want)
	}
	files := RenderAudit(report)
	if _, ok := files["index.md"]; !ok || len(files) != len(report.Companies)+1 {
		t.Fatalf("want one file per company and an index, got %d files", len(files))
	}
	for _, c := range report.Companies {
		body := files[AuditFileName(c.Name)]
		for _, section := range []string{"## Anomalies", "## Start", "## Recruiting", "## Units", "## Armoury of the company", "## Tooltips"} {
			if !strings.Contains(body, section) {
				t.Errorf("%s: report lacks %q", c.Name, section)
			}
		}
		if len(c.Units) == 0 || len(c.Armoury) == 0 {
			t.Errorf("%s: %d units, %d armoury items", c.Name, len(c.Units), len(c.Armoury))
		}
		for _, f := range c.Flags {
			if !slices.Contains(AuditKinds, f.Kind) {
				t.Errorf("%s: unknown flag kind %q", c.Name, f.Kind)
			}
		}
	}
	again := RenderAudit(AuditRules(rs, AuditInput{}))
	for name, body := range files {
		if again[name] != body {
			t.Errorf("%s: output is not stable between runs", name)
		}
	}
}

func TestAuditFlags(t *testing.T) {
	rs := shippedRules(t)
	base, ok := rs.BC.Profile("Minas Tirith")
	if !ok {
		t.Fatal("no Minas Tirith")
	}
	for _, tc := range []struct {
		name   string
		change func(p *CompanyProfile)
		in     AuditInput
		kind   string
		want   string // part of the subject
	}{
		{"unit that is no profile", func(p *CompanyProfile) { p.Start = append(p.Start, StartUnit{Count: 1, Unit: "Nobody At All"}) }, AuditInput{}, FlagUnitUnresolved, "Nobody At All"},
		{"wargear spelled in lower case", func(p *CompanyProfile) {
			p.Start = append(p.Start, StartUnit{Count: 1, Unit: "Warrior of Minas Tirith", Wargear: []string{"shield"}})
		}, AuditInput{}, FlagNameFormat, "shield"},
		{"wargear the item table does not know", func(p *CompanyProfile) {
			p.Start = append(p.Start, StartUnit{Count: 1, Unit: "Warrior of Minas Tirith", Wargear: []string{"Sparkly Thing"}})
		}, AuditInput{}, FlagOptionUnknown, "Sparkly Thing"},
		{"duplicate that first wins", func(p *CompanyProfile) {}, AuditInput{Ambiguities: []VanillaAmbiguity{{Kind: "unit", Name: "Warrior of Minas Tirith", Status: "firstWins"}}}, FlagDuplicateName, "Warrior of Minas Tirith"},
		{"unreviewed duplicate", func(p *CompanyProfile) {}, AuditInput{Unreviewed: []VanillaAmbiguity{{Kind: "unit", Name: "Warrior of Minas Tirith", Status: auditUnreviewedStatus}}}, FlagDuplicateName, "Warrior of Minas Tirith"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			p.Start = slices.Clone(base.Start)
			tc.change(&p)
			ca := auditCompany(rs, tc.in, newItemTable(rs), p.WithPreviews(rs.Vanilla))
			if !slices.ContainsFunc(ca.Flags, func(f AuditFlag) bool { return f.Kind == tc.kind && strings.Contains(f.Subject, tc.want) }) {
				t.Errorf("no %s flag about %q in %v", tc.kind, tc.want, ca.Flags)
			}
		})
	}
}

func TestAuditTooltipNeedsItsName(t *testing.T) {
	rs := shippedRules(t)
	for _, tc := range []struct {
		text, name string
		want       bool
	}{
		{"A Barding protects the steed.", "Barding", true},
		{"Armours the steed.", "Barding", false},
		{"A throwing spear can be thrown.", "Throwing Spears", true},
	} {
		if got := tipMentions(rs, tc.text, tc.name); got != tc.want {
			t.Errorf("tipMentions(%q, %q) = %v, want %v", tc.text, tc.name, got, tc.want)
		}
	}
}

func TestCaptureDuplicatesCollectsLoaderWarnings(t *testing.T) {
	c := &CaptureDuplicates{}
	slog.New(c).Warn("vanilla defines a name more than once", "kind", "gear", "name", "Spear", "occurrences", 2)
	slog.New(c).Warn("something else")
	if len(c.Found) != 1 || c.Found[0].Kind != "gear" || c.Found[0].Name != "Spear" || c.Found[0].Status != auditUnreviewedStatus {
		t.Errorf("found %+v", c.Found)
	}
}
