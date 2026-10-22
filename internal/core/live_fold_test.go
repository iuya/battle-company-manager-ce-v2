package core

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Company 1: unit 1 (Hero, Fate 2, Wounds 3, Might 3, Will 3, mounted), unit 2 (Warrior, Wounds 1). Company 2: unit 10 (Hero, Wounds 3, Fate 1),
// unit 11 (Warrior), unit 12 (monster, mounted with 2 mount wounds), unit 13 absent.
func liveInits() []LiveUnitInit {
	return []LiveUnitInit{
		{UnitID: 1, CompanyID: 1, Wounds: 3, Might: 3, Will: 3, Fate: 2, HasMount: true},
		{UnitID: 2, CompanyID: 1, Wounds: 1},
		{UnitID: 10, CompanyID: 2, Wounds: 3, Might: 2, Will: 2, Fate: 1, CountsAsKill: true},
		{UnitID: 11, CompanyID: 2, Wounds: 1},
		{UnitID: 12, CompanyID: 2, Wounds: 6, HasMount: true, MountWounds: 2, CountsAsKill: true},
		{UnitID: 13, CompanyID: 2, Wounds: 1, Absent: true},
	}
}

// play applies actions one after another through Check, like the service.
func play(t *testing.T, steps ...struct {
	a  LiveAction
	by int64
}) ([]LiveEntry, LiveState) {
	t.Helper()
	var entries []LiveEntry
	s := FoldLive(liveInits(), nil)
	for i, st := range steps {
		res, touches, text, err := s.Check(st.a, st.by)
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		entries = append(entries, LiveEntry{ID: int64(i + 1), Seq: int64(i + 1), CompanyID: st.by, UnitID: st.a.UnitID, Action: res, Touches: touches, Text: text})
		s = FoldLive(liveInits(), entries)
	}
	return entries, s
}

type step = struct {
	a  LiveAction
	by int64
}

func wound(attacker int64, by int64, targets ...WoundTarget) step {
	return step{LiveAction{Kind: LiveWound, UnitID: attacker, Targets: targets}, by}
}

func unit(t *testing.T, s LiveState, id int64) LiveUnit {
	t.Helper()
	u, ok := s.Unit(id)
	if !ok {
		t.Fatalf("no unit %d", id)
	}
	return u
}

func TestFoldStatsSetAndZones(t *testing.T) {
	set := 1
	_, s := play(t,
		step{LiveAction{Kind: LiveStatChange, UnitID: 1, Stat: "wounds", Delta: -1}, 1},
		step{LiveAction{Kind: LiveStatChange, UnitID: 1, Stat: "might", Set: &set}, 1}, // 3 -> 1 is stored as -2
	)
	u := unit(t, s, 1)
	if u.Wounds.Current != 2 || u.Might.Current != 1 || u.Zone != ZoneField {
		t.Errorf("%+v", u)
	}
	// falling to 0 Wounds by hand is a casualty; + brings it back
	zero := 0
	entries, s := play(t, step{LiveAction{Kind: LiveStatChange, UnitID: 2, Stat: "wounds", Set: &zero}, 1})
	if u := unit(t, s, 2); u.Zone != ZoneCasualty || u.Reason != "Wounds lowered to 0" {
		t.Errorf("manual fall: %+v", u)
	}
	if entries[0].Action.Set != nil || entries[0].Action.Delta != -1 {
		t.Errorf("a set is stored as its delta: %+v", entries[0].Action)
	}
	s = FoldLive(liveInits(), append(entries, LiveEntry{CompanyID: 1, Action: LiveAction{Kind: LiveStatChange, UnitID: 2, Stat: "wounds", Delta: 1}}))
	if u := unit(t, s, 2); u.Zone != ZoneField {
		t.Errorf("Wounds +1 brings the unit back: %+v", u)
	}
}

