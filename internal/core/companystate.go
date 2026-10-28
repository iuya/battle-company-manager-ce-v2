package core

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// CompanyState is a company as it is stored: plain data, no rules. BuildDetail turns it, with the campaign's RuleSet and
// settings, into what the pages show; the actions on a company (recruit, buy, level up...) are functions that change a
// *CompanyState inside companyRepository.Update. See docs/architecture.md, "Company aggregate and transactions".
//
// The processes that wait for the player between two requests are part of the state: the company's Recruit and each unit's
// Pending (a level-up roll, a treatment roll). They are stored with it (companies.pending, company_units.pending).
type CompanyState struct {
	// Company is the identity and the results the campaign keeps: id, name, faction (the name of its Battle Companies
	// company), owner and match record. Points, FullPoints and Units are derived by BuildDetail and ignored on write.
	Company   CampaignCompany
	Influence int
	Gold      int
	Rerolls   int         // Against the Odds re-rolls the company can still use
	Units     []UnitState // the roster, in any order
	Pack      []string    // what the Baggage Pony carries
	Order     []int64     // the order the player saved for the roster (unit ids); empty until they do
	Cemetery  []FallenUnit
	Graveyard []int64 // the order the player saved for the cemetery
	Recruit   RecruitProgress
	// Aftermaths are the War's Aftermaths the company still has to finish; the repository fills them on load and ignores them on
	// write. While there are any, the company cannot recruit, buy, drop or heal (Locked).
	Aftermaths []AftermathRef
	// Perks are the company's unused scenario perks (Rendezvous, Vantage Point, A Final Blessing); the repository fills them on load and
	// ignores them on write. PerksUsed are the ids of perks this change uses up: the repository marks them used in the same transaction
	// (and refuses one that is used already); it is empty on load.
	Perks     []Perk
	PerksUsed []int64
	// OpenMatch is the open (not ended) match the company is in; the repository fills it on load and ignores it on write.
	OpenMatch *OpenMatchRef
}

// RecruitProgress is the recruit the company is in the middle of: at most one of its parts is set. The influence it costs is
// spent when the roll is made (Roll.Paid keeps the sum for the summary).
type RecruitProgress struct {
	Roll   *RecruitStep  // rolled (or about to roll on the special chart), waiting for ConfirmRecruit / RollRecruit
	Lower  *LowerChoice  // the result cannot join: the player chooses a lower one
	Choice *OptionChoice // a unit joined with a "choice of options" still to be made
	// Officers is the Ranger of the North's arrival: the roll is paid and the unit chosen, the player has yet to arrange the
	// officers (RecruitService.ArrangeOfficers). Nothing is in the roster until then.
	Officers *OfficerStep
}

// OfficerStep is the Ranger of the North waiting for the officers to be arranged.
type OfficerStep struct {
	Unit string
	Paid int // influence the roll cost, for the summary
	Roll int
}

// Active says whether a recruit is in progress.
func (r RecruitProgress) Active() bool {
	return r.Roll != nil || r.Lower != nil || r.Choice != nil || r.Officers != nil
}

// RecruitStep is the roll of a recruit in progress.
type RecruitStep struct {
	Stage    string // "chart" or "special"
	Rolled   bool
	Roll     int
	Adjusted int // influence spent adjusting this recruit's rolls so far
	// PerkAdjust is the Rendezvous bonus (+1 or -1) applied to the die; PerkRaw is the die before it (Roll already includes the bonus).
	PerkAdjust, PerkRaw int
	Paid                int // influence spent on it so far, for the summary
}

// LowerChoice is a recruit whose result cannot join the company; the influence is spent, the player now chooses another result.
type LowerChoice struct {
	Prefix   string
	Paid     int
	Adjusted int
	Roll     int
	Reason   string
	Cannot   string // what cannot join, and why, for the summary
	Stage    string
	Entries  []LowerEntry
	Picks    int // how many results the player takes (a "choose any 2" result); 0 is one
}

// LowerEntry is a result the player may choose instead of the one that cannot join. Whether it can join either is worked out
// when it is shown.
type LowerEntry struct {
	Res   Reinforcement
	Chart string // "Reinforcements" or "Special chart"
	Roll  string
}

// OptionChoice is a recruit that rolled a "choice of options": the influence is spent, the unit joins once chosen.
type OptionChoice struct {
	Unit    string
	Wargear []string
	Count   int // how many models still wait for their options (a result of two models); 0 is one
}

// UnitPending is what a unit is in the middle of.
type UnitPending struct {
	Level *LevelRoll    // a level-up roll waiting for the player to choose
	Treat *PendingTreat // a treatment roll waiting for the decision on extra influence
}

// PendingTreat is a treatment roll that failed but could still be saved with extra influence.
type PendingTreat struct {
	Injury string
	Result TreatResult
}

// PathSpell is a magical power a hero learned through its path, with the casting value and range it has now.
type PathSpell struct {
	Name    string
	Casting int
	Range   int
}

