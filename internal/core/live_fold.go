package core

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// The experience gains the live mode records (campaign.experience.gains keys).
const (
	GainWoundedEnemy  = "woundedEnemy"
	GainSlainHeroMons = "slainHeroOrMonster"
	GainFatePrevented = "fatePrevented"
	GainBrokenPassed  = "brokenCourageTestPassed"
	GainObjective     = "objective"
	GainMVP           = "mvp"
)

// liveUnit is a LiveUnit plus what the fold needs to derive the zone.
type liveUnit struct {
	LiveUnit
	company      int64
	countsAsKill bool
	absent       bool
	retreat      string // reason while retreated
	died         string // reason while dead
	fellTo       int64  // the unit whose wound took it to 0 Wounds
}

// LiveState is the state of a match: a pure function of the units' starting values and the not-undone entries (FoldLive).
type LiveState struct {
	order     []int64 // unit ids, in the order of the inits
	units     map[int64]*liveUnit
	objective map[int64]bool           // units that got the objective
	votes     map[voteKey]int64        // the unit a voter (user) chose as MVP of a company; the last not-undone vote counts
	secrets   map[secretKey]SecretNote // what a company noted for each secret objective (the last not-undone entries count)
	cfg       LiveFoldConfig           // what the rosters and the rules file add (Break Point percent, models per unit)
	breaks    map[int64]*breakState    // by company: the manual corrections, Broken, the All is Lost answer
	m         *liveMeta                // the turn
	// UnitName and CompanyName turn ids into words for reasons and log texts; unset, ids are shown.
	UnitName    func(id int64) string
	CompanyName func(id int64) string
}

type voteKey struct{ voter, company int64 }

type secretKey struct {
	company   int64
	objective string
}

// SecretNote is what a company noted for one secret objective of the scenario (Fog of War): the unit or terrain piece it nominated
// at the start and the tier it reports at the end (1-based; 0 none yet).
type SecretNote struct {
	Nominee int64  // the nominated Hero, 0 for none or a terrain piece
	Terrain string // the terrain piece noted down
	Tier    int
}

// LiveVoter is who makes an mvpVote: the author of the entry.
type LiveVoter struct {
	ID   int64
	Name string
}

// FoldLive folds the entries (the not-undone ones, in Seq order) over the starting values. It does not validate: an entry that
// no longer fits is applied as far as it can be (counters stay within their range); ValidateLog is the check. It never mutates
// its input.
func FoldLive(inits []LiveUnitInit, entries []LiveEntry) LiveState {
	return FoldLiveWith(inits, entries, LiveFoldConfig{})
}

// FoldLiveWith is FoldLive with the Break Point settings of the companies (see LiveFoldConfig): without them no company ever breaks.
func FoldLiveWith(inits []LiveUnitInit, entries []LiveEntry, cfg LiveFoldConfig) LiveState {
	s := newLiveState(inits, cfg)
	for _, e := range entries {
		s.apply(e.Action, e.CompanyID, e.ByUserID)
		if u := s.units[e.Action.UnitID]; u != nil {
			switch e.Action.Kind {
			case LiveFate:
				u.FateEntry = e.ID
			case LiveBroken:
				u.BrokenEntry = e.ID
			}
		}
	}
	return s
}