func TestCheckRefusesImpossibleStates(t *testing.T) {
	s := FoldLive(liveInits(), nil)
	over := 9
	for name, tc := range map[string]struct {
		a   LiveAction
		by  int64
		err error
	}{
		"above start":       {LiveAction{Kind: LiveStatChange, UnitID: 1, Stat: "might", Delta: 1}, 1, ErrRefused},
		"set above start":   {LiveAction{Kind: LiveStatChange, UnitID: 1, Stat: "might", Set: &over}, 1, ErrRefused},
		"below zero":        {LiveAction{Kind: LiveStatChange, UnitID: 2, Stat: "wounds", Delta: -2}, 1, ErrRefused},
		"zero delta":        {LiveAction{Kind: LiveStatChange, UnitID: 1, Stat: "might"}, 1, ErrInvalidLive},
		"unknown stat":      {LiveAction{Kind: LiveStatChange, UnitID: 1, Stat: "luck", Delta: 1}, 1, ErrInvalidLive},
		"fate at 0":         {LiveAction{Kind: LiveFate, UnitID: 2, Count: 1}, 1, ErrRefused},
		"fate over current": {LiveAction{Kind: LiveFate, UnitID: 1, Count: 3}, 1, ErrRefused},
		"fate count 0":      {LiveAction{Kind: LiveFate, UnitID: 1}, 1, ErrInvalidLive},
		"absent unit":       {LiveAction{Kind: LiveBroken, UnitID: 13}, 2, ErrRefused},
		"unknown unit":      {LiveAction{Kind: LiveBroken, UnitID: 99}, 1, ErrRefused},
		"other company":     {LiveAction{Kind: LiveBroken, UnitID: 10}, 1, ErrRefused},
		"no mount":          {LiveAction{Kind: LiveMount, UnitID: 2, Mounted: false}, 1, ErrRefused},
		"mount already on":  {LiveAction{Kind: LiveMount, UnitID: 1, Mounted: true}, 1, ErrRefused},
		"retreat no why":    {LiveAction{Kind: LiveRetreat, UnitID: 1}, 1, ErrInvalidLive},
		"long note":         {LiveAction{Kind: LiveDied, UnitID: 1, Note: strings.Repeat("x", 201)}, 1, ErrInvalidLive},
		"return in field":   {LiveAction{Kind: LiveReturn, UnitID: 1}, 1, ErrRefused},
		"no targets":        {LiveAction{Kind: LiveWound, UnitID: 1}, 1, ErrInvalidLive},
		"self target":       {LiveAction{Kind: LiveWound, UnitID: 1, Targets: []WoundTarget{{UnitID: 1, Caused: 1}}}, 1, ErrInvalidLive},
		"caused 0":          {LiveAction{Kind: LiveWound, UnitID: 1, Targets: []WoundTarget{{UnitID: 10, Caused: 0}}}, 1, ErrInvalidLive},
		"caused 10":         {LiveAction{Kind: LiveWound, UnitID: 1, Targets: []WoundTarget{{UnitID: 10, Caused: 10}}}, 1, ErrInvalidLive},
		"saved over fate":   {LiveAction{Kind: LiveWound, UnitID: 1, Targets: []WoundTarget{{UnitID: 10, Caused: 2, SavedByFate: 2}}}, 1, ErrInvalidLive},
		"bad part":          {LiveAction{Kind: LiveWound, UnitID: 1, Targets: []WoundTarget{{UnitID: 10, Caused: 1, Part: "tail"}}}, 1, ErrInvalidLive},
		"mount of foot":     {LiveAction{Kind: LiveWound, UnitID: 1, Targets: []WoundTarget{{UnitID: 10, Caused: 1, Part: "mount"}}}, 1, ErrRefused},
		"fate saves mount":  {LiveAction{Kind: LiveWound, UnitID: 1, Targets: []WoundTarget{{UnitID: 12, Caused: 1, Part: "mount", SavedByFate: 1}}}, 1, ErrInvalidLive},
		"absent target":     {LiveAction{Kind: LiveWound, UnitID: 1, Targets: []WoundTarget{{UnitID: 13, Caused: 1}}}, 1, ErrRefused},
		"duplicate target":  {LiveAction{Kind: LiveWound, UnitID: 1, Targets: []WoundTarget{{UnitID: 10, Caused: 1}, {UnitID: 10, Caused: 1}}}, 1, ErrInvalidLive},
		"unknown kind":      {LiveAction{Kind: "dance", UnitID: 1}, 1, ErrInvalidLive},
	} {
		if _, _, _, err := s.Check(tc.a, tc.by); !errors.Is(err, tc.err) {
			t.Errorf("%s: got %v, want %v", name, err, tc.err)
		}
	}
	// the state is not changed by checking
	before := FoldLive(liveInits(), nil)
	if !reflect.DeepEqual(unit(t, s, 1), unit(t, before, 1)) {
		t.Error("Check changed the state")
	}
}

