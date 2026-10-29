package core

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// Against the Odds before the game (docs/matches.md): the lower rated company spends the Rating gap on bonuses after the scenario roll
// and before setting up. This file holds the rules read from the Battle Companies file, the selection, its validation and the card's
// types; the service is in ato_service.go, the storage is atoRepository.

// ATORules are the Against the Odds rules of the campaign's Battle Companies file (campaign.againstTheOdds, data.wanderers).
type ATORules struct {
	Inclusive bool           // budget.inclusive: the bonuses may take the company exactly up to its opponent's Rating
	Bonuses   []ATOBonusRule // in the file's order
	Kits      []ATOKitRule
	MaxKits   int // toolKit.purchase.maxPurchases
	Wanderers []ATOWandererRule
}

// The keys of the bonuses the app gives a structure to (the others, reroll, influence and experience, are plain switches).
const (
	atoToolKit  = "toolKit"
	atoWanderer = "wanderer"
	atoAmbush   = "ambush"
)

// ATOPracticeReason is why a bonus that only pays after the match is disabled in a practice match.
const ATOPracticeReason = "No reward in a practice match"

// atoRewardBonus says whether the bonus only acts as a reward after the match (influence, experience, re-rolls): a practice match
// awards nothing, so these cannot be taken there. The table effects (kits, Wanderer, Ambush!) stay.
func atoRewardBonus(key string) bool {
	return key == "influence" || key == "experience" || key == "reroll"
}

// ATOBonusRule is a bonus of campaign.againstTheOdds.bonuses.
type ATOBonusRule struct {
	Key, Name, Text string
	Rating          int      // Tool Kit: per kit
	GivesRerolls    int      // Reroll: how many re-rolls it gives (the bonus's count)
	AppliesIn       []string // setup | game | aftermath | scenarioChoice
}

// ATOKitRule is a kit of the Tool Kit bonus; Key is made from its name ("Healer's Kit" is "healer").
type ATOKitRule struct {
	Key, Name string
	Items     []ATOKitItem
}

// ATOKitItem is an item of a kit and how many copies it has; HeroOnly follows the equipment's restrictions.
type ATOKitItem struct {
	Item     string
	Count    int
	HeroOnly bool
	Detail   bool // the item names a weapon it is on (Envenom Weapon (X))
	// AfterGame: the item acts in the War's Aftermath (it activates an aftermath effect: Healing Herbs, Wondrous Cram), not during the game.
	AfterGame bool
}

// ATOWandererRule is a Wanderer of data.wanderers.items that can be recruited for a match.
type ATOWandererRule struct {
	Name, Summary string
	OwnPrice      int // its Rating as a permanent Wanderer (shown crossed out)
	// The trackers it starts with and its Courage, for the live match.
	Wounds, Might, Will, Fate, Courage int
}

// kitKey makes the key of a kit from its name.
func kitKey(name string) string {
	first, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(name)), " ")
	return strings.TrimSuffix(first, "'s")
}

// Bonus finds a bonus by key.
func (r ATORules) Bonus(key string) (ATOBonusRule, bool) {
	i := slices.IndexFunc(r.Bonuses, func(b ATOBonusRule) bool { return b.Key == key })
	if i < 0 {
		return ATOBonusRule{}, false
	}
	return r.Bonuses[i], true
}

// Kit finds a kit by key.
func (r ATORules) Kit(key string) (ATOKitRule, bool) {
	i := slices.IndexFunc(r.Kits, func(k ATOKitRule) bool { return k.Key == key })
	if i < 0 {
		return ATOKitRule{}, false
	}
	return r.Kits[i], true
}

// Wanderer finds a Wanderer by name.
func (r ATORules) Wanderer(name string) (ATOWandererRule, bool) {
	i := slices.IndexFunc(r.Wanderers, func(w ATOWandererRule) bool { return w.Name == name })
	if i < 0 {
		return ATOWandererRule{}, false
	}
	return r.Wanderers[i], true
}

