package core

import (
	"reflect"
	"testing"
)

// companyFigures is what the lists and match pages rate a company by: it must equal the figures of the full BuildDetail for companies with
// heroes, injuries, creatures, wanderers and gear.
func TestCompanyFiguresEqualBuildDetail(t *testing.T) {
	rs := shippedRules(t)
	withPony := gondor()
	withPony.Units[0].Creature = "Baggage Pony"
	withPony.Units[4].Creature = "Baggage Pony"
	healed := mordor()
	healed.Units[2].SentToHealers = true
	empty := CompanyState{Company: CampaignCompany{ID: 3, Faction: "Mordor"}}
	unknown := CompanyState{Company: CampaignCompany{ID: 4, Faction: "No Such Company"}, Units: []UnitState{{ID: 1, Base: "Nobody", Role: RoleWarrior}}}
	for name, st := range map[string]CompanyState{"gondor": gondor(), "mordor": mordor(), "pony": withPony, "healers": healed, "empty": empty, "unknown": unknown} {
		for _, settings := range []CampaignSettings{{}, {CustomOnTheirOwnPath: true}} {
			want := BuildDetail(rs, settings, st).CampaignCompany
			if got := companyFigures(rs, settings, st); !reflect.DeepEqual(got, want) {
				t.Errorf("%s %+v: figures %+v, BuildDetail %+v", name, settings, got, want)
			}
		}
	}
}