func TestWoundDeclarations(t *testing.T) {
	entries, s := play(t,
		wound(1, 1, WoundTarget{UnitID: 10, Caused: 2, SavedByFate: 1}), // Hero 10: Wounds 3 -> 2, Fate 1 -> 0, fatePrevented
	)
	target, attacker := unit(t, s, 10), unit(t, s, 1)
	if target.Wounds.Current != 2 || target.Fate.Current != 0 || target.Gains[GainFatePrevented] != 1 {
		t.Errorf("target: %+v", target)
	}
	if attacker.Gains[GainWoundedEnemy] != 1 {
		t.Errorf("attacker: %+v", attacker)
	}
	if !reflect.DeepEqual(entries[0].Touches, []int64{2}) || !strings.Contains(entries[0].Text, "wounded") || !strings.Contains(entries[0].Text, "saved by Fate") {
		t.Errorf("touches/text: %v %q", entries[0].Touches, entries[0].Text)
	}

	// all saved still gives the wounding flag
	_, s = play(t, wound(1, 1, WoundTarget{UnitID: 10, Caused: 1, SavedByFate: 1}))
	if u := unit(t, s, 1); u.Gains[GainWoundedEnemy] != 1 || unit(t, s, 10).Wounds.Current != 3 {
		t.Errorf("all saved: %+v", u)
	}

	// the flag is once; kills of Heroes and Monsters count each, a Warrior kill nothing
	_, s = play(t,
		wound(1, 1, WoundTarget{UnitID: 11, Caused: 1}), // a Warrior falls: no kill xp
		wound(1, 1, WoundTarget{UnitID: 10, Caused: 3}), // a Hero falls
		wound(1, 1, WoundTarget{UnitID: 12, Caused: 6}), // a Monster falls
	)
	a := unit(t, s, 1)
	if a.Gains[GainWoundedEnemy] != 1 || a.Gains[GainSlainHeroMons] != 2 {
		t.Errorf("gains: %+v", a.Gains)
	}
	if u := unit(t, s, 11); u.Zone != ZoneCasualty || !strings.HasPrefix(u.Reason, "Wounded by ") {
		t.Errorf("casualty: %+v", u)
	}
	// a fallen unit cannot be targeted or act
	if _, _, _, err := s.Check(LiveAction{Kind: LiveWound, UnitID: 1, Targets: []WoundTarget{{UnitID: 11, Caused: 1}}}, 1); !errors.Is(err, ErrRefused) {
		t.Errorf("target out of the Field: %v", err)
	}
	if _, _, _, err := s.Check(LiveAction{Kind: LiveBroken, UnitID: 10}, 2); !errors.Is(err, ErrRefused) {
		t.Errorf("casualty acting: %v", err)
	}

	// names in the reason
	s.UnitName = func(id int64) string { return map[int64]string{1: "Aldor"}[id] }
	s.CompanyName = func(id int64) string { return "Rohan" }
	if u := unit(t, s, 11); u.Reason != "Wounded by Aldor (Rohan)" {
		t.Errorf("reason: %q", u.Reason)
	}
}

func TestMountWounds(t *testing.T) {
	_, s := play(t, wound(1, 1, WoundTarget{UnitID: 12, Part: "mount", Caused: 1}))
	if u := unit(t, s, 12); u.MountWounds.Current != 1 || !u.Mounted || u.Wounds.Current != 6 {
		t.Errorf("one mount wound: %+v", u)
	}
	_, s = play(t, wound(1, 1, WoundTarget{UnitID: 12, Part: "mount", Caused: 2}))
	u := unit(t, s, 12)
	if u.Mounted || u.Zone != ZoneField || unit(t, s, 1).Gains[GainWoundedEnemy] != 1 || unit(t, s, 1).Gains[GainSlainHeroMons] != 0 {
		t.Errorf("the mount falls, the rider stays: %+v", u)
	}
}

