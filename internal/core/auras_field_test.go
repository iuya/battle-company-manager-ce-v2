package core

import (
	"reflect"
	"testing"
)

// TestBuiltUnitsCarryTheirAuras: a unit built into a company (the card's data) has the same auras as UnitAuras gives for it, so the chips of the
// cards and the rings of the dialog never disagree (docs/aura-ranges.md).
func TestBuiltUnitsCarryTheirAuras(t *testing.T) {
	rs := shippedRules(t)
	withAuras := 0
	for _, bcCo := range ttsCompanies(rs) {
		for _, e := range ttsEntries(rs, bcCo) {
			u := ttsOne(rs, bcCo, e).Roster[0]
			want := rs.UnitAuras(u)
			if !reflect.DeepEqual(u.Auras, want) {
				t.Fatalf("%s %s: card auras %v, UnitAuras %v", bcCo.Name, u.Type, u.Auras, want)
			}
			if len(want) > 0 {
				withAuras++
			}
		}
	}
	if withAuras == 0 {
		t.Error("no built unit of the 37 companies has an aura: the field is not filled")
	}
}
