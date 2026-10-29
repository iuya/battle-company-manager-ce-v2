package core

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// The names payload with rings (ArmyNames v2, docs/aura-ranges.md section 4.2).

func ringKinds(f TTSFigure) []string {
	var out []string
	for _, r := range f.Auras {
		out = append(out, r.K)
	}
	return out
}

// TestTTSNamesV1StaysByteIdentical: without rings the payload is the version 1 of before: no "a", no "b", version 1, whatever the selection says.
func TestTTSNamesV1StaysByteIdentical(t *testing.T) {
	rs := shippedRules(t)
	for _, bcCo := range ttsCompanies(rs)[:6] {
		var roster []CompanyUnit
		for _, e := range ttsEntries(rs, bcCo) {
			if u := ttsOne(rs, bcCo, e).Roster[0]; len(rs.UnitAuras(u)) == 0 && len(roster) < 8 {
				u.ID = int64(len(roster) + 1)
				roster = append(roster, u)
			}
		}
		roster[0].Role = RoleWarrior
		d := CompanyDetail{CampaignCompany: CampaignCompany{Name: bcCo.Name, Faction: bcCo.Name}, Roster: roster}
		ex := rs.TTSList(d, TTSOptions{FootVersions: true, MountFigures: true})
		plain := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort})
		on := rs.DefaultAuraSelection()
		withSel := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort, Auras: on})
		a, _ := plain.JSON()
		b, _ := withSel.JSON()
		if !bytes.Equal(a, b) || plain.Version != 1 || withSel.Version != 1 {
			t.Errorf("%s: a company with no rings changed: v%d\n%s\n%s", bcCo.Name, withSel.Version, a, b)
		}
		// the shape of version 1, written out
		type v1Figure struct {
			Key        string `json:"k"`
			Line       string `json:"l"`
			Unit       int64  `json:"u"`
			Extra      string `json:"x"`
			Occurrence int    `json:"o"`
			Name       string `json:"n"`
			Desc       string `json:"d"`
		}
		old := struct {
			Version int        `json:"v"`
			Company string     `json:"company"`
			Figures []v1Figure `json:"figures"`
		}{1, plain.Company, nil}
		for _, f := range plain.Figures {
			old.Figures = append(old.Figures, v1Figure{f.Key, f.Line, f.Unit, f.Extra, f.Occurrence, f.Name, f.Desc})
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(old)
		if want := strings.TrimRight(buf.String(), "\n"); string(a) != want {
			t.Errorf("%s: version 1 is not byte identical to the old shape", bcCo.Name)
		}
	}
}

func rohanRingCompany(t *testing.T, rs *RuleSet) (CompanyDetail, TTSExport) {
	t.Helper()
	co := auraCompany(t, rs, "Rohan")
	lead := auraHeroOn(t, rs, "Rohan", "Warrior of Rohan", "Path of the Warlord", nil)
	lead.ID, lead.Role = 1, RoleLeader
	rider := buildUnit(rs, co, UnitState{ID: 2, Base: "Rider of Rohan", Role: RoleWarrior, Gear: []string{"Banner"}}, CampaignSettings{})
	foot := buildUnit(rs, co, UnitState{ID: 3, Base: "Warrior of Rohan", Role: RoleWarrior, Gear: []string{"Banner"}}, CampaignSettings{})
	plain := buildUnit(rs, co, UnitState{ID: 4, Base: "Warrior of Rohan", Role: RoleWarrior}, CampaignSettings{})
	d := CompanyDetail{CampaignCompany: CampaignCompany{Name: "Rohan", Faction: "Rohan"}, Roster: []CompanyUnit{lead, rider, foot, plain}}
	return d, rs.TTSList(d, TTSOptions{FootVersions: true, MountFigures: true})
}

