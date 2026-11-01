package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

type armoryView struct {
	campaignBase
	Company     core.CompanyDetail
	HasCompany  bool
	Entries     []armoryRow
	Creatures   []armoryCreature
	Wanderers   []armoryWanderer
	Subs        []armorySub        // the item categories that exist, in display order
	Weapons     map[int64][]string // per unit: the weapons Poisoned Attacks could still be given to (Envenom Weapon asks for one)
	Applies     string             // JSON: the item categories each main category has, for the filter chips
	Notice      string
	NoticeError bool
}

// HiddenByDefault is how many items, creatures and wanderers the armoury's switch hides before anyone touches it ("Hide what the
// company could never take" is on by default): the page is drawn that way before Alpine starts, so nothing moves when it does.
func (v armoryView) HiddenByDefault() int {
	n := 0
	for _, e := range v.Entries {
		if !e.Avail {
			n++
		}
	}
	for _, c := range v.Creatures {
		if !c.Avail {
			n++
		}
	}
	for _, w := range v.Wanderers {
		if !w.Avail {
			n++
		}
	}
	return n
}

// tipChip is a name on an armory card with the text of its tooltip (a special rule, an item, a heroic action); the text is empty
// when no file explains the name.
type tipChip struct {
	Name, Title, Text, Meta, TermID string
}

// armoryRow is an item of the armory with the place it has in the page's filters.
type armoryRow struct {
	core.ArmouryEntry
	Main  string // wargear | equipment
	Sub   string // the item category (armorySub.Key)
	Avail bool   // possible in this company (core.ArmouryEntry.Possible): some unit that can exist in it could buy it, now or later
}

type armoryCreature struct {
	core.ArmouryCreature
	Avail     bool // possible in this company: the rules give an officer a creature
	RuleChips []tipChip
}

// Why is the sentence for a creature nobody can take now: what stops each officer.
func (c armoryCreature) Why() string {
	var reasons []string
	for _, o := range c.Owners {
		if o.Reason != "" && !slices.Contains(reasons, o.Reason) {
			reasons = append(reasons, o.Reason)
		}
	}
	if len(reasons) == 0 {
		return "Nobody can take it now."
	}
	return "Nobody can take it now: " + core.LowerFirst(strings.Join(reasons, "; ")) + "."
}

type armoryWanderer struct {
	core.ArmouryWanderer
	Avail                                bool
	RuleChips, WargearChips, ActionChips []tipChip
}

// armorySub is an item category of the filters.
type armorySub struct{ Key, Label string }

var armorySubs = []armorySub{
	{"melee", "Melee"}, {"ranged", "Ranged"}, {"armour", "Armour and shields"}, {"mount", "Mounts"},
	{"consumable", "Consumables"}, {"banner", "Banners and horns"}, {"other", "Other"},
}

// subOf is the item category of an item, by its first class.
func subOf(classes []core.EquipmentClass) string {
	if len(classes) == 0 {
		return "other"
	}
	switch classes[0] {
	case core.EquipMelee, core.EquipTwoHanded, core.EquipSpear, core.EquipLance:
		return "melee"
	case core.EquipBow, core.EquipCrossbow, core.EquipThrowing, core.EquipMissile:
		return "ranged"
	case core.EquipArmour, core.EquipShield:
		return "armour"
	case core.EquipMount:
		return "mount"
	case core.EquipConsumable:
		return "consumable"
	case core.EquipBanner, core.EquipHorn:
		return "banner"
	}
	return "other"
}

// armoryRows places the items in the filters. An item is out of reach when no unit that can exist in the company could ever buy it
// (the goat of another company's riders, a Fell Warg for a company without wargs); lacking influence, a free slot, or every unit
// already carrying it is not: core decides (ArmouryEntry.Possible).
func armoryRows(entries []core.ArmouryEntry) ([]armoryRow, []armorySub, string) {
	has := map[string]map[string]bool{"wargear": {}, "equipment": {}, "creatures": {}, "wanderers": {}}
	rows := make([]armoryRow, len(entries))
	for i, e := range entries {
		r := armoryRow{ArmouryEntry: e, Main: "wargear", Sub: subOf(e.Classes), Avail: e.Possible}
		if e.Size != "" {
			r.Main = "equipment"
		}
		has[r.Main][r.Sub] = true
		rows[i] = r
	}
	var subs []armorySub
	applies := map[string][]string{}
	for m, set := range has {
		applies[m] = []string{}
		for _, sb := range armorySubs {
			if set[sb.Key] {
				applies[m] = append(applies[m], sb.Key)
			}
		}
	}
	for _, sb := range armorySubs {
		if slices.Contains(applies["wargear"], sb.Key) || slices.Contains(applies["equipment"], sb.Key) {
			subs = append(subs, sb)
		}
	}
	b, _ := json.Marshal(applies)
	return rows, subs, string(b)
}

// armoryTips explains the names on the creature and wanderer cards: an item by the armory's own text (or its alias), a rule or a
// heroic action by the glossary.
type armoryTips struct {
	glossary core.Glossary
	items    map[string]core.ArmouryEntry
}