func TestFateBrokenRetreatDiedMountReturn(t *testing.T) {
	_, s := play(t,
		step{LiveAction{Kind: LiveFate, UnitID: 1, Count: 1}, 1},
		step{LiveAction{Kind: LiveBroken, UnitID: 1}, 1},
		step{LiveAction{Kind: LiveMount, UnitID: 1, Mounted: false}, 1},
	)
	u := unit(t, s, 1)
	if u.Fate.Current != 1 || u.Gains[GainFatePrevented] != 1 || u.Gains[GainBrokenPassed] != 1 || u.Mounted || !u.HasMount {
		t.Errorf("%+v", u)
	}
	_, s = play(t, step{LiveAction{Kind: LiveMount, UnitID: 1, Mounted: false}, 1}, step{LiveAction{Kind: LiveMount, UnitID: 1, Mounted: true}, 1})
	if !unit(t, s, 1).Mounted {
		t.Error("mount re-added")
	}

	for _, tc := range []struct {
		a      LiveAction
		zone   Zone
		reason string
	}{
		{LiveAction{Kind: LiveRetreat, UnitID: 2, Why: "courage"}, ZoneRetreated, "Failed courage"},
		{LiveAction{Kind: LiveRetreat, UnitID: 2, Why: "left"}, ZoneRetreated, "Left the board"},
		{LiveAction{Kind: LiveDied, UnitID: 2, Note: "fell off a cliff"}, ZoneCasualty, "Died: fell off a cliff"},
	} {
		entries, s := play(t, step{tc.a, 1})
		if u := unit(t, s, 2); u.Zone != tc.zone || u.Reason != tc.reason {
			t.Errorf("%v: %+v", tc.a.Kind, u)
		}
		// cannot act, then returns; a return lifts Wounds 0 to 1
		if _, _, _, err := s.Check(LiveAction{Kind: LiveBroken, UnitID: 2}, 1); !errors.Is(err, ErrRefused) {
			t.Errorf("acting outside the field: %v", err)
		}
		res, _, _, err := s.Check(LiveAction{Kind: LiveReturn, UnitID: 2}, 1)
		if err != nil {
			t.Fatal(err)
		}
		s = FoldLive(liveInits(), append(entries, LiveEntry{CompanyID: 1, Action: res}))
		if u := unit(t, s, 2); u.Zone != ZoneField {
			t.Errorf("returned: %+v", u)
		}
	}
	// a casualty by wounds returns with 1 Wound; a retreated unit with Wounds 0 is Retreated (zone priority)
	entries, s := play(t, wound(1, 1, WoundTarget{UnitID: 11, Caused: 1}))
	res, _, _, _ := s.Check(LiveAction{Kind: LiveReturn, UnitID: 11}, 2)
	s = FoldLive(liveInits(), append(entries, LiveEntry{CompanyID: 2, Action: res}))
	if u := unit(t, s, 11); u.Zone != ZoneField || u.Wounds.Current != 1 {
		t.Errorf("return from a wound casualty: %+v", u)
	}
	s = FoldLive(liveInits(), []LiveEntry{
		{CompanyID: 1, Action: LiveAction{Kind: LiveStatChange, UnitID: 2, Stat: "wounds", Delta: -1}},
		{CompanyID: 1, Action: LiveAction{Kind: LiveRetreat, UnitID: 2, Why: "left"}},
	})
	if u := unit(t, s, 2); u.Zone != ZoneRetreated {
		t.Errorf("zone priority: %+v", u)
	}
}

func TestCompanyListsPresentUnitsOnly(t *testing.T) {
	s := FoldLive(liveInits(), nil)
	if got := s.Company(2); len(got) != 3 {
		t.Errorf("absent units are not listed: %+v", got)
	}
	if c, ok := s.CompanyOf(10); !ok || c != 2 {
		t.Error("CompanyOf")
	}
}

func TestValidateLogFindsDependentEntries(t *testing.T) {
	// Fate 2: spend 1, +1 by hand (needs the spend before it), spend 2
	logs := []LiveEntry{
		{ID: 1, CompanyID: 1, Text: "spent", Action: LiveAction{Kind: LiveFate, UnitID: 1, Count: 1}},
		{ID: 2, CompanyID: 1, Text: "plus", Action: LiveAction{Kind: LiveStatChange, UnitID: 1, Stat: "fate", Delta: 1}},
		{ID: 3, CompanyID: 1, Text: "spent two", Action: LiveAction{Kind: LiveFate, UnitID: 1, Count: 2}},
	}
	if err := ValidateLog(liveInits(), logs); err != nil {
		t.Fatalf("valid log: %v", err)
	}
	if err := ValidateLog(liveInits(), logs[1:]); !errors.Is(err, ErrLiveDepends) || !errors.Is(err, ErrRefused) {
		t.Errorf("removing the first spend breaks the +1: %v", err)
	}
	// removing a wound with later independent entries is fine and restores state
	entries, _ := play(t,
		wound(1, 1, WoundTarget{UnitID: 10, Caused: 2, SavedByFate: 1}),
		step{LiveAction{Kind: LiveBroken, UnitID: 2}, 1},
	)
	if err := ValidateLog(liveInits(), entries[1:]); err != nil {
		t.Errorf("independent entries stay valid: %v", err)
	}
	s := FoldLive(liveInits(), entries[1:])
	if u := unit(t, s, 10); u.Wounds.Current != 3 || u.Fate.Current != 1 || u.Gains[GainFatePrevented] != 0 || unit(t, s, 1).Gains[GainWoundedEnemy] != 0 {
		t.Errorf("undo restores wounds, fate and flags: %+v", u)
	}
	// removing a retreat that a return follows
	entries, _ = play(t,
		step{LiveAction{Kind: LiveRetreat, UnitID: 2, Why: "left"}, 1},
		step{LiveAction{Kind: LiveReturn, UnitID: 2}, 1},
	)
	if err := ValidateLog(liveInits(), entries[1:]); !errors.Is(err, ErrLiveDepends) {
		t.Errorf("the return needs the retreat: %v", err)
	}
}

