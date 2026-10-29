package core

import (
	"slices"
	"testing"
)

// The company effects of the Battle Companies file (companies[].effects) as BuildDetail and the recruit code read them. One
// table-driven test per kind; the kinds that only inform (ruleSubstitution, aura, breakPoint...) are shown, never enforced.

func warriors(base string, n int) []UnitState {
	units := make([]UnitState, n)
	for i := range units {
		units[i] = UnitState{ID: int64(i + 1), Base: base, Role: RoleWarrior}
	}
	return units
}

func limit(d CompanyDetail, id string) CompanyLimit {
	for _, l := range d.Limits {
		if l.ID == id {
			return l
		}
	}
	return CompanyLimit{}
}

func TestEffectMaxModels(t *testing.T) {
	rs := shippedRules(t)
	for faction, want := range map[string]int{"Minas Tirith": 15, "Rivendell": 12, "Wolves of Isengard": 12, "Lake-town": 20, "Moria": 20, "The Shire": 25, "Goblin-Town": 25} {
		d := BuildDetail(rs, CampaignSettings{}, CompanyState{Company: CampaignCompany{Faction: faction}})
		if d.MaxUnits != want {
			t.Errorf("%s: max models %d, want %d", faction, d.MaxUnits, want)
		}
	}
}

func TestEffectLimitOverrideRaisesTheShareOfTheCompany(t *testing.T) {
	rs := shippedRules(t)
	max := func(faction, base, id string) int {
		d := BuildDetail(rs, CampaignSettings{}, CompanyState{Company: CampaignCompany{Faction: faction}, Units: warriors(base, 12)})
		return limit(d, id).Max
	}
	usual := max("Minas Tirith", "Warrior of Minas Tirith", "bow")
	if usual == 0 {
		t.Fatal("the usual bow limit is a share of the roster")
	}
	if got := max("Osgiliath & Ithilien", "Warrior of Minas Tirith", "bow"); got <= usual {
		t.Errorf("Osgiliath & Ithilien bow limit %d, want more than the usual %d (50%%)", got, usual)
	}
	if got := max("Harad", "Harad Warrior", "bow"); got <= usual {
		t.Errorf("Harad bow limit %d, want more than the usual %d (50%%)", got, usual)
	}
}

func TestEffectLimitExemptionLeavesModelsOutOfTheCount(t *testing.T) {
	rs := shippedRules(t)
	riders := warriors("Rider of Rohan", 4)
	for i := range riders {
		riders[i].Bought = []string{"Bow"}
	}
	rohan := BuildDetail(rs, CampaignSettings{}, CompanyState{Company: CampaignCompany{Faction: "Rohan"}, Units: riders})
	if got := limit(rohan, "bow").Used; got != 0 {
		t.Errorf("Riders of Rohan do not count towards the bow limit in Rohan, used %d", got)
	}
	if got := limit(rohan, "cavalry").Used; got != 0 {
		t.Errorf("nor towards the cavalry limit, used %d", got)
	}
	other := BuildDetail(rs, CampaignSettings{}, CompanyState{Company: CampaignCompany{Faction: "Minas Tirith"}, Units: riders})
	if got := limit(other, "bow").Used; got == 0 {
		t.Error("the same riders count elsewhere")
	}
}

func TestEffectCompanyLimitKeepsAKeywordToItsShare(t *testing.T) {
	rs := shippedRules(t)
	profile, _ := rs.BC.Profile("Helm's Deep")
	if len(profile.KeywordLimits) != 1 || profile.KeywordLimits[0].Keyword != "Elf" || profile.KeywordLimits[0].Percent != 33 {
		t.Fatalf("Helm's Deep limits: %+v", profile.KeywordLimits)
	}
	bcCo, _ := rs.BC.Company("Helm's Deep")
	roster := BuildDetail(rs, CampaignSettings{}, CompanyState{Company: CampaignCompany{Faction: "Helm's Deep"}, Units: warriors("Galadhrim Warrior", 4)}).Roster
	why, kind := companyBlock(rs, bcCo, roster, "Galadhrim Warrior")
	if why == "" || kind != "limit" {
		t.Errorf("a fifth Elf is over a third of the company: %q %q", why, kind)
	}
}

