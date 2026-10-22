package core

import (
	"errors"
	"strings"
	"testing"
)

func TestShireRangerStartsAsLeader(t *testing.T) {
	rs := shippedRules(t)
	shire, ok := rs.BC.Profile("The Shire")
	if !ok {
		t.Fatal("no Shire profile")
	}
	ranger := -1
	for i, s := range shire.StartList() {
		if s.Unit == "Ranger of the North" {
			ranger = i
		}
	}
	if ranger < 0 {
		t.Fatal("the Shire starts with a Ranger of the North")
	}
	other := func(skip ...int) int { // the first Hobbit that is not skipped
		for i := range shire.StartList() {
			if i != ranger && !contains(skip, i) && shire.CanLead(shire.StartList()[i].Unit) {
				return i
			}
		}
		t.Fatal("no other hero")
		return 0
	}
	a, b, c := other(), other(other()), other(other(), other(other()))
	tests := []struct {
		name    string
		heroes  []StartHero
		wantErr string
	}{
		{"default", nil, ""},
		{"ranger leader", []StartHero{{ranger, RoleLeader}, {a, RoleSergeant}, {b, RoleSergeant}}, ""},
		{"ranger demoted to sergeant", []StartHero{{a, RoleLeader}, {ranger, RoleSergeant}, {b, RoleSergeant}}, "must start as the Leader"},
		{"ranger not an officer", []StartHero{{a, RoleLeader}, {b, RoleSergeant}, {c, RoleSergeant}}, "must start as the Leader"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := shire.ValidateStart(tt.heroes)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidCompany) || err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want ErrInvalidCompany containing %q, got %v", tt.wantErr, err)
			}
		})
	}
	// Another company keeps its free choice of the leader.
	hd, _ := rs.BC.Profile("Minas Tirith")
	list := hd.StartList()
	var picks []StartHero
	for i, s := range list {
		if len(picks) < 3 && hd.CanLead(s.Unit) {
			role := RoleSergeant
			if len(picks) == 2 {
				role = RoleLeader
			}
			picks = append(picks, StartHero{i, role})
		}
	}
	if _, err := hd.ValidateStart(picks); err != nil {
		t.Errorf("a free company chooses its leader: %v", err)
	}
}

func contains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