func TestFoldIsDeterministicAndPure(t *testing.T) {
	entries, a := play(t, wound(1, 1, WoundTarget{UnitID: 10, Caused: 2}), step{LiveAction{Kind: LiveBroken, UnitID: 1}, 1})
	inits := liveInits()
	b := FoldLive(inits, entries)
	if !reflect.DeepEqual(unit(t, a, 10), unit(t, b, 10)) || !reflect.DeepEqual(inits, liveInits()) {
		t.Error("fold differs or mutated its input")
	}
}

func TestCanUndoEntryAndLiveAccess(t *testing.T) {
	e := LiveEntry{CompanyID: 1, Touches: []int64{2}}
	owners := map[int64]int64{1: 100, 2: 200, 3: 300}
	for name, tc := range map[string]struct {
		u    User
		want bool
	}{
		"author":       {User{ID: 100}, true},
		"touched":      {User{ID: 200}, true},
		"third player": {User{ID: 300}, false},
		"organiser":    {User{ID: 400}, true},
		"admin":        {User{ID: 500, Role: RoleAdmin}, true},
	} {
		if got := CanUndoEntry(tc.u, e, owners, 400); got != tc.want {
			t.Errorf("%s: %v", name, got)
		}
	}
	m := Match{}
	if LiveAccessFor(User{ID: 100}, m, CampaignCompany{OwnerID: 100}, 400) != LiveEdit || LiveAccessFor(User{ID: 300}, m, CampaignCompany{OwnerID: 100}, 400) != LiveNone {
		t.Error("access")
	}
}

func vote(voter int64, company, unit int64) LiveEntry {
	return LiveEntry{CompanyID: company, ByUserID: voter, By: "Frodo", Action: LiveAction{Kind: LiveMVPVote, TargetCompanyID: company, UnitID: unit}}
}

