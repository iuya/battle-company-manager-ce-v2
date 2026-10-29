package core

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrMatchChanged is returned by a write made on a stale Match.Revision: somebody else changed the match meanwhile.
var ErrMatchChanged = errors.New("the match changed")

// Limits of the free text of a match (characters).
const (
	maxScenarioName = 60
	maxScenarioText = 1000
	maxResultNote   = 500
	maxVictoryPts   = 99
)

// ScenarioKind says where a match's scenario comes from.
type ScenarioKind string

const (
	ScenarioRulebook ScenarioKind = "rulebook" // a scenario of the campaign's Battle Companies file
	ScenarioCustom   ScenarioKind = "custom"   // a one-off, free text
)

// MatchScenario is the scenario of one match. Name is the rules file's name for a rulebook scenario (matched exactly) and
// the players' title for a custom one.
type MatchScenario struct {
	Kind        ScenarioKind
	Name        string // 1-60 characters
	Description string // custom only, 0-1000 characters, plain text
	Victory     string // custom only: how it is won, 0-1000 characters, plain text
}

// ScenarioInput is what the picker or the one-off form submits. Kind decides which fields count.
type ScenarioInput = MatchScenario

// Validate trims the input and checks it; a rejection is ErrInvalidMatch (joined), its message fit to show. A rulebook
// scenario keeps only its name.
func (in ScenarioInput) Validate() (ScenarioInput, error) {
	in.Name, in.Description, in.Victory = strings.TrimSpace(in.Name), strings.TrimSpace(in.Description), strings.TrimSpace(in.Victory)
	var errs []error
	switch in.Kind {
	case ScenarioRulebook:
		in.Description, in.Victory = "", ""
	case ScenarioCustom:
	default:
		return in, errors.Join(ErrInvalidMatch, errors.New("choose a rulebook scenario or describe a one-off one"))
	}
	if n := utf8.RuneCountInString(in.Name); n == 0 || n > maxScenarioName {
		errs = append(errs, fieldError("name", "the name must be between 1 and 60 characters"))
	}
	if utf8.RuneCountInString(in.Description) > maxScenarioText {
		errs = append(errs, fieldError("description", "the description can have at most 1000 characters"))
	}
	if utf8.RuneCountInString(in.Victory) > maxScenarioText {
		errs = append(errs, fieldError("victory", "how it is won can have at most 1000 characters"))
	}
	if len(errs) > 0 {
		return in, errors.Join(append([]error{ErrInvalidMatch}, errs...)...)
	}
	return in, nil
}

// SideResult is what is reported for one side.
type SideResult struct {
	VictoryPoints *int // nil: not reported; 0-99
}

// CompanyResult is what is reported for one company of the match.
type CompanyResult struct {
	CompanyID  int64
	Fled       bool           // All is Lost: left the battlefield once Broken; the opponent claims the win
	Objectives map[string]int // objective key (rules file) -> times completed (0 or absent: not completed)
}

// MatchResult is the whole report. Winner uses Match.Winner's convention (1-based side, 0 a draw) once Decided; SaveResult
// also sets Match.Winner.
type MatchResult struct {
	Decided   bool // the players reported an outcome; false means nothing reported yet (Winner 0 is then not a draw)
	Winner    int
	Sides     []SideResult    // same order as Match.Sides
	Companies []CompanyResult // zero or one per company of the match
	Note      string          // 0-500 characters: "a word about the game" shown on the match page
}

// Company returns what was reported for a company (the zero value when nothing was).
func (r MatchResult) Company(id int64) CompanyResult {
	for _, c := range r.Companies {
		if c.CompanyID == id {
			return c
		}
	}
	return CompanyResult{CompanyID: id}
}

// SideVP is the Victory Points reported for side i (0-based), nil when none were.
func (m Match) SideVP(i int) *int {
	if i < 0 || i >= len(m.Result.Sides) {
		return nil
	}
	return m.Result.Sides[i].VictoryPoints
}

// ReportedWinner is the name of the side the players reported as the winner, empty for a draw or nothing reported yet.
func (m Match) ReportedWinner() string {
	if m.Result.Decided && m.Result.Winner >= 1 && m.Result.Winner <= len(m.Sides) {
		return m.Sides[m.Result.Winner-1].Name
	}
	return ""
}