func newLiveState(inits []LiveUnitInit, cfg LiveFoldConfig) LiveState {
	s := LiveState{units: map[int64]*liveUnit{}, objective: map[int64]bool{}, votes: map[voteKey]int64{}, secrets: map[secretKey]SecretNote{},
		cfg: cfg, breaks: map[int64]*breakState{}, m: &liveMeta{}}
	for _, in := range inits {
		s.breaks[in.CompanyID] = &breakState{}
		u := &liveUnit{company: in.CompanyID, countsAsKill: in.CountsAsKill, absent: in.Absent}
		u.UnitID, u.Zone, u.Temporary = in.UnitID, ZoneField, in.Temporary
		for _, l := range cfg.Loaned {
			if l.UnitID == in.UnitID {
				u.Loaned = append(u.Loaned, l)
			}
		}
		u.Wounds, u.Might, u.Will, u.Fate = LiveStat{in.Wounds, in.Wounds}, LiveStat{in.Might, in.Might}, LiveStat{in.Will, in.Will}, LiveStat{in.Fate, in.Fate}
		u.HasMount, u.Mounted = in.HasMount, in.HasMount
		if in.HasMount {
			n := max(1, in.MountWounds)
			u.MountWounds = LiveStat{n, n}
		}
		u.Gains = map[string]int{}
		u.BreakModels = max(1, cfg.BreakModels[in.UnitID])
		s.units[in.UnitID] = u
		s.order = append(s.order, in.UnitID)
	}
	return s
}

// zone derives where a unit is: Retreated, then Field casualties (died, or no Wounds left), then the Field.
func (u *liveUnit) zone() (Zone, string) {
	switch {
	case u.retreat != "":
		return ZoneRetreated, u.retreat
	case u.died != "":
		return ZoneCasualty, u.died
	case u.Wounds.Start > 0 && u.Wounds.Current == 0:
		if u.fellTo != 0 {
			return ZoneCasualty, ""
		}
		return ZoneCasualty, "Wounds lowered to 0"
	}
	return ZoneField, ""
}

// Unit returns a unit with its zone and reason worked out.
func (s LiveState) Unit(id int64) (LiveUnit, bool) {
	u, ok := s.units[id]
	if !ok {
		return LiveUnit{}, false
	}
	out := u.LiveUnit
	out.Zone, out.Reason = u.zone()
	if out.Zone == ZoneCasualty && out.Reason == "" {
		out.Reason = "Wounded by " + s.unitNameWithCompany(u.fellTo)
	}
	out.Gains = make(map[string]int, len(u.Gains))
	for k, v := range u.Gains {
		out.Gains[k] = v
	}
	out.Loaned, out.UsedItems = slices.Clone(u.Loaned), slices.Clone(u.UsedItems)
	out.Objective = s.objective[id]
	return out, true
}

// Company lists a company's present units (not absent ones), in the order of the inits.
func (s LiveState) Company(companyID int64) []LiveUnit {
	var out []LiveUnit
	for _, id := range s.order {
		if u := s.units[id]; u.company == companyID && !u.absent {
			lu, _ := s.Unit(id)
			out = append(out, lu)
		}
	}
	return out
}

// CompanyOf is the company a unit belongs to in the match.
func (s LiveState) CompanyOf(unitID int64) (int64, bool) {
	u, ok := s.units[unitID]
	if !ok {
		return 0, false
	}
	return u.company, true
}

func (s LiveState) name(id int64) string {
	if s.UnitName != nil {
		if n := s.UnitName(id); n != "" {
			return n
		}
	}
	return fmt.Sprintf("unit %d", id)
}

func (s LiveState) unitNameWithCompany(id int64) string {
	u, ok := s.units[id]
	if !ok {
		return s.name(id)
	}
	if s.CompanyName != nil {
		if c := s.CompanyName(u.company); c != "" {
			return fmt.Sprintf("%s (%s)", s.name(id), c)
		}
	}
	return s.name(id)
}

// clone copies the state so that Check can try an action without changing it.
func (s LiveState) clone() LiveState {
	c := LiveState{order: s.order, units: make(map[int64]*liveUnit, len(s.units)), objective: maps.Clone(s.objective), votes: maps.Clone(s.votes), secrets: maps.Clone(s.secrets), UnitName: s.UnitName, CompanyName: s.CompanyName,
		cfg: s.cfg, breaks: make(map[int64]*breakState, len(s.breaks)), m: &liveMeta{turn: s.m.turn}}
	for id, b := range s.breaks {
		cp := *b
		c.breaks[id] = &cp
	}
	for id, u := range s.units {
		cp := *u
		cp.Loaned, cp.UsedItems = slices.Clone(u.Loaned), slices.Clone(u.UsedItems)
		cp.Gains = make(map[string]int, len(u.Gains))
		for k, v := range u.Gains {
			cp.Gains[k] = v
		}
		c.units[id] = &cp
	}
	return c
}