func TestObjectiveFlags(t *testing.T) {
	_, s := play(t,
		step{LiveAction{Kind: LiveObjective, UnitID: 1, On: true}, 1},
		step{LiveAction{Kind: LiveObjective, UnitID: 2, On: true}, 1},
	)
	if !s.HasObjective(1) || !s.HasObjective(2) || s.HasObjective(10) {
		t.Error("several units of a company can have the objective")
	}
	for name, tc := range map[string]struct {
		a   LiveAction
		by  int64
		err error
	}{
		"again":     {LiveAction{Kind: LiveObjective, UnitID: 1, On: true}, 1, ErrRefused},
		"never had": {LiveAction{Kind: LiveObjective, UnitID: 10, On: false}, 2, ErrRefused},
		"absent":    {LiveAction{Kind: LiveObjective, UnitID: 13, On: true}, 2, ErrRefused},
		"not yours": {LiveAction{Kind: LiveObjective, UnitID: 10, On: true}, 1, ErrRefused},
	} {
		if _, _, _, err := s.Check(tc.a, tc.by); !errors.Is(err, tc.err) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// switching off is its own entry; a casualty cannot have it, a retreated unit can (it may have left by the board edge)
	_, s = play(t, step{LiveAction{Kind: LiveRetreat, UnitID: 2, Why: "left"}, 1}, step{LiveAction{Kind: LiveObjective, UnitID: 2, On: true}, 1})
	if !s.HasObjective(2) {
		t.Error("retreated units may have the objective")
	}
	_, s = play(t, step{LiveAction{Kind: LiveDied, UnitID: 2}, 1})
	if _, _, _, err := s.Check(LiveAction{Kind: LiveObjective, UnitID: 2, On: true}, 1); !errors.Is(err, ErrRefused) {
		t.Errorf("a casualty: %v", err)
	}
	// undoing the retreat that the objective entry did not need is fine; ValidateLog agrees
	entries, _ := play(t, step{LiveAction{Kind: LiveObjective, UnitID: 1, On: true}, 1}, step{LiveAction{Kind: LiveObjective, UnitID: 1, On: false}, 1})
	if err := ValidateLog(liveInits(), entries[1:]); !errors.Is(err, ErrLiveDepends) {
		t.Errorf("switching off needs the switch on: %v", err)
	}
}

func TestMVPVotesTallyAndUndo(t *testing.T) {
	check := func(s LiveState, entries []LiveEntry) []LiveEntry {
		return entries
	}
	_ = check
	// two players of company 1's team vote on company 2: voters 100 and 101
	log := []LiveEntry{vote(100, 2, 10), vote(101, 2, 10), vote(100, 2, 11)} // 100 changes its mind
	s := FoldLive(liveInits(), log)
	if s.Vote(100, 2) != 11 || s.Vote(101, 2) != 10 {
		t.Errorf("the last vote of a voter counts: %d %d", s.Vote(100, 2), s.Vote(101, 2))
	}
	counts, winner, tied := s.MVPTally(2)
	if winner != 0 || len(tied) != 2 || len(counts) != 2 || counts[0].Votes != 1 {
		t.Errorf("a tie has no winner: %+v %d %v", counts, winner, tied)
	}
	// undo of the new vote restores the earlier one: 100 -> 10, so 10 has two votes
	s = FoldLive(liveInits(), log[:2])
	if _, winner, _ := s.MVPTally(2); winner != 10 {
		t.Errorf("winner %d", winner)
	}
	// withdraw
	s = FoldLive(liveInits(), append(log[:2:2], vote(100, 2, 0)))
	if s.Vote(100, 2) != 0 {
		t.Error("withdrawn")
	}
	if _, w, tied := FoldLive(liveInits(), nil).MVPTally(2); w != 0 || tied != nil {
		t.Error("no votes, no MVP")
	}

	base := FoldLive(liveInits(), nil)
	for name, tc := range map[string]struct {
		a   LiveAction
		err error
	}{
		"unit of another company": {LiveAction{Kind: LiveMVPVote, TargetCompanyID: 2, UnitID: 1}, ErrInvalidLive},
		"absent unit":             {LiveAction{Kind: LiveMVPVote, TargetCompanyID: 2, UnitID: 13}, ErrRefused},
		"withdraw without vote":   {LiveAction{Kind: LiveMVPVote, TargetCompanyID: 2}, ErrRefused},
		"wrong target":            {LiveAction{Kind: LiveMVPVote, TargetCompanyID: 1, UnitID: 10}, ErrInvalidLive},
		"unknown company":         {LiveAction{Kind: LiveMVPVote, TargetCompanyID: 9, UnitID: 10}, ErrRefused},
	} {
		by := int64(2)
		if name == "unknown company" {
			by = 9
		}
		if _, _, _, err := base.CheckAs(tc.a, by, LiveVoter{ID: 100, Name: "Frodo"}); !errors.Is(err, tc.err) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// a casualty may still be voted for (it took part)
	cas := FoldLive(liveInits(), []LiveEntry{{CompanyID: 2, Action: LiveAction{Kind: LiveDied, UnitID: 11}}})
	if _, _, text, err := cas.CheckAs(LiveAction{Kind: LiveMVPVote, TargetCompanyID: 2, UnitID: 11}, 2, LiveVoter{ID: 100, Name: "Frodo"}); err != nil || !strings.Contains(text, "voted") {
		t.Errorf("casualty vote: %q %v", text, err)
	}
	// undoing a first vote that a withdrawal needs is refused
	if err := ValidateLog(liveInits(), []LiveEntry{vote(100, 2, 0)}); !errors.Is(err, ErrLiveDepends) {
		t.Errorf("withdraw log: %v", err)
	}
	// only the voter, an administrator or the organiser undo a vote
	e := vote(100, 2, 10)
	owners := map[int64]int64{2: 20}
	for user, want := range map[int64]bool{100: true, 20: false, 99: true} {
		if got := CanUndoEntry(User{ID: user}, e, owners, 99); got != want {
			t.Errorf("user %d: %v", user, got)
		}
	}
}