// LeanATO reads the Against the Odds rules of a lean document: the bonuses of campaign.againstTheOdds (the table as the file stores it
// until a parameter slice types it), the equipment the kits name and the wanderers a match may recruit.
func LeanATO(d *lean.Document, vanilla *VanillaBook) (ATORules, error) {
	var ato struct {
		Budget struct {
			Inclusive bool `json:"inclusive"`
		} `json:"budget"`
		Bonuses []struct {
			Key       string   `json:"key"`
			Name      string   `json:"name"`
			Text      string   `json:"text"`
			Rating    int      `json:"rating"`
			Count     int      `json:"count"`
			AppliesIn []string `json:"appliesIn"`
			Purchase  struct {
				Max int `json:"maxPurchases"`
			} `json:"purchase"`
			Kits []struct {
				Name  string `json:"name"`
				Items []struct {
					Equipment string `json:"equipment"`
					Count     int    `json:"count"`
				} `json:"items"`
			} `json:"kits"`
		} `json:"bonuses"`
	}
	if err := leanRoundTrip(d.Campaign.AgainstTheOdds, &ato); err != nil {
		return ATORules{}, errors.Join(ErrInvalidRuleset, err)
	}
	heroOnly, afterGame := map[string]bool{}, map[string]bool{}
	for _, it := range d.Gear {
		if it.Kind == "equipment" {
			heroOnly[it.Name] = slices.Contains(it.Who.Badges(), "hero")
			afterGame[it.Name] = it.AftermathEffect != ""
		}
	}
	r := ATORules{Inclusive: ato.Budget.Inclusive}
	for _, b := range ato.Bonuses {
		r.Bonuses = append(r.Bonuses, ATOBonusRule{Key: b.Key, Name: b.Name, Text: b.Text, Rating: b.Rating, GivesRerolls: b.Count, AppliesIn: b.AppliesIn})
		if b.Key != atoToolKit {
			continue
		}
		r.MaxKits = b.Purchase.Max
		for _, k := range b.Kits {
			kit := ATOKitRule{Key: kitKey(k.Name), Name: k.Name}
			for _, it := range k.Items {
				kit.Items = append(kit.Items, ATOKitItem{Item: it.Equipment, Count: max(1, it.Count), HeroOnly: heroOnly[it.Equipment], Detail: strings.Contains(it.Equipment, "(X)"), AfterGame: afterGame[it.Equipment]})
			}
			r.Kits = append(r.Kits, kit)
		}
	}
	for _, g := range d.Gear {
		if g.Kind != "wanderer" {
			continue
		}
		w := wandererFrom(followerItem(g), vanilla)
		st := w.Stats
		r.Wanderers = append(r.Wanderers, ATOWandererRule{Name: w.Name, OwnPrice: w.Points, Wounds: st["wounds"], Might: st["might"], Will: st["will"], Fate: st["fate"], Courage: st["courage"],
			Summary: fmt.Sprintf("Fight %d, Strength %d, Defence %d, Attacks %d, Wounds %d, Courage %d", st["fight"], st["strength"], st["defence"], st["attack"], st["wounds"], st["courage"])})
	}
	if _, ok := r.Bonus(atoToolKit); ok && (len(r.Kits) == 0 || r.MaxKits < 1) {
		return ATORules{}, errors.Join(ErrInvalidRuleset, errors.New("the Tool Kit bonus has no kits"))
	}
	return r, nil
}

// ATOSelection is what the lower rated company took; it is saved as one document with the match.
type ATOSelection struct {
	Bonuses  []string        // campaign.againstTheOdds.bonuses[].key
	Kits     []string        // toolKit only: distinct kit keys
	Items    []KitAssignment // where each kit item went; fewer than the items are allowed (the rest is unassigned)
	Wanderer string          // wanderer only: the Wanderer's name
	Ambush   string          // ambush only: the rulebook scenario chosen
}

// KitAssignment gives one copy of a kit item to a model of the match.
type KitAssignment struct {
	Kit    string
	Item   string
	Index  int   // 0-based copy of the item inside the kit
	UnitID int64 // 0: unassigned
	Detail string
}

// Empty says whether nothing was chosen.
func (s ATOSelection) Empty() bool { return len(s.Bonuses) == 0 }

// Has says whether the bonus was taken.
func (s ATOSelection) Has(key string) bool { return slices.Contains(s.Bonuses, key) }

const maxKitDetail = 40