// loanedIndex finds the loaned item (by name and copy), -1 when the unit has none such.
func (u *liveUnit) loanedIndex(item string, index int) int {
	return slices.IndexFunc(u.Loaned, func(l LoanedItem) bool { return sameName(l.Item, strings.TrimSpace(item)) && l.Index == index })
}

func (u *liveUnit) stat(name string) *LiveStat {
	switch strings.ToLower(name) {
	case "wounds":
		return &u.Wounds
	case "might":
		return &u.Might
	case "will":
		return &u.Will
	case "fate":
		return &u.Fate
	}
	return nil
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }

// apply changes the state by an action that has been resolved (deltas, no Set). It clamps instead of failing.
func (s LiveState) apply(a LiveAction, byCompany, voter int64) {
	if a.Kind.Informational() {
		return
	}
	if a.Kind.Secret() {
		k := secretKey{byCompany, a.ObjectiveKey}
		n := s.secrets[k]
		if a.Kind == LiveSecretNominate {
			n.Nominee, n.Terrain = a.UnitID, strings.TrimSpace(a.Note)
		} else {
			n.Tier = a.Tier
		}
		if n == (SecretNote{}) {
			delete(s.secrets, k)
		} else {
			s.secrets[k] = n
		}
		return
	}
	if a.Kind == LiveMVPVote {
		if a.UnitID == 0 {
			delete(s.votes, voteKey{voter, byCompany})
		} else {
			s.votes[voteKey{voter, byCompany}] = a.UnitID
		}
		return
	}
	switch a.Kind {
	case LiveTurn:
		s.startTurn(a.Turn)
		return
	case LiveBreakAdjust:
		if b := s.breaks[byCompany]; b != nil {
			if a.Field == "start" {
				b.adjStart += a.Delta
			} else if a.Field == "casualties" {
				b.adjCasualties += a.Delta
			}
		}
		return
	case LiveAllIsLost:
		if b := s.breaks[byCompany]; b != nil {
			b.decided = true
			if a.Flee {
				b.fled = true
				s.fleeBattlefield(byCompany)
			}
		}
		return
	}
	u := s.units[a.UnitID]
	if u == nil {
		return
	}
	switch a.Kind {
	case LiveCourageSkip:
		u.CourageTurn = s.m.turn
		u.CourageHow = a.Reason
	case LiveItemUsed:
		if i := u.loanedIndex(a.Item, a.Index); i >= 0 {
			u.Loaned[i].Used = true
		} else {
			u.UsedItems = append(u.UsedItems, a.Item)
		}
	case LiveStatChange:
		if st := u.stat(a.Stat); st != nil {
			before := u.Wounds.Current
			st.Current = clamp(st.Current+a.Delta, 0, st.Start)
			if strings.EqualFold(a.Stat, "wounds") && before > 0 && st.Current == 0 {
				u.fellTo = 0
			}
			if strings.EqualFold(a.Stat, "wounds") && st.Current > 0 {
				u.fellTo = 0
			}
		}
	case LiveWound:
		for _, t := range a.Targets {
			tu := s.units[t.UnitID]
			if tu == nil || t.Caused < 1 {
				continue
			}
			u.Gains[GainWoundedEnemy] = 1
			if t.Part == "mount" {
				tu.MountWounds.Current = clamp(tu.MountWounds.Current-t.Caused, 0, tu.MountWounds.Start)
				if tu.MountWounds.Current == 0 {
					tu.Mounted = false
				}
				continue
			}
			saved := min(t.SavedByFate, tu.Fate.Current, t.Caused)
			if saved > 0 {
				tu.Fate.Current -= saved
				tu.Gains[GainFatePrevented] = 1
			}
			before := tu.Wounds.Current
			tu.Wounds.Current = clamp(tu.Wounds.Current-(t.Caused-saved), 0, tu.Wounds.Start)
			if before > 0 && tu.Wounds.Current == 0 {
				tu.fellTo = u.UnitID
				if tu.countsAsKill {
					u.Gains[GainSlainHeroMons]++
				}
			}
		}
	case LiveFate:
		u.Fate.Current -= min(a.Count, u.Fate.Current) // Count 0 only marks the milestone: the Fate tracker is set by hand
		u.Gains[GainFatePrevented] = 1
	case LiveBroken:
		u.Gains[GainBrokenPassed] = 1
		u.CourageTurn = s.m.turn
		u.CourageHow = "passed"
		if a.StandFast {
			u.StandFastTurn = s.m.turn
		}
	case LiveRetreat:
		if a.Why == "courage" {
			u.CourageTurn = s.m.turn
			u.CourageHow = "failed"
			u.retreat = "Failed courage"
		} else {
			u.retreat = "Left the board"
		}
	case LiveDied:
		u.died = "Died"
		if n := strings.TrimSpace(a.Note); n != "" {
			u.died += ": " + n
		}
	case LiveMount:
		if a.Mounted && u.HasMount {
			u.Mounted = true
			u.MountWounds.Current = max(u.MountWounds.Current, 1)
		} else {
			u.Mounted = false
		}
	case LiveObjective:
		if a.On {
			s.objective[a.UnitID] = true
		} else {
			delete(s.objective, a.UnitID)
		}
	case LiveReturn:
		u.retreat, u.died, u.fellTo = "", "", 0
		if u.Wounds.Start > 0 && u.Wounds.Current == 0 {
			u.Wounds.Current = 1
		}
	}
}