func TestEffectOnTheirOwnPath(t *testing.T) {
	rs := shippedRules(t)
	son := []UnitState{{ID: 1, Base: "Son of Eorl", Role: RoleWarrior, XP: 7}}
	st := CompanyState{Company: CampaignCompany{Faction: "Rohan"}, Units: son}
	tests := []struct {
		name     string
		settings CampaignSettings
		role     UnitRole
		path     string
		xp       int
		fate     int
		ownPath  bool
	}{
		{"by default no experience, counted as a warrior", CampaignSettings{}, RoleWarrior, "On Their Own Path", 0, 0, true},
		{"the homebrew path: a warrior with real experience", CampaignSettings{CustomOnTheirOwnPath: true}, RoleWarrior, "", 7, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := BuildDetail(rs, tt.settings, st).Roster[0]
			if u.Role != tt.role || (u.Path == nil) != (tt.path == "") || (u.Path != nil && u.Path.Name != tt.path) || u.XP != tt.xp || u.OwnPath != tt.ownPath {
				t.Fatalf("role %s path %+v xp %d ownPath %v", u.Role, u.Path, u.XP, u.OwnPath)
			}
			for _, a := range u.Stats {
				if a.Abbr == "F" && a.HeroOnly && tt.role == RoleHero && a.Value != tt.fate {
					t.Errorf("Fate %d, want %d", a.Value, tt.fate)
				}
				if (a.Abbr == "M" || a.Abbr == "W") && a.HeroOnly && tt.role == RoleHero && a.Value != 0 {
					t.Errorf("%s %d, want 0", a.Abbr, a.Value)
				}
			}
		})
	}
	other := BuildDetail(rs, CampaignSettings{CustomOnTheirOwnPath: true}, CompanyState{Company: CampaignCompany{Faction: "Minas Tirith"}, Units: warriors("Warrior of Minas Tirith", 1)}).Roster[0]
	if other.Role != RoleWarrior || other.OwnPath {
		t.Errorf("a normal unit is not On Their Own Path: %+v", other)
	}
}

func TestEffectHeroUpgradesBelongToTheCompanyAndTheirSubgroup(t *testing.T) {
	rs := shippedRules(t)
	tests := []struct {
		faction string
		names   []string
	}{
		{"Minas Tirith", []string{"The Charge of Gondor"}},
		{"The Shire", []string{"Of a Party Sort", "Throwing Stones"}},
		{"Wolves of Isengard", []string{"Savage Charge", "Feral Charge"}},
	}
	for _, tt := range tests {
		p, _ := rs.BC.Profile(tt.faction)
		var got []string
		for _, h := range p.HeroUpgrades {
			got = append(got, h.Name)
			if h.Text == "" {
				t.Errorf("%s: upgrade %s has no rule text", tt.faction, h.Name)
			}
		}
		if !slices.Equal(got, tt.names) {
			t.Errorf("%s upgrades = %v, want %v", tt.faction, got, tt.names)
		}
	}
	wolves, _ := rs.BC.Profile("Wolves of Isengard")
	if wolves.HeroUpgrades[0].Restriction() != "Orc" || wolves.HeroUpgrades[1].Restriction() != "Warg" {
		t.Errorf("restriction subgroups: %+v", wolves.HeroUpgrades)
	}
}

func TestEffectRosterSlots(t *testing.T) {
	rs := shippedRules(t)
	d := BuildDetail(rs, CampaignSettings{}, CompanyState{Company: CampaignCompany{Faction: "Mirkwood"}, Units: warriors("Warg Marauder", 2)})
	if d.ModelCount() != 6 {
		t.Errorf("two Warg Marauders take 6 places of the roster, got %d", d.ModelCount())
	}
	if got := limit(d, "bow").Used; got != 2 {
		t.Errorf("but count once each towards the bow limit (Dark Union), got %d", got)
	}
}

func TestEffectStartingHeroNoStatGain(t *testing.T) {
	rs := shippedRules(t)
	for _, faction := range []string{"Helm's Deep", "The Shire"} {
		p, _ := rs.BC.Profile(faction)
		if p.StartGains("Ranger of the North") {
			t.Errorf("%s: a Ranger of the North keeps its own Might, Will and Fate", faction)
		}
		if !p.StartGains("Warrior of Rohan") {
			t.Errorf("%s: other units gain the usual point", faction)
		}
	}
}