// MatchAccess is what a viewer may do with a match.
type MatchAccess int

const (
	MatchNoAccess  MatchAccess = iota // read only
	MatchEdit                         // open match: participant, organiser or administrator
	MatchEditEnded                    // ended match: administrator or the campaign's organiser
)

// Access holds the editing rule so every backend and the page apply the same one: the owners of the companies in the
// match, the campaign's organiser and administrators edit an open match; only a manager of the campaign (the organiser or an
// administrator, CanManageCampaign) edits an ended one.
func (m Match) Access(u User, organiserID int64) MatchAccess {
	manager := CanManageCampaign(u, organiserID)
	if m.Ended {
		if manager {
			return MatchEditEnded
		}
		return MatchNoAccess
	}
	if manager {
		return MatchEdit
	}
	for _, s := range m.Sides {
		for _, c := range s.Companies {
			if c.OwnerID == u.ID {
				return MatchEdit
			}
		}
	}
	return MatchNoAccess
}

// FleeConflict is "" or why the reported winner contradicts a Fled company: a company that left the battlefield cannot be on
// the winning side, its opponent claims the win.
func (m Match) FleeConflict() string {
	if !m.Result.Decided || m.Result.Winner < 1 || m.Result.Winner > len(m.Sides) {
		return ""
	}
	for _, c := range m.Sides[m.Result.Winner-1].Companies {
		if m.Result.Company(c.ID).Fled {
			return c.Name + " fled the battlefield, so " + m.Sides[m.Result.Winner-1].Name + " cannot be the winner."
		}
	}
	return ""
}

// CanEnd is nil, or ErrRefused with the reason the match cannot be ended yet.
func (m Match) CanEnd() error {
	switch {
	case m.Ended:
		return Refuse("This match has already ended.")
	case m.Scenario == nil:
		return Refuse("Choose the scenario before ending the match.")
	case !m.Result.Decided:
		return Refuse("Report who won, or a draw, before ending the match.")
	}
	if why := m.FleeConflict(); why != "" {
		return Refuse("%s", why)
	}
	return nil
}

// ValidateResult trims and checks a reported result against the match and, for a rulebook scenario, the scenario's
// objectives (info; nil when the rules file does not know the scenario). Objectives of a custom scenario are rejected, zero
// counts dropped, an undecided result has no winner. The returned result is what to store.
func (m Match) ValidateResult(r MatchResult, info *ScenarioInfo) (MatchResult, error) {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	r.Note = strings.TrimSpace(r.Note)
	if utf8.RuneCountInString(r.Note) > maxResultNote {
		bad("the note can have at most 500 characters")
	}
	if r.Winner < 0 || r.Winner > len(m.Sides) {
		bad("the winner must be one of the sides or a draw")
	}
	if !r.Decided {
		r.Winner = 0
	}
	if len(r.Sides) != len(m.Sides) {
		bad("the result needs one entry per side")
	}
	for _, s := range r.Sides {
		if s.VictoryPoints != nil && (*s.VictoryPoints < 0 || *s.VictoryPoints > maxVictoryPts) {
			bad("Victory Points must be between 0 and 99")
			break
		}
	}
	companies := make([]CompanyResult, 0, len(r.Companies))
	seen := map[int64]bool{}
	for _, c := range r.Companies {
		if !m.hasCompany(c.CompanyID) || seen[c.CompanyID] {
			bad("a company in the result is not in this match")
			continue
		}
		seen[c.CompanyID] = true
		objs := map[string]int{}
		for key, n := range c.Objectives {
			if n == 0 {
				continue
			}
			max := 0
			if m.Scenario != nil && m.Scenario.Kind == ScenarioRulebook && info != nil {
				for _, o := range info.Objectives {
					if o.Key == key {
						max = o.Max
					}
				}
			}
			if max == 0 {
				bad("%q is not an objective of this scenario", key)
			} else if n < 0 || n > max {
				bad("%q can be completed 0 to %d times", key, max)
			} else {
				objs[key] = n
			}
		}
		if len(objs) == 0 {
			objs = nil
		}
		companies = append(companies, CompanyResult{CompanyID: c.CompanyID, Fled: c.Fled, Objectives: objs})
	}
	if len(errs) > 0 {
		return r, errors.Join(append([]error{ErrInvalidMatch}, errs...)...)
	}
	r.Companies = companies
	return r, nil
}