func invalid(format string, args ...any) error {
	return errors.Join(ErrInvalidLive, fmt.Errorf(format, args...))
}

// Check validates an action against the state, for the company that acts. It returns the action resolved (a Set becomes its
// Delta), the other companies it touches and the log text. Bad input is ErrInvalidLive (joined, fit to show); a move the state does
// not allow is ErrRefused with the reason. The state is not changed.
func (s LiveState) Check(a LiveAction, byCompany int64) (resolved LiveAction, touches []int64, text string, err error) {
	return s.CheckAs(a, byCompany, LiveVoter{})
}

// CheckAs is Check for an action whose author matters: an mvpVote belongs to the voter.
func (s LiveState) CheckAs(a LiveAction, byCompany int64, voter LiveVoter) (resolved LiveAction, touches []int64, text string, err error) {
	if len(a.Note) > 200 {
		return a, nil, "", invalid("The note is at most 200 characters.")
	}
	if a.Kind.Informational() { // written by the app, never by a player
		return a, nil, "", invalid("That entry is written by the app.")
	}
	switch a.Kind {
	case LiveMVPVote:
		return s.checkVote(a, byCompany, voter)
	case LiveTurn:
		return s.checkTurn(a)
	case LiveBreakAdjust:
		return s.checkBreakAdjust(a, byCompany)
	case LiveAllIsLost:
		return s.checkAllIsLost(a, byCompany)
	}
	if a.Kind.Secret() {
		return s.checkSecret(a, byCompany, voter)
	}
	u, ok := s.units[a.UnitID]
	if !ok {
		return a, nil, "", Refuse("That unit is not in this match.")
	}
	if u.absent {
		return a, nil, "", Refuse("%s takes no part in this match.", s.name(a.UnitID))
	}
	if u.company != byCompany {
		return a, nil, "", Refuse("%s is not in that company.", s.name(a.UnitID))
	}
	zone, _ := u.zone()
	inField := func() error {
		if zone != ZoneField {
			return Refuse("%s is not in the battle.", s.name(a.UnitID))
		}
		return nil
	}
	who := s.name(a.UnitID)
	resolved = a
	resolved.Set = nil
	switch a.Kind {
	case LiveStatChange:
		st := u.stat(a.Stat)
		if st == nil {
			return a, nil, "", invalid("Unknown tracker %q.", a.Stat)
		}
		if zone == ZoneRetreated || (zone == ZoneCasualty && u.died != "") {
			return a, nil, "", Refuse("%s is not in the battle.", who)
		}
		if st.Start == 0 {
			return a, nil, "", Refuse("%s has no %s.", who, strings.ToLower(a.Stat))
		}
		delta := a.Delta
		if a.Set != nil {
			delta = *a.Set - st.Current
		}
		if delta == 0 {
			return a, nil, "", invalid("That changes nothing.")
		}
		if n := st.Current + delta; n < 0 || n > st.Start {
			return a, nil, "", Refuse("%s can only go from 0 to %d.", Capitalise(strings.ToLower(a.Stat)), st.Start)
		}
		resolved.Delta = delta
		text = fmt.Sprintf("%s: %s %+d (manual)", who, Capitalise(strings.ToLower(a.Stat)), delta)
	case LiveWound:
		if err := inField(); err != nil {
			return a, nil, "", err
		}
		if len(a.Targets) == 0 {
			return a, nil, "", invalid("Choose at least one unit that was wounded.")
		}
		var parts []string
		seen := map[int64]bool{}
		touched := map[int64]bool{}
		sim := s.clone()
		for _, t := range a.Targets {
			if t.UnitID == a.UnitID {
				return a, nil, "", invalid("A unit cannot wound itself.")
			}
			if seen[t.UnitID] {
				return a, nil, "", invalid("A unit can be wounded once per declaration.")
			}
			seen[t.UnitID] = true
			tu, ok := sim.units[t.UnitID]
			if !ok || tu.absent {
				return a, nil, "", Refuse("That unit is not in this match.")
			}
			if t.Caused < 1 || t.Caused > 9 {
				return a, nil, "", invalid("Wounds caused are from 1 to 9.")
			}
			if t.Part != "" && t.Part != "rider" && t.Part != "mount" {
				return a, nil, "", invalid("Unknown part %q.", t.Part)
			}
			if z, _ := tu.zone(); z != ZoneField {
				return a, nil, "", Refuse("%s is not in the battle.", s.name(t.UnitID))
			}
			name := s.name(t.UnitID)
			if t.Part == "mount" {
				if t.SavedByFate != 0 {
					return a, nil, "", invalid("Fate cannot save a mount.")
				}
				if !tu.HasMount || !tu.Mounted {
					return a, nil, "", Refuse("%s has no mount.", name)
				}
				parts = append(parts, fmt.Sprintf("%s's mount (%d)", name, t.Caused))
			} else {
				if t.SavedByFate < 0 || t.SavedByFate > min(t.Caused, tu.Fate.Current) {
					return a, nil, "", invalid("Fate can save from 0 to %d wounds of %s.", min(t.Caused, tu.Fate.Current), name)
				}
				desc := fmt.Sprintf("%s (%d wound%s", name, t.Caused, Plural(t.Caused, "", "s"))
				if t.SavedByFate > 0 {
					desc += fmt.Sprintf(", %d saved by Fate", t.SavedByFate)
				}
				desc += ")"
				if tu.Wounds.Current > 0 && tu.Wounds.Current-(t.Caused-t.SavedByFate) <= 0 {
					desc += " and it falls"
				}
				parts = append(parts, desc)
			}
			if tu.company != byCompany {
				touched[tu.company] = true
			}
			sim.apply(LiveAction{Kind: LiveWound, UnitID: a.UnitID, Targets: []WoundTarget{t}}, byCompany, 0)
		}
		for c := range touched {
			touches = append(touches, c)
		}
		slices.Sort(touches)
		text = fmt.Sprintf("%s wounded %s", who, strings.Join(parts, ", "))
	case LiveFate:
		if err := inField(); err != nil {
			return a, nil, "", err
		}
		if a.Count < 0 || a.Count > 9 {
			return a, nil, "", invalid("Fate points spent are from 0 to 9.")
		}
		if u.Fate.Current < a.Count {
			return a, nil, "", Refuse("%s has %d Fate left.", who, u.Fate.Current)
		}
		if a.Count == 0 {
			text = who + " passed a Fate roll"
		} else {
			text = fmt.Sprintf("%s spent Fate (%d left)", who, u.Fate.Current-a.Count)
		}
	case LiveBroken:
		if err := inField(); err != nil {
			return a, nil, "", err
		}
		if a.StandFast && s.cfg.NoStandFast[a.UnitID] {
			return a, nil, "", Refuse("%s cannot call Stand Fast.", who)
		}
		text = who + " passed a Broken courage test"
		if a.StandFast {
			text += " and called Stand Fast"
		}
	case LiveItemUsed:
		if err := inField(); err != nil {
			return a, nil, "", err
		}
		item := strings.TrimSpace(a.Item)
		if item == "" {
			return a, nil, "", invalid("Choose the item that was used.")
		}
		if i := u.loanedIndex(item, a.Index); i >= 0 {
			switch l := u.Loaned[i]; {
			case l.AfterGame || s.cfg.AfterGame[NameKey(item)]:
				return a, nil, "", Refuse("%s acts after the game: use it in the aftermath.", l.Item)
			case l.Used:
				return a, nil, "", Refuse("%s has already used its %s.", who, l.Item)
			}
			item = u.Loaned[i].Item
		} else {
			owned := slices.IndexFunc(s.cfg.Owned[a.UnitID], func(n string) bool { return sameName(n, item) })
			switch {
			case owned < 0:
				return a, nil, "", Refuse("%s does not have %s.", who, item)
			case s.cfg.AfterGame[NameKey(item)]:
				return a, nil, "", Refuse("%s acts after the game: use it in the aftermath.", item)
			case slices.ContainsFunc(u.UsedItems, func(n string) bool { return sameName(n, item) }):
				return a, nil, "", Refuse("%s has already used its %s.", who, item)
			}
			item = s.cfg.Owned[a.UnitID][owned]
			resolved.Index = 0
		}
		resolved.Item = item
		text = who + " used " + item
	case LiveCourageSkip:
		if err := inField(); err != nil {
			return a, nil, "", err
		}
		why, known := courageSkipReasons[a.Reason]
		if !known {
			return a, nil, "", invalid("Choose why %s needs no courage test.", who)
		}
		if courageExempt(a.Reason) {
			text = fmt.Sprintf("%s takes no courage test (%s)", who, why)
		} else {
			text = fmt.Sprintf("%s passes courage automatically (%s)", who, why)
		}
	case LiveRetreat:
		if err := inField(); err != nil {
			return a, nil, "", err
		}
		switch a.Why {
		case "courage":
			text = who + " retreated (failed courage)"
		case "left":
			text = who + " retreated (left the board)"
		default:
			return a, nil, "", invalid("Choose why %s retreated.", who)
		}
		if n := strings.TrimSpace(a.Note); n != "" {
			text += ": " + n
		}
	case LiveDied:
		if err := inField(); err != nil {
			return a, nil, "", err
		}
		text = who + " died"
		if n := strings.TrimSpace(a.Note); n != "" {
			text += ": " + n
		}
	case LiveMount:
		if err := inField(); err != nil {
			return a, nil, "", err
		}
		if !u.HasMount {
			return a, nil, "", Refuse("%s has no mount.", who)
		}
		if u.Mounted == a.Mounted {
			if a.Mounted {
				return a, nil, "", Refuse("%s already has its mount.", who)
			}
			return a, nil, "", Refuse("%s has already lost its mount.", who)
		}
		if a.Mounted {
			text = who + " has its mount back"
		} else {
			text = who + " lost its mount"
		}
	case LiveObjective:
		if zone != ZoneField && zone != ZoneRetreated {
			return a, nil, "", Refuse("%s cannot have the objective: it is out of the game.", who)
		}
		if s.objective[a.UnitID] == a.On {
			if a.On {
				return a, nil, "", Refuse("%s already has the objective.", who)
			}
			return a, nil, "", Refuse("%s does not have the objective.", who)
		}
		resolved.On = a.On
		if a.On {
			text = who + " got the objective"
		} else {
			text = who + " no longer has the objective"
		}
	case LiveReturn:
		if zone == ZoneField {
			return a, nil, "", Refuse("%s is already in the battle.", who)
		}
		text = who + " returned to the battle"
	default:
		return a, nil, "", invalid("Unknown action %q.", a.Kind)
	}
	return resolved, touches, text, nil
}