func newArmoryTips(g core.Glossary, entries []core.ArmouryEntry) armoryTips {
	t := armoryTips{glossary: g, items: map[string]core.ArmouryEntry{}}
	for _, e := range entries {
		for _, n := range append([]string{e.Name}, e.AlsoCalled...) {
			t.items[strings.ToLower(n)] = e
		}
	}
	return t
}

func (t armoryTips) chip(name, kind string) tipChip {
	c := tipChip{Name: name, Title: name, Meta: kind}
	if e, ok := t.items[strings.ToLower(name)]; ok && e.Description != "" {
		c.Text = e.Description
		if e.Stats != "" {
			c.Meta = kind + " · " + e.Stats
		}
		return c
	}
	if term, ok := t.glossary.Find(name); ok {
		c.TermID, c.Title, c.Text = term.ID, term.Name, term.Summary(520)
	}
	return c
}

func (t armoryTips) chips(names []string, kind string) []tipChip {
	var out []tipChip
	for _, n := range names {
		out = append(out, t.chip(n, kind))
	}
	return out
}

// armoryPage renders the Armory tab: every item the company's armoury sells, with who could buy it.
func (s *Server) armoryPage(w http.ResponseWriter, r *http.Request, status int, notice string, noticeErr bool) {
	base, ok := s.loadCampaign(w, r, "armory")
	if !ok {
		return
	}
	if len(base.Campaign.Companies) > 0 { // the company is named by ?company= (and by the hidden field of the buy forms)
		co, ok := s.ownCompany(w, r, base.Campaign)
		if !ok {
			return
		}
		base.Current = &co
	}
	view := armoryView{campaignBase: base, Notice: notice, NoticeError: noticeErr}
	if mine := base.Current; mine != nil {
		detail, err := s.campaigns.CompanyDetail(r.Context(), base.Campaign.ID, mine.ID)
		if s.fail(w, r, err) {
			return
		}
		entries, err := s.campaigns.ArmouryEntries(r.Context(), base.Campaign.ID, mine.ID)
		if s.fail(w, r, err) {
			return
		}
		view.Company, view.HasCompany = detail, true
		view.Weapons = map[int64][]string{}
		for _, u := range detail.Roster {
			view.Weapons[u.ID] = u.PoisonableWeapons
		}
		view.Entries, view.Subs, view.Applies = armoryRows(entries)
		g, err := s.glossary.ForCampaign(r.Context(), base.Campaign.ID) // rules are explained by the campaign's glossary
		if err != nil && !errors.Is(err, core.ErrNotFound) {
			s.fail(w, r, err)
			return
		}
		tips := newArmoryTips(g, entries)
		creatures, err := s.campaigns.ArmouryCreatures(r.Context(), base.Campaign.ID, mine.ID)
		if s.fail(w, r, err) {
			return
		}
		for _, c := range creatures {
			view.Creatures = append(view.Creatures, armoryCreature{ArmouryCreature: c, Avail: c.Possible, RuleChips: tips.chips(c.Rules, "Special rule")})
		}
		wanderers, err := s.campaigns.ArmouryWanderers(r.Context(), base.Campaign.ID, mine.ID)
		if s.fail(w, r, err) {
			return
		}
		for _, wd := range wanderers {
			view.Wanderers = append(view.Wanderers, armoryWanderer{ArmouryWanderer: wd, Avail: wd.Possible,
				RuleChips: tips.chips(wd.Rules, "Special rule"), WargearChips: tips.chips(wd.Wargear, "Wargear"), ActionChips: tips.chips(wd.HeroicActions, "Heroic action")})
		}
	}
	s.render.page(w, r, status, "campaign_armory", view)
}

func (s *Server) handleArmory(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("company") == "" { // the bare tab URL: go to the armoury of the company opened last (the first until the cookie)
		base, ok := s.loadCampaign(w, r, "armory")
		if !ok {
			return
		}
		if len(base.Campaign.Companies) > 0 {
			path := base.ArmoryPath(base.Campaign.Companies[0].ID)
			if boostedLink(r) { // a click: answer with the armoury now instead of a redirect (see boostedLink)
				target := r.Clone(r.Context())
				target.URL.RawQuery = "company=" + itoa(int(base.Campaign.Companies[0].ID))
				target.Form, target.PostForm = nil, nil
				w.Header().Set("HX-Push-Url", path)
				s.armoryPage(w, target, http.StatusOK, "", false)
				return
			}
			http.Redirect(w, r, path, http.StatusSeeOther)
			return
		}
	}
	s.armoryPage(w, r, http.StatusOK, "", false)
}

// handleArmoryBuy buys an item for the unit chosen in the armory and shows the armory again.
func (s *Server) handleArmoryBuy(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	cid, ok := s.ownCompanyID(w, r, id)
	if !ok {
		return
	}
	unitID, _ := strconv.ParseInt(r.PostFormValue("unit"), 10, 64)
	item := r.PostFormValue("item")
	err := s.buyEquipment(r, id, cid, unitID, item)
	switch {
	case errors.Is(err, core.ErrRefused):
		s.armoryPage(w, r, http.StatusUnprocessableEntity, reason(err), true)
	case s.fail(w, r, err):
	default:
		s.armoryPage(w, r, http.StatusOK, "Bought "+item+".", false)
	}
}