func (m Match) hasCompany(id int64) bool {
	return slices.ContainsFunc(m.Sides, func(s MatchSide) bool {
		return slices.ContainsFunc(s.Companies, func(c CampaignCompany) bool { return c.ID == id })
	})
}

// ScenarioInfo is a scenario of the rules file as the picker and the match page show it.
type ScenarioInfo struct {
	Name       string
	Table      int                 // 1-3 (the scenario selection table it is on); 0 when the file does not place it
	Roll       int                 // 1..Sides on that table's second die; 0 when unknown
	FirstDie   [2]int              // the range of the first die that picks its table (campaign.scenarioSelection.tables[].roll)
	Objectives []ScenarioObjective // the ticks of the scenario's rewards; empty for most scenarios
	Scoring    []string            // text of the Victory Point conditions, shown as a read-only reminder
	About      ScenarioAbout       // what the rulebook says about the set-up and the special rules
	Prizes     []ScenarioPrize     // rewards that follow from the result (the winner, a surviving Leader); not ticks
	// ScoringObjectives are the Victory Point conditions as data, in the printed order (data.scenarioDetails[].scoringObjectives).
	ScoringObjectives []ScoringObjective
}

// HasRoles says whether the scenario has an Attacker and a Defender.
func (si ScenarioInfo) HasRoles() bool { return len(si.About.Roles) > 0 }

// ObjectivesFor lists the scoring objectives a company with that tag scores: those for both sides and for its role. Without a tag
// (role "") all are listed.
func (si ScenarioInfo) ObjectivesFor(role string) []ScoringObjective {
	var out []ScoringObjective
	for _, o := range si.ScoringObjectives {
		if role == "" || o.Side == "" || o.Side == "both" || o.Side == role {
			out = append(out, o)
		}
	}
	return out
}

// ScoringObjective is one Victory Point condition of a scenario. Side is "both", "attacker" or "defender". A Secret one (Fog of War)
// is kept hidden until the end of the game; a Nomination says what the player notes down at the start.
type ScoringObjective struct {
	Key, Text, Side, Kind, Instruction string
	Label                              string // a short name for lists and the log: "Own hero survives"
	MaxPoints                          int
	Secret                             bool
	Nomination                         *ObjectiveNomination
	Options                            []ScoringTier // the tiers, ascending
}

