package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The War's Aftermath (docs/aftermath.md). This file holds the Aftermath card of the match page; the page of one company's
// aftermath follows the same pattern as the match page.

// aftermathRow is one company of an ended match in the Aftermath card.
type aftermathRow struct {
	Ref     core.AftermathRef
	Owner   string
	Edit    bool   // the viewer may Start or Continue it
	Waive   bool   // the viewer may waive it and it is not finished yet
	Summary string // what a finished aftermath did, or the note of a waived one
}

// aftermathRows gathers the card of an ended match: one row per company. It never fails the page; the error text says what
// went wrong so the card can show it.
func (s *Server) aftermathRows(r *http.Request, base campaignBase, m core.Match) ([]aftermathRow, string) {
	refs, err := s.campaigns.AftermathStatuses(r.Context(), base.Campaign.ID, m.ID)
	if err != nil {
		return nil, "The aftermath could not be loaded. Reload the page to try again."
	}
	user := *userFrom(r.Context())
	var rows []aftermathRow
	for _, ref := range refs {
		row := aftermathRow{Ref: ref}
		var company core.CampaignCompany
		for _, side := range m.Sides {
			for _, c := range side.Companies {
				if c.ID == ref.CompanyID {
					company = c
				}
			}
		}
		row.Owner = company.Owner
		row.Edit = core.AftermathAccessFor(user, m, company, base.Campaign.OrganiserID) == core.AftermathEdit
		row.Waive = ref.Status.Locks() && core.CanWaive(user, base.Campaign.OrganiserID)
		if ref.Status == core.AftermathDone || ref.Status == core.AftermathWaived {
			if a, err := s.campaigns.Aftermath(r.Context(), user, base.Campaign.ID, m.ID, ref.CompanyID); err == nil {
				row.Summary = aftermathSummary(a)
			}
		}
		rows = append(rows, row)
	}
	return rows, ""
}

// aftermathSummary is the one line a finished aftermath leaves on the match page.
func aftermathSummary(a core.Aftermath) string {
	if a.Status == core.AftermathWaived {
		if a.WaivedNote == "" {
			return "Waived: nothing was applied."
		}
		return "Waived: " + a.WaivedNote
	}
	var parts []string
	if n := len(a.Casualties); n > 0 {
		dead := 0
		for _, c := range a.Casualties {
			if strings.EqualFold(c.ResultKey, "dead") {
				dead++
			}
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, core.Plural(n, "casualty", "casualties")))
		if dead > 0 {
			parts[len(parts)-1] += fmt.Sprintf(" (%d dead)", dead)
		}
	} else {
		parts = append(parts, "no casualties")
	}
	xp, inf := 0, 0
	for _, g := range a.Preview.Experience {
		xp += g.Total
	}
	for _, l := range a.Preview.Influence {
		inf += l.Amount
	}
	parts = append(parts, fmt.Sprintf("%+d Experience", xp), fmt.Sprintf("%+d Influence", inf))
	return strings.Join(parts, " · ")
}

// handleWaiveAftermath lets the organiser or an administrator waive a company's aftermath (with a note) from the match page.
func (s *Server) handleWaiveAftermath(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	companyID, ok := s.idParam(w, r, "company")
	if !ok {
		return
	}
	user := *userFrom(r.Context())
	if !core.CanWaive(user, base.Campaign.OrganiserID) {
		s.refuse(w, r, http.StatusForbidden, "Only the organiser or an administrator can waive an aftermath.")
		return
	}
	note := strings.TrimSpace(r.PostFormValue("note"))
	if n := len([]rune(note)); n < 1 || n > 200 {
		s.answer(w, r, http.StatusUnprocessableEntity, base, m, "", "Say why in 1 to 200 characters to waive an aftermath.", true, nil)
		return
	}
	_, err := s.campaigns.WaiveAftermath(r.Context(), user, base.Campaign.ID, m.ID, companyID, note)
	switch {
	case errors.Is(err, core.ErrRefused), errors.Is(err, core.ErrInvalidAftermath):
		s.answer(w, r, http.StatusUnprocessableEntity, base, m, "", reason(err), true, nil)
		return
	case s.fail(w, r, err):
		return
	}
	s.answer(w, r, http.StatusOK, base, m, "", "Aftermath waived.", false, nil)
}

// ---------------------------------------------------------------------------------------------------------------------
// The aftermath page of one company: /campaigns/{id}/matches/{match}/aftermath/{company}

// sheetUnit is a unit on the sheet with what was written for it.
type sheetUnit struct {
	Unit  core.CompanyUnit
	Sheet core.ModelSheet
	Err   string // what is wrong with it, from the last rejected save
	Items []itemRow
}

// itemRow is a consumable a unit may elect on the sheet (Healing Herbs, Wondrous Cram).
type itemRow struct {
	Key, Name, Label, Text string
	Stackable              bool
	Loaned                 bool // from a Healer's Kit: not in the unit's equipment
	Outcomes               []string
	Reasons                map[string]string // outcome or "took" -> why the unit cannot use it now
}

