package lean

import (
	"reflect"
	"testing"
)

func validated(t *testing.T, extra string) Report {
	t.Helper()
	doc, ix, rep := resolved(t, extra, newFakeLower())
	Validate(doc, ix, newFakeLower(), NewReporter("f.json", &rep))
	return rep
}

func TestVersionIssue(t *testing.T) {
	i := VersionIssue("battle_companies.json", 1)
	want := "battle_companies.json: schemaVersion 1 is not supported (this app reads 2); the file must be regenerated or converted\n  hint: the Battle Companies file must say \"schemaVersion\": 2"
	if i.String() != want {
		t.Errorf("got\n%s", i)
	}
}

func TestValidateOfficers(t *testing.T) {
	company := func(heroes string) string {
		return `"companies":[{"name":"A","alignment":"good","description":"d",` + heroes + `
 "start":{"gold":0,"units":[{"unit":"Ranger of Arnor"},{"unit":"Knight of Arnor"}]},
 "reinforcements":{"influence":1,"rows":[{"roll":"1-6","unit":"Ranger of Arnor"}]}}]`
	}
	for _, c := range []struct {
		name, heroes string
		want         []string
	}{
		{"no restriction", ``, nil},
		{"one may lead", `"heroes":{"excluded":["Ranger of Arnor"]},`, nil},
		{"all excluded", `"heroes":{"excluded":["ranger of arnor","Knight of Arnor"]},`, []string{"companies[0].start: the company has no unit that may lead (heroes.excluded names every starting unit)"}},
		{"allowed names one", `"heroes":{"allowed":["Knight of Arnor"]},`, nil},
		{"allowed names none", `"heroes":{"allowed":["Cave Troll"]},`, []string{"companies[0].start: the company has no unit that may lead (heroes.allowed names none of the starting units)"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, i := range validated(t, company(c.heroes)).Issues {
				if i.Path == "companies[0].start" || i.Path == "" {
					got = append(got, i.Path+": "+i.Message)
				}
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestValidateCleanFixturesHaveNoWarnings(t *testing.T) {
	rep := validated(t, `"vanilla":{"pins":[{"kind":"hero","name":"Peregrin Took","occurrence":2}]}`)
	if len(rep.Issues) != 0 {
		t.Fatal(rep)
	}
}

func TestValidatePinKinds(t *testing.T) {
	rep := validated(t, `"vanilla":{"pins":[{"kind":"item","name":"Spear","occurrence":2},{"kind":"rule","name":"Fearless","occurrence":1},{"kind":"mount","name":"Warg","occurrence":1}]}`)
	got := issueLines(rep)
	want := []string{
		`vanilla.pins[0].occurrence: occurrence 2 does not exist for "Spear" in this vanilla file`,
		`vanilla.pins[2].occurrence: occurrence 1 does not exist for "Warg" in this vanilla file`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	for _, i := range rep.Issues {
		if i.Severity != Warn {
			t.Error("drift is a warning")
		}
	}
	if !rep.OK() {
		t.Error("warnings do not block")
	}
}