// checkVote validates an mvpVote for the company byCompany (the one voted on) by the voter. Who may vote is the service's rule
// (the voter owns a company on another team); the unit must be one of the company that took part in the match.
func (s LiveState) checkVote(a LiveAction, company int64, voter LiveVoter) (LiveAction, []int64, string, error) {
	if a.TargetCompanyID != company {
		return a, nil, "", invalid("The vote is for another company.")
	}
	known := false
	for _, u := range s.units {
		if u.company == company {
			known = true
			break
		}
	}
	if !known {
		return a, nil, "", Refuse("That company is not in this match.")
	}
	who := voter.Name
	if who == "" {
		who = "A player"
	}
	cname := "its company"
	if s.CompanyName != nil && s.CompanyName(company) != "" {
		cname = s.CompanyName(company)
	}
	if a.UnitID == 0 {
		if _, voted := s.votes[voteKey{voter.ID, company}]; !voted {
			return a, nil, "", Refuse("You have not voted for %s.", cname)
		}
		return a, nil, fmt.Sprintf("%s withdrew the MVP vote for %s", who, cname), nil
	}
	u, ok := s.units[a.UnitID]
	if !ok || u.company != company {
		return a, nil, "", invalid("That unit is not in %s.", cname)
	}
	if u.Temporary {
		return a, nil, "", Refuse("A temporary Wanderer cannot be the MVP.")
	}
	if u.absent {
		return a, nil, "", Refuse("%s took no part in this match.", s.name(a.UnitID))
	}
	return a, nil, fmt.Sprintf("%s voted %s as MVP of %s", who, s.name(a.UnitID), cname), nil
}