// Spent is the Rating the selection costs.
func (s ATOSelection) Spent(r ATORules) int {
	n := 0
	for _, key := range s.Bonuses {
		b, _ := r.Bonus(key)
		if key == atoToolKit {
			n += b.Rating * len(s.Kits)
		} else {
			n += b.Rating
		}
	}
	return n
}

// Validate checks the selection against the rules, the roster of the company (who may carry the kit items, whether it has a permanent
// Wanderer), the Rating gap and the scenarios Ambush! may choose from, and returns it cleaned: bonuses in the file's order, kits in
// the file's order, assignments sorted. A rejection wraps ErrInvalidMatch and its message is fit to show.
func (s ATOSelection) Validate(r ATORules, roster []CompanyUnit, gap int, scenarios []string) (ATOSelection, error) {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	var out ATOSelection
	seen := map[string]bool{}
	for _, key := range s.Bonuses {
		switch _, ok := r.Bonus(key); {
		case !ok:
			bad("%q is not an Against the Odds bonus", key)
		case seen[key]:
			bad("%s is chosen twice", key)
		default:
			seen[key] = true
		}
	}
	for _, b := range r.Bonuses {
		if seen[b.Key] {
			out.Bonuses = append(out.Bonuses, b.Key)
		}
	}
	// the Tool Kit: distinct kits, at least one and at most the file's maximum
	kits := map[string]bool{}
	for _, k := range s.Kits {
		switch _, ok := r.Kit(k); {
		case !ok:
			bad("%q is not a kit", k)
		case kits[k]:
			bad("you can only have one of each kit")
		default:
			kits[k] = true
		}
	}
	switch {
	case seen[atoToolKit] && len(kits) == 0:
		bad("choose at least one kit for the Tool Kit")
	case !seen[atoToolKit] && len(s.Kits) > 0:
		bad("kits need the Tool Kit bonus")
	case len(kits) > r.MaxKits:
		bad("a Tool Kit has at most %d kits", r.MaxKits)
	}
	for _, k := range r.Kits {
		if kits[k.Key] {
			out.Kits = append(out.Kits, k.Key)
		}
	}
	// the items go to models that take part; Hero-only items to Heroes
	type slot struct {
		kit, item string
		index     int
	}
	given := map[slot]bool{}
	for _, a := range s.Items {
		detail := strings.TrimSpace(a.Detail)
		kit, ok := r.Kit(a.Kit)
		switch {
		case !ok || !kits[a.Kit]:
			bad("%q is not one of the kits you chose", a.Kit)
			continue
		case !slices.ContainsFunc(kit.Items, func(it ATOKitItem) bool { return it.Item == a.Item && a.Index >= 0 && a.Index < it.Count }):
			bad("%s has no %s (copy %d)", kit.Name, a.Item, a.Index+1)
			continue
		case given[slot{a.Kit, a.Item, a.Index}]:
			bad("%s is assigned twice", a.Item)
			continue
		}
		given[slot{a.Kit, a.Item, a.Index}] = true
		i := slices.IndexFunc(kit.Items, func(it ATOKitItem) bool { return it.Item == a.Item })
		item := kit.Items[i]
		if utf8.RuneCountInString(detail) > maxKitDetail {
			bad("the weapon of %s can have at most %d characters", a.Item, maxKitDetail)
		}
		if detail != "" && !item.Detail {
			bad("%s takes no weapon", a.Item)
		}
		if a.UnitID != 0 {
			u, ok := rosterUnit(roster, a.UnitID)
			switch {
			case !ok:
				bad("%s: that model is not on the roster", a.Item)
			case Absent(u):
				bad("%s: %s misses this game", a.Item, u.Title())
			case item.HeroOnly && !u.IsHero():
				bad("%s can only go to a Hero, not to %s", a.Item, u.Title())
			}
		}
		out.Items = append(out.Items, KitAssignment{Kit: a.Kit, Item: a.Item, Index: a.Index, UnitID: a.UnitID, Detail: detail})
	}
	slices.SortFunc(out.Items, func(a, b KitAssignment) int {
		if c := strings.Compare(a.Kit, b.Kit); c != 0 {
			return c
		}
		if c := strings.Compare(a.Item, b.Item); c != 0 {
			return c
		}
		return a.Index - b.Index
	})
	// the Wanderer
	switch {
	case seen[atoWanderer]:
		if _, ok := r.Wanderer(s.Wanderer); !ok {
			bad("choose which Wanderer joins for the match")
		}
		if slices.ContainsFunc(roster, func(u CompanyUnit) bool { return u.Wanderer }) {
			bad("the company already has a Wanderer")
		}
		out.Wanderer = s.Wanderer
	case s.Wanderer != "":
		bad("a Wanderer needs the Wanderer bonus")
	}
	// Ambush!
	switch {
	case seen[atoAmbush]:
		if !slices.Contains(scenarios, s.Ambush) {
			bad("choose the scenario for Ambush!")
		}
		out.Ambush = s.Ambush
	case s.Ambush != "":
		bad("a scenario needs the Ambush! bonus")
	}
	// the budget
	if spent := out.Spent(r); spent > gap || (!r.Inclusive && spent == gap && spent > 0) {
		bad("the bonuses cost %d Rating and the gap is %d", spent, gap)
	}
	if len(errs) > 0 {
		return s, errors.Join(append([]error{ErrInvalidMatch}, errs...)...)
	}
	return out, nil
}

