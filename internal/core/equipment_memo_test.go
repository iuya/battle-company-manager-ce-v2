package core

import (
	"reflect"
	"testing"
)

// equipmentFor is remembered per rule set: the remembered answer equals a fresh build for every item of every company, and the flags and
// the company are part of the key.
func TestEquipmentForMemoEqualsFreshBuild(t *testing.T) {
	rs := shippedRules(t)
	names := []string{"Shield", "Bow", "Elven Glaive", "Horse", "Backpack", "Twin Blades", "No Such Item"}
	for _, co := range rs.BC.companies {
		for _, n := range names {
			for _, flags := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
				want, wg := buildEquipment(rs, co, n, flags[0], flags[1])
				for range 2 { // the first call stores, the second reads the memo
					got, gg := equipmentFor(rs, co, n, flags[0], flags[1], false)
					if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gg, wg) {
						t.Fatalf("%s / %s %v: memoised answer differs from a fresh build", co.Name, n, flags)
					}
				}
			}
		}
	}
}