// checkSecret validates a secret objective entry structurally; that the objective is a secret one of the scenario, and who may be
// nominated, is the service's rule (it knows the scenario). The text is generic: only the author ever sees the entry while the
// match is open, and the service words it when it is revealed.
func (s LiveState) checkSecret(a LiveAction, company int64, voter LiveVoter) (LiveAction, []int64, string, error) {
	if a.ObjectiveKey == "" || len(a.ObjectiveKey) > 60 {
		return a, nil, "", invalid("Choose the secret objective.")
	}
	known := false
	for _, u := range s.units {
		known = known || u.company == company
	}
	if !known {
		return a, nil, "", Refuse("That company is not in this match.")
	}
	who := voter.Name
	if who == "" {
		who = "A player"
	}
	switch a.Kind {
	case LiveSecretNominate:
		if a.UnitID != 0 {
			u, ok := s.units[a.UnitID]
			if !ok || u.absent {
				return a, nil, "", Refuse("That unit takes no part in this match.")
			}
			if strings.TrimSpace(a.Note) != "" {
				return a, nil, "", invalid("Nominate a unit or a terrain piece, not both.")
			}
		}
		return a, nil, who + " noted a secret objective", nil
	default:
		if a.Tier < 0 || a.Tier > 9 {
			return a, nil, "", invalid("Choose the tier reached.")
		}
		return a, nil, who + " reported a secret objective", nil
	}
}

