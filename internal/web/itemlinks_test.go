package web

import (
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

var (
	itemWarSpear  = "Counts as a lance while mounted and a spear while dismounted."
	itemBoots     = "The model gains Mountain Dweller."
	itemDrum      = "A war drum lets its bearer declare a Heroic March."
	itemBackpack  = "A model with a Backpack may carry one more piece of equipment."
	termLinkTexts = regexp.MustCompile(`<a href="/campaigns/7/glossary#t-[^"]+" class="term-link"[^>]*>([^<]+)</a>`)
)

// linkTextsIn is the texts of the term links of the first <p>...</p> that holds text.
func linkTextsIn(html, text string) []string {
	i := strings.Index(html, text)
	if i < 0 {
		return nil
	}
	start := strings.LastIndex(html[:i], "<p ")
	end := i + strings.Index(html[i:], "</p>")
	var out []string
	for _, m := range termLinkTexts.FindAllStringSubmatch(html[start:end], -1) {
		out = append(out, strings.ToLower(m[1]))
	}
	return out
}

// itemSegments links the glossary terms of an item's description, never the item's own name, and the longest phrase wins.
func TestItemTextLinksTermsButNotTheItemsOwnName(t *testing.T) {
	g := shippedRules(t).GlossaryWithUnits()
	linked := func(text string, names ...string) []string {
		var out []string
		for _, s := range itemText(g, text, names...) {
			if s.TermID != "" || s.UnitID != "" {
				out = append(out, strings.ToLower(s.Text))
				if s.TipText == "" {
					t.Errorf("%q: no tooltip", s.Text)
				}
			}
		}
		return out
	}
	if got := strings.Join(linked(itemWarSpear, "War spear"), ","); got != "lance,spear" {
		t.Errorf("war spear: %q", got)
	}
	if got := strings.Join(linked(itemBoots, "Mountain boots"), ","); got != "mountain dweller" {
		t.Errorf("mountain boots: %q", got)
	}
	if got := strings.Join(linked(itemDrum, "Company drum"), ","); !strings.Contains(got, "war drum") {
		t.Errorf("company drum: %q", got)
	}
	if got := linked(itemBackpack, "Backpack"); slices.Contains(got, "backpack") {
		t.Errorf("the Backpack's own name is linked: %q", got)
	}
	if got := linked(itemBackpack, "Pack", "Backpack"); slices.Contains(got, "backpack") { // its other spellings count too
		t.Errorf("an alias is linked: %q", got)
	}
	if got := linked(itemBackpack); !slices.Contains(got, "backpack") { // but it is a term for any other item
		t.Errorf("another item's Backpack: %q", got)
	}
}

func TestArmoryTabLinksTheGlossaryInItemDescriptions(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(shippedRules(t).GlossaryWithUnits(), nil).AnyTimes()
	f.camp.EXPECT().ArmouryCreatures(gomock.Any(), int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().ArmouryWanderers(gomock.Any(), int64(7), int64(1)).Return(nil, nil).AnyTimes()
	unit := []core.UnitOption{{ID: 5, Title: "Hador", Type: "Warrior", Role: core.RoleSergeant}}
	f.camp.EXPECT().ArmouryEntries(gomock.Any(), int64(7), int64(1)).Return([]core.ArmouryEntry{
		{GearOffer: core.GearOffer{Name: "War spear", Classes: []core.EquipmentClass{core.EquipSpear}, Cost: 1, Description: itemWarSpear}, Units: unit, Possible: true},
		{GearOffer: core.GearOffer{Name: "Mountain boots", Classes: []core.EquipmentClass{core.EquipOther}, Cost: 1, Size: "small", Description: "Wearing them.", Grants: []string{"Mountain Dweller"}}, Units: unit, Possible: true},
		{GearOffer: core.GearOffer{Name: "Company drum", Classes: []core.EquipmentClass{core.EquipBanner}, Cost: 1, Size: "small", Description: itemDrum}, Units: unit, Possible: true},
		{GearOffer: core.GearOffer{Name: "Backpack", Classes: []core.EquipmentClass{core.EquipOther}, Cost: 1, Size: "small", Description: itemBackpack}, Units: unit, Possible: true},
	}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/armory?company=1", nil))).Body.String()

	if got := strings.Join(linkTextsIn(body, "while mounted and a"), ","); got != "lance,spear" {
		t.Errorf("war spear links: %q", got)
	}
	if got := linkTextsIn(body, "Gives its mount:"); got != nil && len(got) > 1 {
		t.Errorf("unexpected links %q", got)
	}
	if !strings.Contains(body, `class="term-link" hx-boost="false" data-concept data-tip-title="Mountain Dweller"`) {
		t.Error("the Mountain Dweller the boots give is not a link with a tooltip")
	}
	if got := linkTextsIn(body, "lets its bearer"); len(got) == 0 || got[0] != "war drum" {
		t.Errorf("company drum links: %q", got)
	}
	if got := linkTextsIn(body, "A model with a Backpack"); slices.Contains(got, "backpack") {
		t.Errorf("the Backpack links its own name: %q", got)
	}
}

func TestArmouryWindowLinksTheGlossaryInItemDescriptions(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(shippedRules(t).GlossaryWithUnits(), nil).AnyTimes()
	f.camp.EXPECT().ArmouryCreatures(gomock.Any(), int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.camp.EXPECT().Armoury(gomock.Any(), int64(7), int64(1), int64(5)).Return([]core.ArmouryItem{
		{GearOffer: core.GearOffer{Name: "War spear", Classes: []core.EquipmentClass{core.EquipSpear}, Cost: 1, Description: itemWarSpear}, PayInfluence: true},
		{GearOffer: core.GearOffer{Name: "Backpack", Classes: []core.EquipmentClass{core.EquipOther}, Cost: 1, Size: "small", Description: itemBackpack}, PayInfluence: true},
	}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1/units/5/armoury", nil))).Body.String()
	if got := strings.Join(linkTextsIn(body, "while mounted and a"), ","); got != "lance,spear" {
		t.Errorf("war spear links: %q", got)
	}
	if got := linkTextsIn(body, "A model with a Backpack"); slices.Contains(got, "backpack") {
		t.Errorf("the Backpack links its own name: %q", got)
	}
}