// ATOOption is one row of the card.
type ATOOption struct {
	Key, Name, Text string
	Rating          int
	Taken           bool
	Disabled        bool
	Reason          string // why it cannot be chosen now ("Needs 45, 10 left")
	OverBudget      bool   // the budget alone stops it (the page works that out live); any other Reason is a rule
	AppliesIn       []string
}

// ATOKit is a kit of the card.
type ATOKit struct {
	Key, Name string
	Items     []ATOKitItem
	Taken     bool
}

// ATOModel is a model that may carry a kit item: it takes part in the match.
type ATOModel struct {
	UnitID     int64
	Name, Type string
	Hero       bool
}

// ATOAccess is what a viewer may do with the card.
type ATOAccess int

const (
	ATONone ATOAccess = iota // read only
	ATOEdit
)

// ATOAccessFor: the owner of the (lower rated) company, the campaign's organiser and administrators edit while the match is open. The
// service adds the rules that need more than the match: the campaign is not finished and the game has not started.
func ATOAccessFor(u User, m Match, c CampaignCompany, organiserID int64) ATOAccess {
	if m.Ended {
		return ATONone
	}
	if CanManageCampaign(u, organiserID) || (c.OwnerID != 0 && c.OwnerID == u.ID) {
		return ATOEdit
	}
	return ATONone
}

// ATO is the whole state of the card.
type ATO struct {
	MatchID, CompanyID int64 // the lower rated company; 0 when not eligible
	CompanyName        string
	Eligible           bool
	Why                string // when not eligible: the sentence shown
	OwnRating          int
	OppRating          int
	OppName            string
	Gap                int
	Spent              int
	Options            []ATOOption
	Kits               []ATOKit
	Wanderers          []ATOWandererRule
	Models             []ATOModel
	Selection          ATOSelection
	Locked             bool // the game began
	LockedWhy          string
	Revision           int // the match's
	Access             ATOAccess
}

// ATORecord is the stored selection of a match.
type ATORecord struct {
	MatchID, CompanyID int64
	Selection          ATOSelection
	// Previous is the scenario the match had before Ambush! replaced it (nil: it had none); HadPrevious tells the two apart from "no record".
	Previous    *MatchScenario
	HadPrevious bool
}

// LoanedFor lists the items the record's Tool Kit put on models of the company for this match (assigned ones only). AfterGame is set for
// the items that activate an aftermath effect.
func (r ATORules) LoanedFor(rec ATORecord, companyID int64) []LoanedItem {
	if rec.CompanyID != companyID || rec.CompanyID == 0 || !rec.Selection.Has(atoToolKit) {
		return nil
	}
	var out []LoanedItem
	for _, a := range rec.Selection.Items {
		if a.UnitID == 0 {
			continue
		}
		kit, _ := r.Kit(a.Kit)
		item := ATOKitItem{}
		for _, it := range kit.Items {
			if it.Item == a.Item {
				item = it
			}
		}
		out = append(out, LoanedItem{UnitID: a.UnitID, Item: a.Item, Kit: kit.Name, Detail: a.Detail, Index: a.Index, AfterGame: item.AfterGame})
	}
	return out
}
