package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
)

// Moving a campaign to the app's current default rules files (docs/admin.md, "Rules files of a campaign"). A campaign is pinned to the
// hashes of its three files when it is created, so a release that changes the shipped files leaves running campaigns on the old ones.
// The organiser (or an administrator) can move it: the new files are loaded, every company is rebuilt against them, and the hashes
// change only when nothing blocks.

// CampaignRulesState is what the repository knows of a campaign for the rules update, read under a lock when it is changed.
type CampaignRulesState struct {
	OrganiserID       int64
	Name              string
	Status            CampaignStatus
	Hashes            RulesetHashes
	OpenMatches       int // matches not ended, practice ones too
	PendingAftermaths int // companies with an aftermath still to finish
}

// RulesFile is one of the three files of a campaign against the app's current default of its kind.
type RulesFile struct {
	Kind    RulesetKind
	Pinned  RulesetInfo
	Current RulesetInfo
	Differs bool
	// Custom: the pinned file is not an older edition of the default but another file (another name): a forked or uploaded one.
	Custom bool
}

// RulesUpdateCompany is what the new files do to one company.
type RulesUpdateCompany struct {
	ID, OwnerID  int64
	Name         string
	Problems     []string // blocking: the company cannot be played with the new files as it is
	Changes      []string // what differs in the roster, for the organiser to read (never blocking)
	Points, Gold [2]int   // before and after
}

// RulesUpdate is the answer to "what would moving this campaign to the current files do?".
type RulesUpdate struct {
	CampaignID int64
	Name       string
	Files      []RulesFile
	// Differs: at least one file is not the default. Custom: at least one of those is another file, not an older edition.
	Differs, Custom bool
	// Blockers are the reasons the campaign cannot be moved now whatever the companies say (finished, open matches, files that do not load).
	Blockers  []string
	Companies []RulesUpdateCompany
}

// Problems counts the blocking problems of the companies.
func (u RulesUpdate) Problems() (n int) {
	for _, c := range u.Companies {
		n += len(c.Problems)
	}
	return n
}

// Clean says the campaign can be moved: something differs and nothing blocks.
func (u RulesUpdate) Clean() bool { return u.Differs && len(u.Blockers) == 0 && u.Problems() == 0 }

// ErrRulesUpdateBlocked is returned by UpdateRules when the preview is not clean; its reasons are in the preview.
var ErrRulesUpdateBlocked = errors.Join(ErrRefused, errors.New("The campaign cannot be moved to the current files: check the update again and read what blocks it."))

// RulesUpdatePreview works out what moving the campaign to the app's default files would do, without changing or storing anything.
// Only the organiser and administrators may ask (ErrRefused otherwise); ErrNotFound for an unknown campaign.
func (c *Campaigns) RulesUpdatePreview(ctx context.Context, actor User, campaignID int64) (RulesUpdate, error) {
	st, err := c.campaigns.RulesState(ctx, campaignID)
	if err != nil {
		return RulesUpdate{}, err
	}
	if !CanManageCampaign(actor, st.OrganiserID) {
		return RulesUpdate{}, Refuse("Only the organiser of the campaign or an administrator can move it to other rules files.")
	}
	return c.rulesUpdate(ctx, campaignID, st)
}