func TestTTSNamesV2Rings(t *testing.T) {
	rs := shippedRules(t)
	d, ex := rohanRingCompany(t, rs)
	p := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort, Auras: rs.DefaultAuraSelection()})
	checkTTSPayload(t, "rings", ex, p)
	if p.Version != 2 {
		t.Fatalf("version %d", p.Version)
	}
	rings := map[string][]TTSRing{}
	for _, f := range p.Figures {
		key := f.Extra
		if f.Unit == 2 {
			key = "rider" + f.Extra
		}
		if f.Unit == 1 {
			key = "leader"
		}
		if len(f.Auras) > 0 {
			rings[key] = f.Auras
		}
		if f.Extra == "mount" && len(f.Auras) > 0 {
			t.Errorf("a mount has rings: %+v", f)
		}
	}
	want := []TTSRing{{3, "banner", "a", "Commanding Bellow"}, {12, "command", "a", "Voice of Command"}, {6, "enemy", "a", "Furious Tirade, Master of Battle"}}
	if !slices.Equal(rings["leader"], want) {
		t.Errorf("leader %+v, want %+v", rings["leader"], want)
	}
	// the rider carries the banner (its on-foot version too when the list has one); the mount none
	if len(rings["rider"]) != 1 || rings["rider"][0].K != "banner" || rings["rider"][0].R != 3 {
		t.Errorf("rider %+v foot %+v", rings["rider"], rings["riderfoot"])
	}
	body, _ := p.JSON()
	if !strings.Contains(string(body), `"a":[{"r":3,"k":"banner","w":"a","n":"Banner"}]`) || !strings.HasPrefix(string(body), `{"v":2,`) {
		t.Errorf("json: %.400s", body)
	}
	again, _ := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort, Auras: rs.DefaultAuraSelection()}).JSON()
	if !bytes.Equal(body, again) {
		t.Error("the JSON is not deterministic")
	}
	var back TTSNamesPayload
	if err := json.Unmarshal(body, &back); err != nil || back.Version != 2 || len(back.Figures) != len(p.Figures) {
		t.Errorf("reads back as %v %+v", err, back.Version)
	}
	// the zero selection: version 1
	if v1 := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort}); v1.Version != 1 {
		t.Errorf("zero selection: version %d", v1.Version)
	}
	// only the kinds that are on
	sel := rs.DefaultAuraSelection()
	sel.Kinds = map[AuraKind]bool{"banner": true}
	only := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort, Auras: sel})
	for _, f := range only.Figures {
		if !slices.Equal(ringKinds(f), nil) && !slices.Equal(ringKinds(f), []string{"banner"}) {
			t.Errorf("kinds %v with only banners on", ringKinds(f))
		}
	}
}

func TestTTSNamesV2FallenAndCreatures(t *testing.T) {
	rs := shippedRules(t)
	d, _ := rohanRingCompany(t, rs)
	d.Creatures = []CompanyCreature{{Name: "Falcon", OwnerID: 1, Rules: []string{"Keen Sight (Falcon)"}}}
	ex := rs.TTSLiveList(d, TTSLive{Units: []LiveUnit{{UnitID: 3, Zone: ZoneCasualty}}}, TTSOptions{})
	p := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort, Auras: rs.DefaultAuraSelection()})
	checkTTSPayload(t, "live rings", ex, p)
	var falcon bool
	for _, f := range p.Figures {
		if f.Unit == 3 && len(f.Auras) > 0 {
			t.Errorf("a fallen unit has rings: %+v", f)
		}
		if strings.HasPrefix(f.Line, "Crebain") && len(f.Auras) == 1 && f.Auras[0].R == 12 && f.Auras[0].K == "enemy" {
			falcon = true
		}
	}
	if !falcon {
		t.Errorf("the falcon figure has no Keen Sight ring: %+v", p.Figures)
	}
}

// TestTTSNamesV2Caps: at most 6 rings per figure and 150 in all, with a note; the rings that stay are the first in kind order.
func TestTTSNamesV2Caps(t *testing.T) {
	rs := shippedRules(t)
	co := auraCompany(t, rs, "Rohan")
	// a Sorcerer with everything on has more than six rings
	sorc := auraHeroOn(t, rs, "Rohan", "Warrior of Rohan", "Path of the Sorcerer", func(s *UnitState) {
		s.Spells = []PathSpell{{Name: "Aura of Command", Casting: 4}, {Name: "Fury", Casting: 4}, {Name: "Aura of Dismay", Casting: 4}, {Name: "Blinding Light", Casting: 4}, {Name: "Writhing Vines", Casting: 4}}
		s.Gear = []string{"War horn, shield, and throwing spears"}
	})
	sorc.Role = RoleLeader
	d := CompanyDetail{CampaignCompany: CampaignCompany{Name: "Rohan", Faction: "Rohan"}, Roster: []CompanyUnit{sorc}}
	ex := rs.TTSList(d, TTSOptions{})
	all := AuraSelection{Draw: true, Declared: true, Conditional: true, Cast: true, Target: true, Large: true, Optional: true}
	p := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort, Auras: all})
	if len(p.Figures[0].Auras) != AurasPerFigure {
		t.Fatalf("%d rings on one figure: %+v", len(p.Figures[0].Auras), p.Figures[0].Auras)
	}
	if k := ringKinds(p.Figures[0]); !slices.IsSortedFunc(k, func(a, b string) int { return rs.kindRank(AuraKind(a)) - rs.kindRank(AuraKind(b)) }) {
		t.Errorf("kinds not in order: %v", k)
	}
	if n := noteOf(p.Notes, TTSAurasLeftOut); n == "" || !strings.Contains(n, "left out") {
		t.Errorf("notes %+v", p.Notes)
	}
	// 200 banners: 150 rings in all
	var roster []CompanyUnit
	for id := int64(1); id <= 200; id++ {
		roster = append(roster, buildUnit(rs, co, UnitState{ID: id, Base: "Warrior of Rohan", Role: RoleWarrior, Gear: []string{"Banner"}}, CampaignSettings{}))
	}
	roster[0].Role = RoleLeader
	d = CompanyDetail{CampaignCompany: CampaignCompany{Name: "Rohan", Faction: "Rohan"}, Roster: roster}
	ex = rs.TTSList(d, TTSOptions{})
	figs, st := rs.figureRings(d, ex, rs.DefaultAuraSelection())
	rings := 0
	for _, f := range figs {
		rings += len(f.Rings)
	}
	if rings != AurasPerPayload || st.leftOut != 201-AurasPerPayload || noteOf(st.notes, TTSAurasLeftOut) == "" {
		t.Errorf("%d rings, %d left out, notes %+v", rings, st.leftOut, st.notes)
	}
	// the dialog counts before the limits: 201 banner and Stand Fast rings are counted, 150 are in the payload
	sum := rs.AuraSummaryFor(d, ex, rs.DefaultAuraSelection())
	counted := 0
	for _, k := range sum.Kinds {
		counted += k.Rings
	}
	if sum.Rings != AurasPerPayload || counted != 201 || sum.LeftOut != 201-AurasPerPayload {
		t.Errorf("summary rings %d, counted %d, left out %d", sum.Rings, counted, sum.LeftOut)
	}
	// that many figures do not fit the size limit: the rings go with a note, the names stay
	p = rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesNone, Auras: rs.DefaultAuraSelection()})
	if p.Version != 1 || noteOf(p.Notes, TTSAurasLeftOut) == "" {
		t.Errorf("version %d notes %+v", p.Version, p.Notes)
	}
}