// UnitState is one unit as it is stored. The unit names a vanilla profile (Base); what the company added to it is kept
// apart (equipment bought, what level-ups gave) so the profile, the rules and the points are always derived from the rules.
// Unit ids are never reused or shared between the roster and the cemetery.
type UnitState struct {
	ID            int64
	Name          string // what the player called the unit; may be empty
	Base          string // the vanilla unit it is (or the company's own label for one)
	PromotedFrom  string // set by BuildDetail, never stored: the profile the unit was recruited as, when a promotion changed Base
	Role          UnitRole
	Kind          UnitKind       // overrides the kind derived from the vanilla unit types
	Bonus         map[string]int // characteristics it has gained besides what Progress gave (a starting officer: 1 Might, Will, Fate)
	Gear          []string       // equipment it has from the start besides its vanilla wargear (what a recruit comes with)
	Bought        []string       // equipment bought during the campaign (origin "bought")
	Dropped       []string       // starting equipment it has dropped
	Path          string         // the hero's path
	PathRules     int            // how many of the path's special rules it has unlocked
	Actions       []string       // heroic actions the path unlocked
	BoughtActions []string       // heroic actions bought later
	Spells        []PathSpell    // magical powers learned through the path
	Level         int
	XP            int
	Progress      *Progress // what level-ups gave it
	Wanderer      string    // the armoury wanderer this unit is
	Points        int       // what the company's file says it is worth, when the vanilla profile has no points (a member of a Vault Warden Team)
	Injuries      map[string]int
	Injured       bool   // misses the next game: not counted in the company's rating
	SentToHealers bool   // a hero sent to the healers: it misses the next game too
	Creature      string // the creature an officer owns (an armoury creature's name)
	Pending       UnitPending
}

// Clone copies the unit so that changing the copy leaves the state alone.
func (u UnitState) Clone() UnitState {
	u.Bonus = cloneMap(u.Bonus)
	u.Injuries = cloneMap(u.Injuries)
	u.Gear, u.Bought, u.Dropped = slices.Clone(u.Gear), slices.Clone(u.Bought), slices.Clone(u.Dropped)
	u.Actions, u.BoughtActions, u.Spells = slices.Clone(u.Actions), slices.Clone(u.BoughtActions), slices.Clone(u.Spells)
	if u.Progress != nil {
		p := cloneProgress(u.Progress)
		u.Progress = &p
	}
	if u.Pending.Level != nil {
		l := cloneLevelRoll(*u.Pending.Level)
		u.Pending.Level = &l
	}
	if u.Pending.Treat != nil {
		t := *u.Pending.Treat
		u.Pending.Treat = &t
	}
	return u
}

// Clone copies the state so that changing the copy leaves the original alone.
func (s CompanyState) Clone() CompanyState {
	units := make([]UnitState, len(s.Units))
	for i, u := range s.Units {
		units[i] = u.Clone()
	}
	s.Units = units
	s.Pack, s.Order, s.Graveyard = slices.Clone(s.Pack), slices.Clone(s.Order), slices.Clone(s.Graveyard)
	s.Cemetery = slices.Clone(s.Cemetery)
	s.Recruit = s.Recruit.Clone()
	s.Aftermaths = slices.Clone(s.Aftermaths)
	s.Perks, s.PerksUsed = slices.Clone(s.Perks), slices.Clone(s.PerksUsed)
	return s
}

// Clone copies the recruit so that changing the copy leaves the original alone.
func (r RecruitProgress) Clone() RecruitProgress {
	if r.Roll != nil {
		v := *r.Roll
		r.Roll = &v
	}
	if r.Lower != nil {
		v := *r.Lower
		v.Entries = slices.Clone(v.Entries)
		r.Lower = &v
	}
	if r.Choice != nil {
		v := *r.Choice
		v.Wargear = slices.Clone(v.Wargear)
		r.Choice = &v
	}
	return r
}

// cloneLevelRoll copies the slices a level roll holds (a roll in progress is small and edited by value).
func cloneLevelRoll(l LevelRoll) LevelRoll {
	l.Pairs, l.Notes = slices.Clone(l.Pairs), slices.Clone(l.Notes)
	l.Offers = slices.Clone(l.Offers)
	if l.Upgrade != nil {
		u := *l.Upgrade
		l.Upgrade = &u
	}
	return l
}

func cloneMap(m map[string]int) map[string]int {
	if m == nil {
		return nil
	}
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneProgress(p *Progress) Progress {
	c := *p
	c.Bonus, c.Cast = cloneMap(p.Bonus), cloneMap(p.Cast)
	c.Rules, c.Minor, c.Actions, c.Powers, c.Gear = slices.Clone(p.Rules), slices.Clone(p.Minor), slices.Clone(p.Actions), slices.Clone(p.Powers), slices.Clone(p.Gear)
	c.Upgrades, c.Drop = slices.Clone(p.Upgrades), slices.Clone(p.Drop)
	return c
}

// NewCompany is a validated company ready to be stored: its starting units and resources.
type NewCompany struct {
	Name, Faction string
	Influence     int
	Gold          int
	Units         []UnitState // ids are assigned by the repository when zero
}

// Locked is nil, or ErrRefused when the company is in a running match or has an aftermath to finish: it cannot recruit, buy, drop, heal or change its
// roster until then (docs/aftermath.md).
func (s CompanyState) Locked() error {
	if s.OpenMatch != nil { // a running match wins over an older aftermath
		return Refuse("%s", LockMessageOpenMatch(*s.OpenMatch))
	}
	if s.Recruit.Officers != nil {
		return Refuse("Arrange the officers first: the Ranger of the North is waiting to take command.")
	}
	if len(s.Aftermaths) == 0 {
		return nil
	}
	return Refuse("%s", LockMessage(s.Aftermaths))
}

// LockMessage names every aftermath the company still has to finish: "Finish the aftermath of Match 3 first." or "Finish the
// aftermaths of Matches 3, 4 and 5 first." (empty when nothing is pending).
func LockMessage(refs []AftermathRef) string {
	switch len(refs) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("Finish the aftermath of Match %d first.", refs[0].MatchNumber)
	}
	var names []string
	for _, r := range refs {
		names = append(names, strconv.Itoa(r.MatchNumber))
	}
	return "Finish the aftermaths of Matches " + strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1] + " first."
}