// ObjectiveLabel makes a short name out of an objective's key ("ownHeroSurvives" is "Own hero survives").
func ObjectiveLabel(key string) string {
	var b strings.Builder
	for i, r := range key {
		switch {
		case i == 0:
			b.WriteRune(unicode.ToUpper(r))
		case unicode.IsUpper(r):
			b.WriteRune(' ')
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ObjectiveNomination: From is "ownHeroes", "enemyHeroes" or "terrain"; AllowLeader "onlyHero" (the Leader only when the company has one Hero);
// Where "opponentHalf" for a terrain piece.
type ObjectiveNomination struct{ From, AllowLeader, Where string }

// ScoringTier is a reachable level of a scoring objective.
type ScoringTier struct {
	Text   string
	Points int
}

// ScenarioAbout is the rulebook text of a scenario that is not about scoring: the page shows it folded.
type ScenarioAbout struct {
	Outline, Description string
	Sections             []ScenarioText // Armies, Layout, Starting positions, How the game ends (those the file has)
	Rules                []ScenarioText // the scenario's special rules, by name
	Roles                []string       // Attacker, Defender, when the scenario has them
	Uses                 []string       // rules the scenario applies without text of their own (Maelstrom of Battle): shown by name
}

// ScenarioText is a titled paragraph of rulebook text (paragraphs separated by blank lines).
type ScenarioText struct {
	Title, Text string
	Table       *ScenarioRuleTable // a table the rule prints, after its text (Strange Aura, the Monster Budget)
}

// ScenarioRuleTable is a table of a scenario special rule. Lookup says what selects a row: a dice string such as "d6" (rolled at the table) or
// "combinedRating" (the two companies' ratings added, known when the match is made).
type ScenarioRuleTable struct {
	Lookup    string
	Columns   []string
	Rows      []ScenarioTableRow
	TableDice DiceSpec // the dice of a Lookup that is a dice string ("d6"); the rows cover Min..Max of it. Zero for combinedRating
}

// ScenarioTableRow covers the values Min to Max (Max below zero: open ended) and prints its Cells.
type ScenarioTableRow struct {
	Min, Max int
	Cells    []string
}

// RowFor is the index of the row that covers value, or -1.
func (t ScenarioRuleTable) RowFor(value int) int {
	for i, r := range t.Rows {
		if value >= r.Min && (r.Max < 0 || value <= r.Max) {
			return i
		}
	}
	return -1
}

// ScenarioPrize is a reward the aftermath hands out because of the result (Rendezvous for the victor of Chance Encounter).
// Who says who earns it ("The winner"); the empty string when the file names nobody the page knows.
type ScenarioPrize struct{ Name, Who, Text string }

// ScenarioObjective is one tick on the match page, per company: a reward condition of the rules file the players tick
// (Key and Label are the condition's, Max is 1). Rewards is what completing it earns, for the preview.
type ScenarioObjective struct {
	Key, Label string
	Max        int  // 1 for a tick
	Model      bool // a unit holds it (a modelFlag condition): the result form ticks it company-wide, the aftermath sheet says which unit
	Rewards    []ScenarioReward
}

// ScenarioReward is what completing an objective earns. Kind: gold, influence, experience, equipment, other.
type ScenarioReward struct {
	Kind   string
	Amount int
	Text   string
}

// Preview is what the match page says completing the objective earns: an amount for Influence and Experience, the rules
// file's wording for the rest.
func (r ScenarioReward) Preview() string {
	switch {
	case r.Kind == "influence" && r.Amount != 0:
		return fmt.Sprintf("%d Influence", r.Amount)
	case r.Kind == "experience" && r.Amount != 0:
		return fmt.Sprintf("%d Experience", r.Amount)
	case r.Kind == "gold" && r.Amount != 0:
		return fmt.Sprintf("%d Gold", r.Amount)
	}
	return r.Text
}

// NoAccessReason is why a viewer who cannot edit the match may not: it has ended (the organiser or an administrator can still correct it) or
// they are not one of its players.
func (m Match) NoAccessReason() string {
	if m.Ended {
		return "This match has ended; ask the organiser or an administrator."
	}
	return "Only the players in this match can change it."
}

// SamePlayer says that one player owns several companies of a match (several companies per player are allowed in a campaign). It is a
// note for the match setup and the match page, never a refusal: ValidateSides allows it, a player's own companies may share a side or
// face each other (docs/multi-company-spec.md, decision 2).
type SamePlayer struct {
	OwnerID int64
	Owner   string
	// Companies is the number of companies of the player in the match.
	Companies int
	// SameSide lists the 0-based sides that hold two or more of the player's companies.
	SameSide []int
	// AcrossSides is true when the player has companies on at least two different sides.
	AcrossSides bool
}

// SamePlayers lists the players who own two or more companies among the sides, in the order of their first company.
func SamePlayers(sides []MatchSide) []SamePlayer {
	type tally struct {
		SamePlayer
		perSide map[int]int
	}
	var order []int64
	by := map[int64]*tally{}
	for i, s := range sides {
		for _, c := range s.Companies {
			t := by[c.OwnerID]
			if t == nil {
				t = &tally{SamePlayer: SamePlayer{OwnerID: c.OwnerID, Owner: c.Owner}, perSide: map[int]int{}}
				by[c.OwnerID] = t
				order = append(order, c.OwnerID)
			}
			t.Companies++
			t.perSide[i]++
		}
	}
	var out []SamePlayer
	for _, id := range order {
		t := by[id]
		if t.Companies < 2 {
			continue
		}
		for i := range sides {
			if t.perSide[i] >= 2 {
				t.SameSide = append(t.SameSide, i)
			}
		}
		t.AcrossSides = len(t.perSide) > 1
		out = append(out, t.SamePlayer)
	}
	return out
}

// SamePlayers is SamePlayers of the match's sides.
func (m Match) SamePlayers() []SamePlayer { return SamePlayers(m.Sides) }