func (c *Campaigns) rulesUpdate(ctx context.Context, campaignID int64, st CampaignRulesState) (RulesUpdate, error) {
	out := RulesUpdate{CampaignID: campaignID, Name: st.Name}
	var next RulesetHashes
	for _, f := range []struct {
		kind   RulesetKind
		pinned string
		set    *string
	}{{KindVanilla, st.Hashes.Vanilla, &next.Vanilla}, {KindBattleCompanies, st.Hashes.BattleCompanies, &next.BattleCompanies}, {KindGlossary, st.Hashes.Glossary, &next.Glossary}} {
		pinned, err := c.rulesets.Info(ctx, f.pinned)
		if err != nil {
			return out, err
		}
		cur, err := c.rulesets.Default(ctx, f.kind)
		if err != nil {
			return out, err
		}
		*f.set = cur.Hash
		file := RulesFile{Kind: f.kind, Pinned: pinned, Current: cur, Differs: pinned.Hash != cur.Hash, Custom: pinned.Hash != cur.Hash && pinned.Name != cur.Name}
		out.Differs = out.Differs || file.Differs
		out.Custom = out.Custom || file.Custom
		out.Files = append(out.Files, file)
	}
	if !out.Differs {
		return out, nil
	}
	switch {
	case st.Status == CampaignFinished:
		out.Blockers = append(out.Blockers, "The campaign has ended; an ended campaign never changes.")
	case st.OpenMatches > 0:
		out.Blockers = append(out.Blockers, fmt.Sprintf("%d %s still open: finish or delete %s first, so no match is played half with one set of rules and half with another.", st.OpenMatches, plural(st.OpenMatches, "match is", "matches are"), plural(st.OpenMatches, "it", "them")))
	}
	if st.PendingAftermaths > 0 {
		out.Blockers = append(out.Blockers, fmt.Sprintf("%d %s an aftermath to finish: let the players finish %s first.", st.PendingAftermaths, plural(st.PendingAftermaths, "company has", "companies have"), plural(st.PendingAftermaths, "it", "them")))
	}
	oldRS, settings, err := c.ruleset(ctx, campaignID)
	if err != nil {
		return out, err
	}
	newRS, err := c.rules.For(ctx, next)
	if err != nil {
		out.Blockers = append(out.Blockers, "The current files could not be loaded: "+reasonOf(err))
		return out, nil
	}
	list, err := c.campaigns.Companies(ctx, campaignID)
	if err != nil {
		return out, err
	}
	for _, co := range list {
		state, err := c.companies.Read(ctx, campaignID, co.ID)
		if err != nil {
			return out, err
		}
		out.Companies = append(out.Companies, compareCompany(oldRS, newRS, settings, state))
	}
	return out, nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// UpdateRules moves the campaign to the app's default files. It refuses (ErrRulesUpdateBlocked, ErrRefused) unless the preview is clean,
// re-checked under the lock the change is made under, and a campaign pinned to a custom file only when switchCustom says the caller
// knows the switch drops it. Only the organiser and administrators may.
func (c *Campaigns) UpdateRules(ctx context.Context, actor User, campaignID int64, switchCustom bool) error {
	var moved RulesetHashes
	err := c.campaigns.SetRulesets(ctx, campaignID, func(st CampaignRulesState) (RulesetHashes, error) {
		if !CanManageCampaign(actor, st.OrganiserID) {
			return st.Hashes, Refuse("Only the organiser of the campaign or an administrator can move it to other rules files.")
		}
		u, err := c.rulesUpdate(ctx, campaignID, st)
		if err != nil {
			return st.Hashes, err
		}
		switch {
		case !u.Differs:
			return st.Hashes, Refuse("The campaign already uses the app's current rules files.")
		case u.Custom && !switchCustom:
			return st.Hashes, Refuse("This campaign is played with files of its own; confirm that you want to switch to the app's files.")
		case !u.Clean():
			return st.Hashes, ErrRulesUpdateBlocked
		}
		for _, f := range u.Files {
			switch f.Kind {
			case KindVanilla:
				moved.Vanilla = f.Current.Hash
			case KindBattleCompanies:
				moved.BattleCompanies = f.Current.Hash
			case KindGlossary:
				moved.Glossary = f.Current.Hash
			}
		}
		return moved, nil
	})
	if err != nil {
		return err
	}
	slog.Info("campaign moved to the current rules files", "campaign", campaignID, "by", actor.ID, "vanilla", shortHash(moved.Vanilla), "battle_companies", shortHash(moved.BattleCompanies), "glossary", shortHash(moved.Glossary))
	return nil
}

func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

// compareCompany builds the company with the old and the new files and tells what blocks and what differs.
func compareCompany(oldRS, newRS *RuleSet, settings CampaignSettings, st CompanyState) RulesUpdateCompany {
	out := RulesUpdateCompany{ID: st.Company.ID, OwnerID: st.Company.OwnerID, Name: st.Company.Name}
	before := BuildDetail(oldRS, settings, st)
	bcCo, ok := newRS.BC.Company(st.Company.Faction)
	if !ok {
		out.Problems = append(out.Problems, fmt.Sprintf("The company %q is not in the new Battle Companies file.", st.Company.Faction))
		out.Points = [2]int{before.Points, before.Points}
		out.Gold = [2]int{before.Gold, before.Gold}
		return out
	}
	after := BuildDetail(newRS, settings, st)
	out.Points = [2]int{before.Points, after.Points}
	out.Gold = [2]int{before.Gold, after.Gold}
	profile, _ := newRS.BC.Profile(bcCo.Name)
	known := func(n string) bool {
		if _, ok := newRS.Vanilla.Gear(n); ok {
			return true
		}
		if _, ok := newRS.BC.Annotation(n); ok {
			return true
		}
		_, ok := newRS.BC.Offer(bcCo, n)
		return ok
	}
	for _, us := range st.Units {
		label := us.Name
		if label == "" {
			label = us.Base
		}
		if _, _, ok := newRS.ResolveUnit(profile, us.Base); !ok {
			out.Problems = append(out.Problems, fmt.Sprintf("%s: the unit %q is not in the new files.", label, us.Base))
			continue
		}
		var gone []string
		for _, n := range slices.Concat(us.Gear, us.Bought) {
			if !known(n) && !slices.Contains(gone, n) {
				gone = append(gone, n)
			}
		}
		if len(gone) > 0 {
			out.Problems = append(out.Problems, fmt.Sprintf("%s: %s %s not in the new files.", label, quoteList(gone), plural(len(gone), "is", "are")))
		}
		if u := afterUnit(after, us.ID); us.Path != "" && u != nil && u.Path == nil {
			out.Problems = append(out.Problems, fmt.Sprintf("%s: the path %q is not in the new files.", label, us.Path))
		}
		if us.Wanderer != "" && !hasWanderer(newRS, us.Wanderer) {
			out.Problems = append(out.Problems, fmt.Sprintf("%s: the wanderer %q is not in the new files.", label, us.Wanderer))
		}
		if us.Creature != "" && !hasCreature(newRS, us.Creature) {
			out.Problems = append(out.Problems, fmt.Sprintf("%s: the creature %q is not in the new files.", label, us.Creature))
		}
	}
	if after.MaxUnits > 0 && after.ModelCount() > after.MaxUnits {
		out.Problems = append(out.Problems, fmt.Sprintf("The company has %d models and the new files allow %d.", after.ModelCount(), after.MaxUnits))
	}
	was := map[string]CompanyLimit{}
	for _, l := range before.Limits {
		was[l.ID] = l
	}
	for _, l := range after.Limits {
		if l.Used > l.Max && was[l.ID].Used <= was[l.ID].Max {
			out.Problems = append(out.Problems, fmt.Sprintf("%s: %d used and the new files allow %d.", l.Label, l.Used, l.Max))
		}
	}
	out.Changes = rosterChanges(before, after)
	return out
}

func quoteList(ns []string) string {
	q := make([]string, len(ns))
	for i, n := range ns {
		q[i] = fmt.Sprintf("%q", n)
	}
	return strings.Join(q, ", ")
}

func afterUnit(d CompanyDetail, id int64) *CompanyUnit {
	for i := range d.Roster {
		if d.Roster[i].ID == id {
			return &d.Roster[i]
		}
	}
	return nil
}

func hasWanderer(rs *RuleSet, name string) bool {
	_, ws := rs.BC.Wanderers()
	return slices.ContainsFunc(ws, func(w WandererOffer) bool { return sameName(w.Name, name) })
}

func hasCreature(rs *RuleSet, name string) bool {
	_, cs := rs.BC.Creatures()
	return slices.ContainsFunc(cs, func(c CreatureOffer) bool { return sameName(c.Name, name) })
}

// rosterChanges lists what the new files change in a company's roster: points, characteristics, equipment and rules, a line per unit.
func rosterChanges(before, after CompanyDetail) []string {
	var out []string
	if before.Points != after.Points {
		out = append(out, fmt.Sprintf("Company points %d → %d.", before.Points, after.Points))
	}
	if before.Gold != after.Gold {
		out = append(out, fmt.Sprintf("Gold %d → %d.", before.Gold, after.Gold))
	}
	for _, b := range before.Roster {
		a := afterUnit(after, b.ID)
		if a == nil {
			continue
		}
		var parts []string
		if b.Points != a.Points {
			parts = append(parts, fmt.Sprintf("points %d → %d", b.Points, a.Points))
		}
		if bs, as := statText(b.Stats), statText(a.Stats); bs != as {
			parts = append(parts, fmt.Sprintf("profile %s → %s", bs, as))
		}
		if added, removed := diffNames(equipmentNames(b), equipmentNames(*a)); len(added)+len(removed) > 0 {
			parts = append(parts, "wargear "+changeText(added, removed))
		}
		if added, removed := diffNames(ruleNames(b), ruleNames(*a)); len(added)+len(removed) > 0 {
			parts = append(parts, "rules "+changeText(added, removed))
		}
		if len(parts) > 0 {
			name := b.Name
			if name == "" {
				name = b.Type
			}
			out = append(out, name+": "+strings.Join(parts, "; ")+".")
		}
	}
	return out
}

func statText(as []Attribute) string {
	p := make([]string, 0, len(as))
	for _, a := range as {
		p = append(p, fmt.Sprintf("%s %d%s", a.Abbr, a.Value, a.Trail))
	}
	return strings.Join(p, " ")
}

func equipmentNames(u CompanyUnit) []string {
	var n []string
	for _, e := range u.Equipment {
		n = append(n, e.Name)
	}
	return n
}

func ruleNames(u CompanyUnit) []string {
	var n []string
	for _, r := range u.Rules {
		n = append(n, r.Shown())
	}
	return n
}

func diffNames(before, after []string) (added, removed []string) {
	for _, n := range after {
		if !slices.Contains(before, n) {
			added = append(added, n)
		}
	}
	for _, n := range before {
		if !slices.Contains(after, n) {
			removed = append(removed, n)
		}
	}
	return
}

func changeText(added, removed []string) string {
	var p []string
	if len(added) > 0 {
		p = append(p, "+ "+strings.Join(added, ", "))
	}
	if len(removed) > 0 {
		p = append(p, "− "+strings.Join(removed, ", "))
	}
	return strings.Join(p, " ")
}
