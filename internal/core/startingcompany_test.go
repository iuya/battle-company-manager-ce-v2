package core

import (
	"errors"
	"slices"
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

// OfficerLocks is the one source of truth the creation page uses: every choice the page allows (the locks respected) passes
// ValidateStart, and every lock is enforced by it, for every company.
func TestOfficerLocksAgreeWithValidateStartInEveryCompany(t *testing.T) {
	rs := shippedRules(t)
	for _, p := range rs.BC.Profiles() {
		t.Run(p.Name, func(t *testing.T) {
			list, locks := p.StartList(), p.OfficerLocks()
			if len(locks) != len(list) {
				t.Fatalf("%d locks for %d units", len(locks), len(list))
			}
			for i, l := range locks {
				if (l.Kind == OfficerFree) != (l.Reason == "") {
					t.Errorf("unit %d (%s): kind %q with reason %q", i, list[i].Unit, l.Kind, l.Reason)
				}
			}
			// the company's suggestion respects the locks
			if _, err := p.ValidateStart(nil); err != nil {
				t.Errorf("the suggested officers are refused: %v", err)
			}
			// every placement the page allows passes; every one it blocks is refused with the matching message
			var cand []int
			for i, l := range locks {
				if l.Kind != OfficerBarred {
					cand = append(cand, i)
				}
			}
			allowed := func(h []StartHero) bool { // what the page's rules allow
				for i, l := range locks {
					at := slices.IndexFunc(h, func(x StartHero) bool { return x.Index == i })
					switch {
					case l.Kind == OfficerLeader && (at < 0 || h[at].Role != RoleLeader):
						return false
					case l.Kind == OfficerAny && at < 0:
						return false
					}
				}
				return true
			}
			var n int
			for _, l := range cand {
				for a := 0; a < len(cand); a++ {
					for b := a + 1; b < len(cand); b++ {
						if cand[a] == l || cand[b] == l {
							continue
						}
						h := []StartHero{{l, RoleLeader}, {cand[a], RoleSergeant}, {cand[b], RoleSergeant}}
						_, err := p.ValidateStart(h)
						if allowed(h) {
							n++
							if err != nil {
								t.Fatalf("officers %v are allowed by the locks but refused: %v", h, err)
							}
						} else if err == nil {
							t.Fatalf("officers %v break a lock but ValidateStart accepts them", h)
						}
					}
				}
			}
			if n == 0 {
				t.Error("the locks leave no valid choice of officers")
			}
			// a barred unit is refused wherever it is put
			for i, l := range locks {
				if l.Kind != OfficerBarred {
					continue
				}
				h := p.DefaultHeroes()
				h[0].Index = i
				if _, err := p.ValidateStart(h); err == nil {
					t.Errorf("%s is barred but accepted as an officer", list[i].Unit)
				}
			}
		})
	}
}

func TestOfficerLocksOfTheCompaniesWithRules(t *testing.T) {
	rs := shippedRules(t)
	kinds := func(name string) map[string]OfficerLockKind {
		p, ok := rs.BC.Profile(name)
		if !ok {
			t.Fatalf("no %s", name)
		}
		out := map[string]OfficerLockKind{}
		for i, l := range p.OfficerLocks() {
			if l.Kind != OfficerFree {
				out[p.StartList()[i].Unit] = l.Kind
			}
		}
		return out
	}
	if got := kinds("The Shire"); got["Ranger of the North"] != OfficerLeader {
		t.Errorf("Shire: %v", got)
	}
	if got := kinds("Helm's Deep"); got["Ranger of the North"] != OfficerAny {
		t.Errorf("Helm's Deep: %v", got)
	}
	n := 0
	for _, k := range kinds("Wanderers in the Wild") {
		if k == OfficerBarred {
			n++
		}
	}
	if n < 2 {
		t.Errorf("Wanderers: the two heroes in the making are barred")
	}
}