// itemRows lists the consumables the unit carries, or is loaned (A.ItemChoices), that its role could ever use.
func (v aftermathView) itemRows(u core.CompanyUnit) (rows []itemRow) {
	seen := map[string]bool{}
	add := func(key string, loaned bool) {
		eff, ok := v.Rules.Effects[key]
		if !ok || seen[key] || (eff.Carrier == "hero" && !u.IsHero()) {
			return
		}
		seen[key] = true
		name := effectItemName(v.Rules, key)
		r := itemRow{Key: key, Name: name, Text: eff.Text, Stackable: eff.Stackable, Loaned: loaned, Reasons: map[string]string{}}
		switch {
		case eff.Kind == "injuryRollModifier":
			r.Label = fmt.Sprintf("Use %s (%+d to every injury roll of the company)", name, eff.Modifier)
		case eff.NoRoll:
			r.Label = "Use " + name + " (skips the injury roll: counts as " + effectResultName(v.Rules, eff) + ")"
		default:
			r.Label = "Use " + name
		}
		r.Outcomes = []string{"survived", "casualty", "fled"}
		for _, req := range eff.Requires {
			switch req {
			case "casualty":
				r.Outcomes = []string{"casualty"}
				r.Reasons["survived"], r.Reasons["fled"] = "Only a casualty can use "+name+".", "Only a casualty can use "+name+"."
			case "notCasualty":
				r.Outcomes = []string{"survived", "fled"}
				r.Reasons["casualty"] = "Casualties cannot use " + name + "."
			case "participated":
				r.Reasons["took"] = "A unit that did not take part cannot use " + name + "."
			}
		}
		rows = append(rows, r)
	}
	for _, e := range u.Equipment {
		if eff, ok := v.Rules.ItemEffect(e.Name); ok {
			add(eff.Key, false)
		}
	}
	for _, key := range v.A.ItemChoices[u.ID] {
		add(key, true)
	}
	return rows
}

// effectItemName is the equipment name that activates an effect ("Healing Herbs").
func effectItemName(rules core.AftermathRules, key string) string {
	best := ""
	for name, k := range rules.ItemEffects {
		if k == key && (best == "" || name < best) {
			best = name
		}
	}
	if best == "" {
		return key
	}
	return strings.Title(best) //nolint:staticcheck // ASCII equipment names
}

// effectResultName is the chart row a Wondrous Cram counts as ("Full Recovery").
func effectResultName(rules core.AftermathRules, eff core.RewardEffect) string {
	for _, r := range append(slices.Clone(rules.Warrior), rules.Hero...) {
		if r.Key == eff.Result {
			return r.Name
		}
	}
	return eff.Result
}