// SecretOf is what a company noted for a secret objective.
func (s LiveState) SecretOf(companyID int64, objective string) (SecretNote, bool) {
	n, ok := s.secrets[secretKey{companyID, objective}]
	return n, ok
}

// HasObjective says whether the unit got the objective.
func (s LiveState) HasObjective(unitID int64) bool { return s.objective[unitID] }

// Vote is the unit a voter chose as the MVP of a company, 0 for none.
func (s LiveState) Vote(voterID, companyID int64) int64 { return s.votes[voteKey{voterID, companyID}] }

// MVPCount is how many votes a unit has.
type MVPCount struct {
	UnitID int64
	Votes  int
}

// MVPTally counts the votes for a company's units, most votes first (ties by unit id). Winner is the unit with the most votes, 0
// on a tie or without votes; Tied lists the units sharing the top count when there is a tie.
func (s LiveState) MVPTally(companyID int64) (counts []MVPCount, winner int64, tied []int64) {
	n := map[int64]int{}
	for k, unit := range s.votes {
		if k.company == companyID {
			n[unit]++
		}
	}
	for unit, v := range n {
		counts = append(counts, MVPCount{unit, v})
	}
	slices.SortFunc(counts, func(a, b MVPCount) int {
		if a.Votes != b.Votes {
			return b.Votes - a.Votes
		}
		return int(a.UnitID - b.UnitID)
	})
	if len(counts) == 0 {
		return counts, 0, nil
	}
	for _, c := range counts {
		if c.Votes == counts[0].Votes {
			tied = append(tied, c.UnitID)
		}
	}
	if len(tied) == 1 {
		return counts, tied[0], nil
	}
	return counts, 0, tied
}