func noteOf(notes []TTSNote, kind string) string {
	for _, n := range notes {
		if n.Kind == kind {
			return n.Text
		}
	}
	return ""
}

// TestTTSNamesV2SizeLimit: when the rings are what makes the payload too big, they are the last thing given up, after the rule texts, with a note.
func TestTTSNamesV2SizeLimit(t *testing.T) {
	rs := shippedRules(t)
	co := auraCompany(t, rs, "Rohan")
	names := func(n int) (TTSNamesPayload, TTSNamesPayload) {
		var roster []CompanyUnit
		for id := int64(1); id <= int64(n); id++ {
			roster = append(roster, buildUnit(rs, co, UnitState{ID: id, Base: "Warrior of Rohan", Role: RoleWarrior, Gear: []string{"Banner"}}, CampaignSettings{}))
		}
		d := CompanyDetail{CampaignCompany: CampaignCompany{Name: "Rohan", Faction: "Rohan"}, Roster: roster}
		ex := rs.TTSList(d, TTSOptions{})
		return rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesNone}), rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesNone, Auras: rs.DefaultAuraSelection()})
	}
	without, with := names(30)
	if with.Version != 2 || with.Size <= without.Size || with.Size > TTSNamesMaxBytes || noteOf(with.Notes, TTSAurasLeftOut) != "" {
		t.Errorf("30 figures: v%d %d bytes (%d without), notes %+v", with.Version, with.Size, without.Size, with.Notes)
	}
	without, with = names(63) // the names alone fit, the rings do not
	if without.Size > TTSNamesMaxBytes {
		t.Skipf("the base payload is already %d bytes", without.Size)
	}
	if with.Size > TTSNamesMaxBytes {
		t.Errorf("63 figures: %d bytes with rings, over the limit, and no fallback ran", with.Size)
	}
	if with.Version == 2 {
		t.Logf("63 figures still fit with rings (%d bytes)", with.Size)
	} else if noteOf(with.Notes, TTSAurasLeftOut) == "" || with.Size != without.Size {
		t.Errorf("the rings were given up without a note, or the names changed: %d %d %+v", with.Size, without.Size, with.Notes)
	}
}

// TestTTSNamesWithoutAuraData: a ruleset without data.auras writes version 1 even when the selection asks for rings.
func TestTTSNamesWithoutAuraData(t *testing.T) {
	rs := shippedRules(t)
	withoutAuras(t, rs)
	u := auraHeroOn(t, rs, "Rohan", "Warrior of Rohan", "Path of the Warlord", nil)
	d := CompanyDetail{CampaignCompany: CampaignCompany{Faction: "Rohan"}, Roster: []CompanyUnit{u}}
	sel := AuraSelection{Draw: true, Declared: true, Conditional: true, Cast: true, Target: true, Large: true}
	if p := rs.TTSNames(d, rs.TTSList(d, TTSOptions{}), TTSNamesOptions{Auras: sel}); p.Version != TTSNamesVersion || len(p.Notes) != 0 {
		t.Errorf("payload version %d, notes %+v without data", p.Version, p.Notes)
	}
}