// ItemsJSON is what the unit's switches need in the browser: per effect the outcomes it allows and the reasons it is off.
func (u sheetUnit) ItemsJSON() string {
	m := map[string]any{}
	for _, r := range u.Items {
		m[r.Key] = map[string]any{"outcomes": r.Outcomes, "reasons": r.Reasons}
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// Elected is the election of the sheet as a JSON object, unit id to effect key.
func (v aftermathView) Elected() string {
	m := map[string]string{}
	for _, u := range v.Units {
		if u.Sheet.UsesItem != "" {
			m[strconv.FormatInt(u.Unit.ID, 10)] = u.Sheet.UsesItem
		}
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// Raw is the sum of a casualty's dice, before a modifier ("9" of "9 +1 = 10").
func (v aftermathView) Raw(c core.Casualty) (n int) {
	for _, d := range c.Dice {
		n += d
	}
	return n
}

// UnitNames is the names of the sheet's units by id, as JSON, for the "already used by" notes.
func (v aftermathView) UnitNames() string {
	m := map[string]string{}
	for _, u := range v.Units {
		m[strconv.FormatInt(u.Unit.ID, 10)] = u.Unit.Title()
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// ItemModifier is the injury-roll modifier the saved sheet elects (Healing Herbs: +1), 0 without.
func (v aftermathView) ItemModifier() int { return core.RollModifierOf(v.Rules, v.A.Input).Amount }

// ItemBanner is the sentence of the injuries step when a modifier is in force.
func (v aftermathView) ItemBanner() string {
	rm := core.RollModifierOf(v.Rules, v.A.Input)
	if rm.Amount == 0 {
		return ""
	}
	return fmt.Sprintf("%s: %+d to every injury roll", effectItemName(v.Rules, rm.Effect), rm.Amount)
}

// ItemName names an elected effect for the read-only sheet.
func (v aftermathView) ItemName(key string) string { return effectItemName(v.Rules, key) }

// LevelEvery is the experience per level-up of the campaign's rules (progression.threshold).
func (v aftermathView) LevelEvery() int { return v.Rules.LevelEvery }

type aftermathView struct {
	campaignBase
	Match   core.Match
	Company core.CampaignCompany
	A       core.Aftermath
	Rules   core.AftermathRules
	Result  core.CompanyResult
	Units   []sheetUnit
	Absent  []core.CompanyUnit
	Roster  map[int64]core.CompanyUnit

	// RangerFell: the company's Ranger of the North fell in this match and a reinforcement may bring one back (an informational line).
	RangerFell bool

	Locked string // why nothing can be done yet (the match is still open)

	Notice    string
	NoticeErr bool
	Problems  []string // messages of a rejected sheet that belong to no unit

	Animate map[int64]bool // the casualties the server just rolled: their dice are shown rolling first

	ShowFinish bool                            // the Finish block is open (the player continued, or no level-up roll waits)
	Offers     map[string][]core.FreeEquipment // free equipment to choose from, by the key of the reward that offers it
}

// Owner is whether the viewer plays this company: the level-up window of My Company works on the viewer's own company.
func (v aftermathView) Owner() bool {
	return v.Campaign.Company != nil && v.Campaign.Company.ID == v.Company.ID
}

// Waiting lists the units of the company with level-up rolls waiting.
func (v aftermathView) Waiting() (out []core.CompanyUnit) {
	for _, u := range v.A.Input.Models {
		if unit, ok := v.Roster[u.UnitID]; ok && unit.PendingLevelUps() > 0 {
			out = append(out, unit)
		}
	}
	// units that were not on the sheet but have rolls waiting (heroes of earlier matches) are listed too
	for _, unit := range v.Roster {
		if unit.PendingLevelUps() > 0 && !slices.ContainsFunc(out, func(x core.CompanyUnit) bool { return x.ID == unit.ID }) {
			out = append(out, unit)
		}
	}
	slices.SortFunc(out, func(a, b core.CompanyUnit) int { return strings.Compare(a.Title(), b.Title()) })
	return out
}

// Chosen says whether the player already answered the decision of a reward (it is not in Pending any more).
func (v aftermathView) Chosen(key string) bool {
	return !slices.ContainsFunc(v.A.Pending, func(r core.RewardApply) bool { return r.Key == key })
}

// SummaryGroup is the lines of the finished aftermath that belong to one unit (UnitID 0: the company).
type SummaryGroup struct {
	Name  string
	Lines []core.AftermathLine
}

// Summary groups the audit trail by unit, the company's own lines first.
func (v aftermathView) Summary() []SummaryGroup {
	var order []int64
	by := map[int64][]core.AftermathLine{}
	for _, l := range v.A.Lines {
		if _, seen := by[l.UnitID]; !seen {
			order = append(order, l.UnitID)
		}
		by[l.UnitID] = append(by[l.UnitID], l)
	}
	slices.SortStableFunc(order, func(a, b int64) int {
		switch {
		case a == 0 && b != 0:
			return -1
		case b == 0 && a != 0:
			return 1
		}
		return 0
	})
	var out []SummaryGroup
	for _, id := range order {
		name := "The company"
		if id != 0 {
			name = v.UnitName(id)
			if name == "" {
				name = "A unit that left"
			}
		}
		out = append(out, SummaryGroup{Name: name, Lines: by[id]})
	}
	return out
}

// Settings are the campaign's table rules (typed and thrown dice).
func (v aftermathView) Settings() core.CampaignSettings { return v.Campaign.Settings }

// Unrolled lists the casualties that still wait for their roll.
func (v aftermathView) Unrolled() (out []core.Casualty) {
	for _, c := range v.A.Casualties {
		if !c.Rolled {
			out = append(out, c)
		}
	}
	return out
}

// UnrolledIDs is the same as comma separated ids, what "Roll all" reports back to know which dice to animate.
func (v aftermathView) UnrolledIDs() string {
	var ids []string
	for _, c := range v.Unrolled() {
		ids = append(ids, strconv.FormatInt(c.UnitID, 10))
	}
	return strings.Join(ids, ",")
}

// Applied says that the injuries have been made real (the casualties are final).
func (v aftermathView) Applied() bool {
	return len(v.A.Casualties) > 0 && v.A.Casualties[0].Final
}

// CanContinue: every casualty is rolled and decided, Against the Odds re-rolls are left and the results are not final yet; the
// player says they are happy with them.
func (v aftermathView) CanContinue() bool {
	if !v.Edit() || v.A.Status != core.AftermathInjuries || v.A.Rerolls == 0 || v.Applied() || len(v.A.Casualties) == 0 {
		return false
	}
	for _, c := range v.A.Casualties {
		if !c.Rolled || c.Decision != nil {
			return false
		}
	}
	return true
}

// CanReroll: an Against the Odds re-roll can be spent on a rolled casualty that is not final.
func (v aftermathView) CanReroll(c core.Casualty) bool {
	return v.Edit() && v.A.Rerolls > 0 && c.Rolled && !c.Final && c.Decision == nil
}

// Chart is the injuries chart of the casualty's kind of unit.
func (v aftermathView) Chart(c core.Casualty) []core.InjuryRow { return v.Rules.Chart(c.Hero) }

// DiceCSV is a casualty's two dice for the rolling animation.
func (v aftermathView) DiceCSV(c core.Casualty) string {
	var ds []string
	for _, d := range c.Dice {
		ds = append(ds, strconv.Itoa(d))
	}
	return strings.Join(ds, ",")
}

// BonusText says what a casualty's bonus die gave ("+2 Experience").
func (v aftermathView) BonusText(c core.Casualty) string {
	if c.Bonus == 0 {
		return ""
	}
	for _, row := range v.Chart(c) {
		if row.Key == c.ResultKey {
			return fmt.Sprintf("+%d %s", c.Bonus, strings.ToUpper(row.BonusKind[:1])+row.BonusKind[1:])
		}
	}
	return fmt.Sprintf("+%d", c.Bonus)
}

// BonusLine spells out the bonus die of a result that has one (Lesson Learned: D3 Experience; Wounds of a Hero: D6 Influence):
// the amount rolled, or that it is still to come while a question about the same result is open. Empty for any other result.
func (v aftermathView) BonusLine(c core.Casualty) string {
	if !c.Rolled {
		return ""
	}
	for _, row := range v.Chart(c) {
		if row.Key != c.ResultKey || row.BonusDice == "" {
			continue
		}
		kind := strings.ToUpper(row.BonusKind[:1]) + row.BonusKind[1:]
		dice := strings.ToUpper(row.BonusDice)
		switch {
		case c.Bonus > 0:
			return fmt.Sprintf("Extra %s %s rolled: +%d %s.", dice, kind, c.Bonus, kind)
		case c.Decision != nil && c.Decision.Kind != "bonus":
			return fmt.Sprintf("The extra %s %s is rolled once the question below is answered.", dice, kind)
		}
	}
	return ""
}

// Fallen says whether the result took the unit out of the company.
func (v aftermathView) Fallen(c core.Casualty) bool {
	return c.Kind == core.CasualtyDead || c.Kind == core.CasualtyRetired
}

// BonusMax is the highest number the player types for the casualty's bonus roll (6 for a D3, rolled on a D6).
func (v aftermathView) BonusMax(c core.Casualty) int {
	for _, row := range v.Chart(c) {
		if row.Key == c.ResultKey && row.BonusDice != "" {
			return core.TypedMax(row.BonusSpec)
		}
	}
	return 6
}

// foldText says how a six-sided die reads for a die of fewer faces: " On a D3 a die of 1-2 reads 1, 3-4 reads 2 and 5-6 reads 3."
func foldText(spec core.DiceSpec) string {
	per := 6 / spec.Sides
	var parts []string
	for k := 1; k <= spec.Sides; k++ {
		parts = append(parts, fmt.Sprintf("%s reads %d", rangeText((k-1)*per+1, k*per), k))
	}
	return fmt.Sprintf(" On a %s a die of %s and %s.", spec.Display(), strings.Join(parts[:len(parts)-1], ", "), parts[len(parts)-1])
}

// BonusDieText asks for the bonus die of Lesson Learned (a D3) or Wounds of a Hero (a D6).
func (v aftermathView) BonusDieText(c core.Casualty) string {
	for _, row := range v.Chart(c) {
		if row.Key == c.ResultKey && row.BonusDice != "" {
			text := fmt.Sprintf("%s: roll a %s for the bonus %s.", row.Name, strings.ToUpper(row.BonusDice), row.BonusKind)
			if core.TypedMax(row.BonusSpec) > row.BonusSpec.Max() { // a die with no physical form (a D3) is rolled on a D6
				text += foldText(row.BonusSpec)
			}
			return text
		}
	}
	return "Roll the bonus die."
}

// RewardText says what a scenario reward of the preview gives, for the bar under the sheet.
func (v aftermathView) RewardText(r core.RewardApply) string {
	switch r.Kind {
	case "influence":
		return fmt.Sprintf("%s: +%d Influence", r.Name, r.Amount)
	case "experience":
		if r.UnitID != 0 {
			return fmt.Sprintf("%s: +%d Experience to %s", r.Name, r.Amount, v.UnitName(r.UnitID))
		}
		return fmt.Sprintf("%s: +%d Experience", r.Name, r.Amount)
	case "freeEquipment":
		return fmt.Sprintf("%s: a free item worth up to %d Influence, chosen at Finish", r.Name, r.MaxValue)
	case "perk":
		return r.Name + ": a perk for the next game, kept on the company"
	}
	return r.Name
}

// Note says in a few words what a settled result did to the unit (shown beside the result in the collapsed Injuries step).
func (v aftermathView) Note(c core.Casualty) string {
	switch c.Kind {
	case core.CasualtyDead:
		return "goes to the cemetery"
	case core.CasualtyRetired:
		return fmt.Sprintf("forced to retire by its injury number %d, goes to the cemetery", c.Count)
	case core.CasualtyInjured, core.CasualtyScratch:
		return "misses the next game"
	case core.CasualtyHeal:
		for _, row := range v.Rules.Hero {
			if row.Key == c.InjuryKey && c.InjuryKey != "" {
				return "healed one " + row.Name
			}
		}
		return "nothing healed"
	}
	return ""
}

// Edit is whether the viewer may change the aftermath.
func (v aftermathView) Edit() bool { return v.A.Access == core.AftermathEdit }

// Step is the index of the current step: 0 Sheet, 1 Injuries, 2 Level-ups (Finish follows on Continue), 4 when it is all over.
func (v aftermathView) Step() int {
	switch v.A.Status {
	case core.AftermathInjuries:
		return 1
	case core.AftermathLevelUps:
		return 2
	case core.AftermathDone, core.AftermathWaived:
		return 4 // every step is behind
	}
	return 0
}

// Editing is whether the Sheet step shows its form.
func (v aftermathView) Editing() bool { return v.Edit() && v.Step() == 0 }

// Closed says that nothing more will happen: the aftermath is done or waived.
func (v aftermathView) Closed() bool {
	return v.A.Status == core.AftermathDone || v.A.Status == core.AftermathWaived
}

// CanReopenSheet: the sheet can be unlocked until the first casualty is rolled.
func (v aftermathView) CanReopenSheet() bool {
	if !v.Edit() || v.A.Status != core.AftermathInjuries {
		return false
	}
	for _, c := range v.A.Casualties {
		if c.Rolled {
			return false
		}
	}
	return true
}

// RelicTicked is whether the match result says the company holds the Relic, so the sheet must say which unit has it.
func (v aftermathView) RelicTicked() bool { return v.Result.Objectives["possessesRelic"] > 0 }

// Relic is the unit that holds the Relic on the sheet, 0 for none.
func (v aftermathView) Relic() int64 {
	for _, m := range v.A.Input.Models {
		if m.Relic {
			return m.UnitID
		}
	}
	return 0
}

// MVP is the unit that is the company's MVP on the sheet, 0 for none.
func (v aftermathView) MVP() int64 {
	for _, m := range v.A.Input.Models {
		if m.Gains["mvp"] > 0 {
			return m.UnitID
		}
	}
	return 0
}

// Participants and Casualties count the sheet, for the collapsed summary.
func (v aftermathView) Participants() (n int) {
	for _, m := range v.A.Input.Models {
		if m.Took() {
			n++
		}
	}
	return n
}

func (v aftermathView) Casualties() (n int) {
	for _, m := range v.A.Input.Models {
		if m.Outcome == core.OutcomeCasualty {
			n++
		}
	}
	return n
}

// XP, Influence and Rolls are the numbers of the preview.
func (v aftermathView) XP() (n int) {
	for _, g := range v.A.Preview.Experience {
		n += g.Total
	}
	return n
}

func (v aftermathView) Influence() (n int) {
	for _, l := range v.A.Preview.Influence {
		n += l.Amount
	}
	return n
}

// Gains are the ways to earn experience the sheet offers as chips: participation is the "Took part" switch.
func (v aftermathView) Gains() []core.XPGain {
	var out []core.XPGain
	for _, g := range v.Rules.Gains {
		if g.Key != "participated" {
			out = append(out, g)
		}
	}
	return out
}

// GainLabels lists, for a read-only card, what a unit earned.
func (v aftermathView) GainLabels(m core.ModelSheet) []string {
	var out []string
	for _, g := range v.Gains() {
		switch n := m.Gains[g.Key]; {
		case n > 1:
			out = append(out, fmt.Sprintf("%s x%d", g.Label, n))
		case n == 1:
			out = append(out, g.Label)
		}
	}
	return out
}

// OutcomeLabel is how the sheet words an outcome.
func (v aftermathView) OutcomeLabel(o core.ModelOutcome) string {
	switch o {
	case core.OutcomeCasualty:
		return "Casualty"
	case core.OutcomeFled:
		return "Fled"
	}
	return "Survived"
}

// OutcomeBadge is the badge colour of an outcome: survived green, fled yellow, casualty red.
func (v aftermathView) OutcomeBadge(o core.ModelOutcome) string {
	switch o {
	case core.OutcomeCasualty:
		return "badge-danger"
	case core.OutcomeFled:
		return "badge-warn"
	}
	return "badge-success"
}

// UnitName is the title of a unit of the company, whoever it is now.
func (v aftermathView) UnitName(id int64) string { return v.Roster[id].Title() }

// OutcomeText is the header's badge text.
func (v aftermathView) OutcomeText() string {
	switch {
	case v.A.Fled:
		return "Fled (All is Lost)"
	case v.A.Outcome == "win":
		return "Win"
	case v.A.Outcome == "draw":
		return "Draw"
	}
	return "Defeat"
}

func (v aftermathView) OutcomeTone() string {
	switch {
	case v.A.Fled, v.A.Outcome == "loss":
		return "loss"
	case v.A.Outcome == "win":
		return "win"
	}
	return "draw"
}

// aftermathCtx loads the campaign, match and company of the request; it answers 404 itself when one is unknown.
func (s *Server) aftermathCtx(w http.ResponseWriter, r *http.Request) (campaignBase, core.Match, core.CampaignCompany, bool) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return base, m, core.CampaignCompany{}, false
	}
	companyID, ok := s.idParam(w, r, "company")
	if !ok {
		return base, m, core.CampaignCompany{}, false
	}
	for _, side := range m.Sides {
		for _, c := range side.Companies {
			if c.ID == companyID {
				return base, m, c, true
			}
		}
	}
	s.notFound(w, r)
	return base, m, core.CampaignCompany{}, false
}

// newAftermathView gathers what the steps need. a is the state to show; an error from the core is returned as is.
func (s *Server) newAftermathView(r *http.Request, base campaignBase, m core.Match, co core.CampaignCompany, a core.Aftermath) (aftermathView, error) {
	v := aftermathView{campaignBase: base, Match: m, Company: co, A: a, Result: m.Result.Company(co.ID), Roster: map[int64]core.CompanyUnit{}}
	var err error
	if v.Rules, err = s.campaigns.AftermathRules(r.Context(), base.Campaign.ID); err != nil {
		return v, err
	}
	detail, err := s.campaigns.CompanyDetail(r.Context(), base.Campaign.ID, co.ID)
	if err != nil {
		return v, err
	}
	for _, u := range core.Distinguish(detail.Roster) {
		v.Roster[u.ID] = u
	}
	if detail.RangerMayReturn {
		where := fmt.Sprintf("Match %d", m.Number)
		for _, f := range detail.Cemetery {
			if f.Unit.Type == detail.RangerUnit && (f.Match == where || strings.HasPrefix(f.Match, where+",")) {
				v.RangerFell = true
			}
		}
	}
	v.fillUnits(a.Input, nil)
	if v.Edit() && a.Status == core.AftermathLevelUps && len(a.Pending) > 0 {
		v.Offers = map[string][]core.FreeEquipment{}
		for _, rw := range a.Pending {
			if rw.Kind != "freeEquipment" {
				continue
			}
			if v.Offers[rw.Key], err = s.campaigns.FreeEquipmentOffers(r.Context(), base.Campaign.ID, co.ID, rw.MaxValue); err != nil {
				return v, err
			}
		}
	}
	v.ShowFinish = a.Status == core.AftermathLevelUps && (len(v.Waiting()) == 0 || !v.Edit()) // a reader has no button to continue: the step is open for them
	return v, nil
}

// fillUnits lists the units on the sheet (in the company's order) and the ones that missed the game; errs puts the messages of
// a rejected save beside their unit.
func (v *aftermathView) fillUnits(in core.AftermathInput, errs map[int64]string) {
	v.Units, v.Absent = nil, nil
	// the sheet is in the company's order; the units that missed the game follow
	for _, m := range in.Models {
		if u, ok := v.Roster[m.UnitID]; ok {
			v.Units = append(v.Units, sheetUnit{Unit: u, Sheet: m, Err: errs[m.UnitID], Items: v.itemRows(u)})
		}
	}
	for _, id := range v.A.Absent {
		if u, ok := v.Roster[id]; ok {
			v.Absent = append(v.Absent, u)
		}
	}
}

// handleAftermath shows a company's aftermath. An open match shows the locked state.
func (s *Server) handleAftermath(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(core.WithReadScope(r.Context())) // a read only request: the services share the match, company and record they read
	base, m, co, ok := s.aftermathCtx(w, r)
	if !ok {
		return
	}
	a, err := s.campaigns.Aftermath(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID, co.ID)
	if errors.Is(err, core.ErrRefused) {
		s.render.page(w, r, http.StatusOK, "aftermath", aftermathView{campaignBase: base, Match: m, Company: co, Locked: reason(err), Roster: map[int64]core.CompanyUnit{}})
		return
	}
	if s.fail(w, r, err) {
		return
	}
	v, err := s.newAftermathView(r, base, m, co, a)
	if s.fail(w, r, err) {
		return
	}
	s.render.page(w, r, http.StatusOK, "aftermath", v)
}

// aftermathAnswer renders the whole page body again after a write, with a notice on top.
func (s *Server) aftermathAnswer(w http.ResponseWriter, r *http.Request, status int, base campaignBase, m core.Match, co core.CampaignCompany, a core.Aftermath, notice string, isErr bool, edit func(*aftermathView)) {
	v, err := s.newAftermathView(r, base, m, co, a)
	if s.fail(w, r, err) {
		return
	}
	v.Notice, v.NoticeErr = notice, isErr
	if edit != nil {
		edit(&v)
	}
	s.render.partial(w, status, "aftermath", "aftermath_body", v)
}

// aftermathFailed answers an error of a write the expected way; ok is true when it has been answered. A viewer who may not edit
// gets a plain 403; a stale form gets the current state with a notice; bad input comes back 422 with the form as it was typed.
func (s *Server) aftermathFailed(w http.ResponseWriter, r *http.Request, base campaignBase, m core.Match, co core.CampaignCompany, err error, typed *core.AftermathInput) bool {
	user := *userFrom(r.Context())
	current := func() (core.Aftermath, bool) {
		a, gerr := s.campaigns.Aftermath(r.Context(), user, base.Campaign.ID, m.ID, co.ID)
		return a, !s.fail(w, r, gerr)
	}
	switch {
	case errors.Is(err, core.ErrAftermathChanged):
		if a, ok := current(); ok {
			s.aftermathAnswer(w, r, http.StatusOK, base, m, co, a, "Somebody changed this aftermath while you were editing, so what you see now is its current state. Check it and make your change again.", true, nil)
		}
	case errors.Is(err, core.ErrRefused):
		a, ok := current()
		if !ok {
			return true
		}
		if a.Access != core.AftermathEdit {
			s.refuse(w, r, http.StatusForbidden, reason(err))
			return true
		}
		s.aftermathAnswer(w, r, http.StatusUnprocessableEntity, base, m, co, a, reason(err), true, nil)
	case errors.Is(err, core.ErrInvalidAftermath):
		a, ok := current()
		if !ok {
			return true
		}
		s.aftermathAnswer(w, r, http.StatusUnprocessableEntity, base, m, co, a, "", false, func(v *aftermathView) {
			byUnit := map[int64]string{}
			for _, msg := range reasons(err) {
				attached := false
				for id, u := range v.Roster {
					if t := u.Title(); strings.HasPrefix(msg, t+":") || strings.HasPrefix(msg, t+" ") {
						byUnit[id], attached = msg, true
						break
					}
				}
				if !attached {
					v.Problems = append(v.Problems, msg)
				}
			}
			in := v.A.Input
			if typed != nil {
				in = *typed
				v.A.Input = in
			}
			v.fillUnits(in, byUnit)
		})
	default:
		return s.fail(w, r, err)
	}
	return true
}

// sheetFromForm reads the sheet form: the units listed in "unit", each with its outcome, "took" switch and gains, the Relic
// holder and the Against the Odds bonuses.
func sheetFromForm(r *http.Request, rules core.AftermathRules) core.AftermathInput {
	_ = r.ParseForm()
	var in core.AftermathInput
	in.AgainstTheOdds = slices.Clone(r.PostForm["odds"])
	relic, _ := strconv.ParseInt(r.PostFormValue("relic"), 10, 64)
	for _, raw := range r.PostForm["unit"] {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			continue
		}
		m := core.ModelSheet{UnitID: id, Outcome: core.ModelOutcome(r.PostFormValue("outcome-" + raw)), Gains: map[string]int{}, Relic: id == relic, UsesItem: r.PostFormValue("item-" + raw)}
		if r.PostFormValue("took-"+raw) != "" {
			m.Gains["participated"] = 1
			for _, g := range rules.Gains {
				if n, _ := strconv.Atoi(r.PostFormValue("gain-" + raw + "-" + g.Key)); n != 0 && g.Key != "participated" {
					m.Gains[g.Key] = n
				}
			}
		}
		in.Models = append(in.Models, m)
	}
	return in
}

// handleSaveSheet saves the sheet (start false) or saves it and starts the injuries (start true).
func (s *Server) handleSheet(start bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		base, m, co, ok := s.aftermathCtx(w, r)
		if !ok {
			return
		}
		rules, err := s.campaigns.AftermathRules(r.Context(), base.Campaign.ID)
		if s.fail(w, r, err) {
			return
		}
		in := sheetFromForm(r, rules)
		user := *userFrom(r.Context())
		var a core.Aftermath
		if start {
			a, err = s.campaigns.StartAftermath(r.Context(), user, base.Campaign.ID, m.ID, co.ID, revisionOf(r), in)
		} else {
			a, err = s.campaigns.SaveAftermath(r.Context(), user, base.Campaign.ID, m.ID, co.ID, revisionOf(r), in)
		}
		if err != nil {
			s.aftermathFailed(w, r, base, m, co, err, &in)
			return
		}
		notice := "Sheet saved."
		if start {
			notice = "Aftermath started: the sheet is locked and the casualty rolls come next."
		}
		s.aftermathAnswer(w, r, http.StatusOK, base, m, co, a, notice, false, nil)
	}
}

// handleReopenSheet unlocks the sheet again before the first casualty roll.
func (s *Server) handleReopenSheet(w http.ResponseWriter, r *http.Request) {
	base, m, co, ok := s.aftermathCtx(w, r)
	if !ok {
		return
	}
	a, err := s.campaigns.ReopenSheet(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID, co.ID, revisionOf(r))
	if err != nil {
		s.aftermathFailed(w, r, base, m, co, err, nil)
		return
	}
	s.aftermathAnswer(w, r, http.StatusOK, base, m, co, a, "The sheet is open again.", false, nil)
}

// ---------------------------------------------------------------------------------------------------------------------
// The Injuries step

// handleCasualtyRoll rolls one casualty (unit) or every unrolled one (no unit): the app's dice, or the dice the player
// typed or threw in the browser (core.CampaignSettings.RollRefusal applies).
func (s *Server) handleCasualtyRoll(w http.ResponseWriter, r *http.Request) {
	base, m, co, ok := s.aftermathCtx(w, r)
	if !ok {
		return
	}
	unit, _ := strconv.ParseInt(r.PostFormValue("unit"), 10, 64)
	entered := enteredRolls(r) // the dice typed or thrown; the core checks their number and range against the injury dice of the rules
	own := entered != nil
	if s.partialRolls(w, r, base, m, co, entered) {
		return
	}
	a, err := s.campaigns.RollCasualty(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID, co.ID, unit, entered, r.PostFormValue("thrown") != "")
	if err != nil {
		s.aftermathFailed(w, r, base, m, co, err, nil)
		return
	}
	animate := map[int64]bool{}
	if !own {
		if unit != 0 {
			animate[unit] = true
		} else {
			for _, raw := range strings.Split(r.PostFormValue("unrolled"), ",") {
				if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
					animate[id] = true
				}
			}
		}
	}
	s.aftermathAnswer(w, r, http.StatusOK, base, m, co, a, "", false, func(v *aftermathView) { v.Animate = animate })
}

// partialRolls answers a form with some dice typed and others left on "Roll for me" (422, the sheet as it was) and reports it did.
func (s *Server) partialRolls(w http.ResponseWriter, r *http.Request, base campaignBase, m core.Match, co core.CampaignCompany, entered []int) bool {
	if !slices.Contains(entered, 0) {
		return false
	}
	a, err := s.campaigns.Aftermath(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID, co.ID)
	if s.fail(w, r, err) {
		return true
	}
	s.aftermathAnswer(w, r, http.StatusUnprocessableEntity, base, m, co, a, "Enter all the dice, each showing a face of the die, or leave them all on Roll for me.", true, nil)
	return true
}

// handleCasualtyReroll spends an Against the Odds re-roll on a casualty.
func (s *Server) handleCasualtyReroll(w http.ResponseWriter, r *http.Request) {
	base, m, co, ok := s.aftermathCtx(w, r)
	if !ok {
		return
	}
	unit, _ := strconv.ParseInt(r.PostFormValue("unit"), 10, 64)
	entered := enteredRolls(r)
	if s.partialRolls(w, r, base, m, co, entered) {
		return
	}
	a, err := s.campaigns.RerollCasualty(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID, co.ID, unit, entered, false)
	if err != nil {
		s.aftermathFailed(w, r, base, m, co, err, nil)
		return
	}
	s.aftermathAnswer(w, r, http.StatusOK, base, m, co, a, "", false, func(v *aftermathView) {
		if entered == nil {
			v.Animate = map[int64]bool{unit: true}
		}
	})
}

// handleDecide answers an open question: a casualty's (scratch, heal), the Leader's succession or a new Sergeant, or "continue"
// (the player is happy with the results).
func (s *Server) handleDecide(w http.ResponseWriter, r *http.Request) {
	base, m, co, ok := s.aftermathCtx(w, r)
	if !ok {
		return
	}
	unit, _ := strconv.ParseInt(r.PostFormValue("unit"), 10, 64)
	ans := core.AftermathAnswer{UnitID: unit, Kind: r.PostFormValue("kind"), Key: r.PostFormValue("key"), Answer: r.PostFormValue("answer")}
	if ans.Kind == "" || (ans.Answer == "" && ans.Kind != "heal" && ans.Kind != "sergeant" && ans.Kind != "continue") {
		a, err := s.campaigns.Aftermath(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID, co.ID)
		if s.fail(w, r, err) {
			return
		}
		s.aftermathAnswer(w, r, http.StatusUnprocessableEntity, base, m, co, a, "Choose an answer first.", true, nil)
		return
	}
	a, err := s.campaigns.Decide(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID, co.ID, ans)
	if err != nil {
		s.aftermathFailed(w, r, base, m, co, err, nil)
		return
	}
	animate := map[int64]bool{}
	if ans.Kind == "scratch" && ans.Answer == "reroll" {
		animate[unit] = true
	}
	s.aftermathAnswer(w, r, http.StatusOK, base, m, co, a, "", false, func(v *aftermathView) { v.Animate = animate; v.ShowFinish = v.ShowFinish || ans.Kind == "reward" })
}

// handleFinishAftermath applies the influence and rewards and unlocks the company. A refusal (a decision is still open) is shown
// on the Finish step.
func (s *Server) handleFinishAftermath(w http.ResponseWriter, r *http.Request) {
	base, m, co, ok := s.aftermathCtx(w, r)
	if !ok {
		return
	}
	a, err := s.campaigns.FinishAftermath(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID, co.ID, revisionOf(r))
	if err != nil {
		if errors.Is(err, core.ErrRefused) || errors.Is(err, core.ErrInvalidAftermath) {
			cur, gerr := s.campaigns.Aftermath(r.Context(), *userFrom(r.Context()), base.Campaign.ID, m.ID, co.ID)
			if s.fail(w, r, gerr) {
				return
			}
			if cur.Access == core.AftermathEdit {
				s.aftermathAnswer(w, r, http.StatusUnprocessableEntity, base, m, co, cur, reason(err), true, func(v *aftermathView) { v.ShowFinish = true })
				return
			}
		}
		s.aftermathFailed(w, r, base, m, co, err, nil)
		return
	}
	s.aftermathAnswer(w, r, http.StatusOK, base, m, co, a, "Aftermath finished. The company is unlocked.", false, nil)
}

// ---------------------------------------------------------------------------------------------------------------------
// Perks and history of a company (docs/aftermath.md, "Company pages")

// perkView is a scenario perk as the card shows it.
type perkView struct {
	core.Perk
	Unit string // the relic bearer's name, empty for a company perk
	Used bool
}

// WhenText says when a perk applies.
func (p perkView) WhenText() string {
	switch p.When {
	case "firstReinforcementRollThisAftermath":
		return "On the first reinforcement roll of the aftermath"
	case "nextReinforcementRoll":
		return "On the next reinforcement roll"
	case "nextScenarioRoll", "nextScenarioTableRoll":
		return "On the next scenario roll"
	case "nextGame":
		return "In the next game"
	}
	return ""
}

func perkViews(perks []core.Perk, roster []core.CompanyUnit) []perkView {
	out := make([]perkView, len(perks))
	for i, p := range perks {
		out[i] = perkView{Perk: p, Used: p.UsedOn != nil}
		for _, u := range roster {
			if u.ID == p.UnitID && p.UnitID != 0 {
				out[i].Unit = u.Title()
			}
		}
	}
	return out
}

// Unused and Used split the perks: the card lists unused ones and keeps the used ones under a collapsed heading.
func splitPerks(all []perkView) (unused, used []perkView) {
	for _, p := range all {
		if p.Used {
			used = append(used, p)
		} else {
			unused = append(unused, p)
		}
	}
	return unused, used
}

// handleUsePerk marks a perk used; only the company's owner does it.
func (s *Server) handleUsePerk(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "my-company")
	if !ok {
		return
	}
	perk, ok := s.idParam(w, r, "perk")
	if !ok {
		return
	}
	if base.Campaign.Company == nil {
		s.refuse(w, r, http.StatusForbidden, "You have no company in this campaign.")
		return
	}
	err := s.campaigns.UsePerk(r.Context(), *userFrom(r.Context()), base.Campaign.ID, base.Campaign.Company.ID, perk)
	switch {
	case errors.Is(err, core.ErrRefused):
		s.myCompanyPage(w, r, http.StatusUnprocessableEntity, reason(err), true)
	case s.fail(w, r, err):
	default:
		s.myCompanyPage(w, r, http.StatusOK, "Perk marked as used.", false)
	}
}