// ValidateLog checks that every entry is valid against the state before it, the way Apply checked it. It is the check of an undo: the
// first entry that no longer fits is ErrLiveDepends (naming it).
func ValidateLog(inits []LiveUnitInit, entries []LiveEntry) error {
	return ValidateLogWith(inits, entries, LiveFoldConfig{})
}

// ValidateLogWith is ValidateLog with the Break Point settings of FoldLiveWith.
func ValidateLogWith(inits []LiveUnitInit, entries []LiveEntry, cfg LiveFoldConfig) error {
	s := newLiveState(inits, cfg)
	for _, e := range entries {
		if e.Action.Kind.Informational() {
			continue
		}
		if _, _, _, err := s.CheckAs(e.Action, e.CompanyID, LiveVoter{ID: e.ByUserID, Name: e.By}); err != nil {
			text := e.Text
			if text == "" {
				text = string(e.Action.Kind)
			}
			return fmt.Errorf("%w (%s)", ErrLiveDepends, text)
		}
		s.apply(e.Action, e.CompanyID, e.ByUserID)
	}
	return nil
}

// CanUndoEntry says whether the user may undo an entry: the owner of the acting company or of a company it touched, the
// organiser and administrators. owners maps a company id to its owner's user id.
func CanUndoEntry(u User, e LiveEntry, owners map[int64]int64, organiserID int64) bool {
	if e.Action.Kind.Informational() { // the story of a pending end is kept
		return false
	}
	if e.Action.Kind.Secret() { // a secret objective is its author's alone while the match is open, even for the organiser
		return e.ByUserID != 0 && e.ByUserID == u.ID
	}
	if CanManageCampaign(u, organiserID) {
		return true
	}
	if e.Action.Kind == LiveTurn { // the turn belongs to the whole match: any player of it
		for _, owner := range owners {
			if owner != 0 && owner == u.ID {
				return true
			}
		}
		return false
	}
	if e.Action.Kind == LiveMVPVote { // a vote is the voter's, not the voted company's
		return e.ByUserID != 0 && e.ByUserID == u.ID
	}
	for _, c := range append([]int64{e.CompanyID}, e.Touches...) {
		if o := owners[c]; o != 0 && o == u.ID {
			return true
		}
	}
	return false
}
