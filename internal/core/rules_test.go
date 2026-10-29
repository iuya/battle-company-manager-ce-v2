package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

func capture() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})), &buf
}

// vanillaWithDuplicates repeats "Song" (differently), "Twin" (identically) and units/gear with the same name.
const vanillaWithDuplicates = `{"stats":[{"name":"Fight","abbreviation":"F"},{"name":"Courage","abbreviation":"C"},{"name":"","abbreviation":"|"},{"name":"Might","abbreviation":"M"},{"name":"Will","abbreviation":"W"},{"name":"Fate","abbreviation":"F"}],"data":{
 "heroes":[{"name":"Knight","points":10,"fight":5},{"name":"Knight","points":99,"fight":1}],
 "warriors":[{"name":"Knight","points":50},{"name":"Guard","points":5},{"name":"Guard","points":5}],
 "gear":[{"name":"Axe","type":"axe","definition":"first"},{"name":"axe","type":"axe","definition":"second"}],
 "keywords":[{"name":"Song","definition":"v1"},{"name":"Song","definition":"v2"},{"name":"Song","definition":"v3"},
             {"name":"Twin","definition":"same"},{"name":"Twin","definition":"same"},
             {"name":"Listed","definition":"a"},{"name":"Listed","definition":"b"}],
 "magicalPowers":[{"name":"Blast","duration":"i","definition":"x"},{"name":"Blast","duration":"i","definition":"y"}]}}`

func TestVanillaDuplicatesFirstOccurrenceWins(t *testing.T) {
	log, out := capture()
	b, err := ParseVanillaWith([]byte(vanillaWithDuplicates), nil, log)
	if err != nil {
		t.Fatal(err)
	}
	if u, _ := b.Unit("Knight"); u.Points != 10 {
		t.Errorf("Knight = %+v, want the first (hero) occurrence", u)
	}
	if g, _ := b.Gear("AXE"); g.Definition != "first" {
		t.Errorf("gear = %+v", g)
	}
	if d, _ := b.Keyword("Song"); d != "v1" {
		t.Errorf("Song = %q", d)
	}
	if p, _ := b.Power("Blast"); p.Definition != "x" {
		t.Errorf("Blast = %+v", p)
	}
	log2 := out.String()
	for _, want := range []string{"name=Knight", "name=Axe", "name=Song", "name=Listed", "name=Blast"} {
		if !strings.Contains(log2, want) {
			t.Errorf("no warning for %s in:\n%s", want, log2)
		}
	}
	for _, not := range []string{"name=Twin", "name=Guard"} {
		if strings.Contains(log2, not) {
			t.Errorf("identical duplicates must not warn (%s):\n%s", not, log2)
		}
	}
}

func TestVanillaDuplicatesReviewedAndPinned(t *testing.T) {
	log, out := capture()
	reviewed := []VanillaAmbiguity{
		{Kind: "keyword", Name: "Song", Status: "pinned", Occurrence: 3},
		{Kind: "keyword", Name: "Listed", Status: "firstWins", Occurrence: 1},
		{Kind: "unit", Name: "Knight", Status: "firstWins"},
		{Kind: "gear", Name: "AXE", Status: "equivalent"},
		{Kind: "power", Name: "Blast", Status: "overridden"},
	}
	b, err := ParseVanillaWith([]byte(vanillaWithDuplicates), reviewed, log)
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := b.Keyword("Song"); d != "v3" {
		t.Errorf("pinned Song = %q, want v3", d)
	}
	if d, _ := b.Keyword("Listed"); d != "a" {
		t.Errorf("Listed = %q", d)
	}
	if out.Len() != 0 {
		t.Errorf("reviewed duplicates must not warn:\n%s", out)
	}
}

func TestVanillaPinToMissingOccurrenceWarnsAndKeepsFirst(t *testing.T) {
	log, out := capture()
	b, _ := ParseVanillaWith([]byte(vanillaWithDuplicates), []VanillaAmbiguity{{Kind: "keyword", Name: "Song", Status: "pinned", Occurrence: 9}}, log)
	if d, _ := b.Keyword("Song"); d != "v1" {
		t.Errorf("Song = %q", d)
	}
	if !strings.Contains(out.String(), "does not exist") {
		t.Errorf("log: %s", out)
	}
}

func readDef(t testing.TB, p string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../definitions/" + p)
	if err != nil {
		t.Skip("definitions not available:", err)
	}
	return b
}

const (
	hV = "vanilla-hash"
	hB = "bc-hash"
	hG = "glossary-hash"
)

var triple = RulesetHashes{Vanilla: hV, BattleCompanies: hB, Glossary: hG}

func expectFiles(repo *MockrulesetRepository, vanilla, bc, glossary []byte) {
	repo.EXPECT().Get(gomock.Any(), hV).Return(StoredRuleset{RulesetInfo: RulesetInfo{Kind: KindVanilla, Hash: hV}, Content: vanilla}, nil)
	repo.EXPECT().Get(gomock.Any(), hB).Return(StoredRuleset{RulesetInfo: RulesetInfo{Kind: KindBattleCompanies, Hash: hB}, Content: bc}, nil)
	repo.EXPECT().Get(gomock.Any(), hG).Return(StoredRuleset{RulesetInfo: RulesetInfo{Kind: KindGlossary, Hash: hG}, Content: glossary}, nil)
}

func TestRulesForBuildsOncePerTripleUnderConcurrency(t *testing.T) {
	repo := NewMockrulesetRepository(gomock.NewController(t))
	release := make(chan struct{})
	for kind, hash := range map[RulesetKind]string{KindVanilla: hV, KindBattleCompanies: hB, KindGlossary: hG} {
		content := map[RulesetKind]string{KindVanilla: vanillaWithDuplicates, KindBattleCompanies: bcFile, KindGlossary: `{"name":"G","version":"1","terms":[]}`}[kind]
		repo.EXPECT().Get(gomock.Any(), hash).Times(1).DoAndReturn(func(context.Context, string) (StoredRuleset, error) {
			<-release
			return StoredRuleset{RulesetInfo: RulesetInfo{Kind: kind, Hash: hash}, Content: []byte(content)}, nil
		})
	}
	log, _ := capture()
	rules := NewRules(repo, log)
	const n = 32
	got := make([]*RuleSet, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rs, err := rules.For(context.Background(), triple)
			if err != nil {
				t.Error(err)
			}
			got[i] = rs
		}()
	}
	close(release)
	wg.Wait()
	for i := range got {
		if got[i] == nil || got[i] != got[0] {
			t.Fatalf("call %d returned %p, want %p", i, got[i], got[0])
		}
	}
	again, _ := rules.For(context.Background(), triple)
	if again != got[0] || again.Hashes != triple {
		t.Error("a later call must hit the cache")
	}
}

func TestRulesForSeparatesTriples(t *testing.T) {
	repo := NewMockrulesetRepository(gomock.NewController(t))
	bc := []byte(bcFile)
	gl := []byte(`{"name":"G","version":"1","terms":[]}`)
	expectFiles(repo, []byte(vanillaWithDuplicates), bc, gl)
	other := RulesetHashes{Vanilla: "v2", BattleCompanies: hB, Glossary: hG}
	repo.EXPECT().Get(gomock.Any(), "v2").Return(StoredRuleset{RulesetInfo: RulesetInfo{Kind: KindVanilla, Hash: "v2"}, Content: []byte(vanillaStatsOnly)}, nil)
	repo.EXPECT().Get(gomock.Any(), hB).Return(StoredRuleset{RulesetInfo: RulesetInfo{Kind: KindBattleCompanies, Hash: hB}, Content: bc}, nil)
	repo.EXPECT().Get(gomock.Any(), hG).Return(StoredRuleset{RulesetInfo: RulesetInfo{Kind: KindGlossary, Hash: hG}, Content: gl}, nil)
	rules := NewRules(repo, nil)
	a, err1 := rules.For(context.Background(), triple)
	b, err2 := rules.For(context.Background(), other)
	if err1 != nil || err2 != nil || a == b || a.Vanilla == b.Vanilla {
		t.Errorf("%v %v distinct=%v", err1, err2, a != b)
	}
}

func TestRulesForHonoursPinsFromTheBattleCompaniesFileAndWarns(t *testing.T) {
	repo := NewMockrulesetRepository(gomock.NewController(t))
	bc := strings.Replace(bcFile, `"schemaVersion":2,`, `"schemaVersion":2,"vanilla":{"pins":[{"kind":"rule","name":"Song","occurrence":2}]},`, 1)
	expectFiles(repo, []byte(vanillaWithDuplicates), []byte(bc), []byte(`{"name":"G","version":"1","terms":[]}`))
	log, out := capture()
	rs, err := NewRules(repo, log).For(context.Background(), triple)
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := rs.Vanilla.Keyword("Song"); d != "v2" {
		t.Errorf("Song = %q, want the pinned second occurrence", d)
	}
	if out.Len() != 0 { // a lean file reviews duplicates by pinning only: the others keep the first occurrence quietly
		t.Errorf("log:\n%s", out)
	}
}

func TestRulesForErrorsAreNotCached(t *testing.T) {
	repo := NewMockrulesetRepository(gomock.NewController(t))
	boom := errors.New("db down")
	repo.EXPECT().Get(gomock.Any(), hV).Return(StoredRuleset{}, boom)
	rules := NewRules(repo, nil)
	if _, err := rules.For(context.Background(), triple); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	expectFiles(repo, []byte(vanillaStatsOnly), []byte(bcFile), []byte(`{"name":"G","version":"1","terms":[]}`))
	if _, err := rules.For(context.Background(), triple); err != nil {
		t.Errorf("retry after a failure: %v", err)
	}
}

func TestRulesForRejectsAFileOfAnotherKind(t *testing.T) {
	repo := NewMockrulesetRepository(gomock.NewController(t))
	repo.EXPECT().Get(gomock.Any(), hV).Return(StoredRuleset{RulesetInfo: RulesetInfo{Kind: KindGlossary, Hash: hV}}, nil)
	if _, err := NewRules(repo, nil).For(context.Background(), triple); !errors.Is(err, ErrInvalidRuleset) {
		t.Errorf("err = %v", err)
	}
}

func TestRulesForStopsWaitingWhenTheContextEnds(t *testing.T) {
	repo := NewMockrulesetRepository(gomock.NewController(t))
	release := make(chan struct{})
	repo.EXPECT().Get(gomock.Any(), hV).DoAndReturn(func(context.Context, string) (StoredRuleset, error) {
		<-release
		return StoredRuleset{}, errors.New("late")
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rules := NewRules(repo, nil)
	if _, err := rules.For(ctx, triple); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
	close(release)
	// let the abandoned build finish so the mock sees no stray call after the test
	for {
		rules.mu.Lock()
		n := len(rules.calls)
		rules.mu.Unlock()
		if n == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRuleSetKeywordOrderEntityThenBCThenVanilla(t *testing.T) {
	vb, _ := ParseVanilla([]byte(`{"data":{"keywords":[{"name":"Both","definition":"vanilla"},{"name":"OnlyVanilla","definition":"vanilla only"}]}}`))
	rs := &RuleSet{Vanilla: vb, BC: &BCBook{keywords: map[string]string{"Both": "bc"}}}
	for _, tt := range []struct{ embedded, name, want string }{
		{"entity", "Both", "entity"},
		{"", "Both", "bc"},
		{"", "OnlyVanilla", "vanilla only"},
	} {
		if got, ok := rs.Keyword(tt.embedded, tt.name); !ok || got != tt.want {
			t.Errorf("Keyword(%q,%q) = %q, %v; want %q", tt.embedded, tt.name, got, ok, tt.want)
		}
	}
	if _, ok := rs.Keyword("", "Nope"); ok {
		t.Error("unknown keyword")
	}
}

// With the files the app ships: it loads, carries the three books, and the confirmed first-wins duplicates resolve to
// their first occurrence without any copied override.
func TestRulesForShippedFiles(t *testing.T) {
	vanilla := readDef(t, "vanilla/data2024.json")
	bc := readDef(t, "battle_companies/battle_companies.json")
	gl := readDef(t, "glossary/mesbg_glossary.json")
	repo := NewMockrulesetRepository(gomock.NewController(t))
	expectFiles(repo, vanilla, bc, gl)
	log, out := capture()
	rs, err := NewRules(repo, log).For(context.Background(), triple)
	if err != nil {
		t.Fatal(err)
	}
	if rs.Vanilla == nil || rs.BC == nil || len(rs.Glossary.Terms) == 0 {
		t.Fatalf("incomplete rule set: %+v", rs)
	}
	var doc struct {
		Data struct {
			Keywords []struct{ Name, Definition string } `json:"keywords"`
		} `json:"data"`
	}
	mustUnmarshal(t, vanilla, &doc)
	first := map[string]string{}
	for _, k := range doc.Data.Keywords {
		if _, ok := first[k.Name]; !ok {
			first[k.Name] = k.Definition
		}
	}
	for _, name := range []string{"Enchanting Song", "Mercenary Ambush", "Relentless Advance", "The Honour of Lamedon"} {
		if got, _ := rs.Vanilla.Keyword(name); got != first[name] {
			t.Errorf("%s does not resolve to its first occurrence", name)
		}
	}
	if out.Len() != 0 {
		t.Logf("unreviewed duplicates in the shipped files (ask the curator to list them):\n%s", out)
	}
}

func mustUnmarshal(t *testing.T, b []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func TestHeroicActionNamesUseTheGlossarySpelling(t *testing.T) {
	g, err := BuildGlossary(RulesetInfo{}, []byte(`{"name":"G","version":"1","terms":[
{"name":"Heroic Channelling","category":"heroic actions","aliases":["Heroic Channeling"],"definition":"x","sources":[{"section":"s","rulebookPage":1}]},
{"name":"Heroic Defence","category":"heroic actions","aliases":["Heroic Defense"],"definition":"x"},
{"name":"Heroic March","category":"heroic actions","definition":"x"}]}`))
	if err != nil || len(g.Terms) != 3 {
		t.Fatalf("glossary: %v %d", err, len(g.Terms))
	}
	v := &VanillaBook{units: map[string]VanillaUnit{
		"A": {HeroicActions: []string{"Heroic Channeling", "Heroic Defence"}},
		"B": {HeroicActions: []string{"Heroic Channelling", "Heroic Defense", "Heroic March"}},
	}}
	rs := &RuleSet{Vanilla: v, Glossary: g}
	got := rs.HeroicActionNames()
	want := []string{"Heroic Channelling", "Heroic Defence", "Heroic March"}
	if !slices.Equal(got, want) {
		t.Errorf("names: %v, want %v", got, want)
	}
	if rs.HeroicActionName("Heroic Defense") != "Heroic Defence" || rs.HeroicActionName("Unknown") != "Unknown" {
		t.Error("canonical name")
	}
	l := LevelLists{ActionName: rs.HeroicActionName}
	if l.canonAction("heroic channeling") != "Heroic Channelling" {
		t.Error("lists canonicalise owned actions")
	}
}

func TestRulesCacheKeepsOnlyTheMostRecentlyUsedSets(t *testing.T) {
	repo := NewMockrulesetRepository(gomock.NewController(t))
	builds := map[string]int{} // vanilla hash -> how many times it was read
	repo.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, hash string) (StoredRuleset, error) {
		switch hash {
		case hB:
			return StoredRuleset{RulesetInfo: RulesetInfo{Kind: KindBattleCompanies, Hash: hB}, Content: []byte(bcFile)}, nil
		case hG:
			return StoredRuleset{RulesetInfo: RulesetInfo{Kind: KindGlossary, Hash: hG}, Content: []byte(`{"name":"G","version":"1","terms":[]}`)}, nil
		}
		builds[hash]++
		return StoredRuleset{RulesetInfo: RulesetInfo{Kind: KindVanilla, Hash: hash}, Content: []byte(vanillaStatsOnly)}, nil
	}).AnyTimes()
	rules := NewRules(repo, nil)
	rules.max = 2
	tri := func(v string) RulesetHashes { return RulesetHashes{Vanilla: v, BattleCompanies: hB, Glossary: hG} }
	get := func(v string) *RuleSet {
		t.Helper()
		rs, err := rules.For(context.Background(), tri(v))
		if err != nil {
			t.Fatal(err)
		}
		return rs
	}
	a := get("a")
	get("b")
	if get("a") != a {
		t.Fatal("a is cached")
	}
	get("c") // b is the least recently used: it goes, a stays
	if rules.Cached() != 2 {
		t.Fatalf("cached = %d, want the cap 2", rules.Cached())
	}
	if get("a") != a || builds["a"] != 1 {
		t.Errorf("the recently used set was rebuilt (builds %v)", builds)
	}
	get("b")
	if builds["b"] != 2 {
		t.Errorf("the evicted set b should be rebuilt once more, builds %v", builds)
	}
	for _, v := range []string{"d", "e", "f", "g"} {
		get(v)
	}
	if rules.Cached() != 2 {
		t.Errorf("cached = %d after many triples, want 2", rules.Cached())
	}
}

func TestRulesCacheNeverDropsABuildInProgress(t *testing.T) {
	repo := NewMockrulesetRepository(gomock.NewController(t))
	release := make(chan struct{})
	repo.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, hash string) (StoredRuleset, error) {
		<-release
		switch hash {
		case hB:
			return StoredRuleset{RulesetInfo: RulesetInfo{Kind: KindBattleCompanies, Hash: hB}, Content: []byte(bcFile)}, nil
		case hG:
			return StoredRuleset{RulesetInfo: RulesetInfo{Kind: KindGlossary, Hash: hG}, Content: []byte(`{"name":"G","version":"1","terms":[]}`)}, nil
		}
		return StoredRuleset{RulesetInfo: RulesetInfo{Kind: KindVanilla, Hash: hash}, Content: []byte(vanillaStatsOnly)}, nil
	}).AnyTimes()
	rules := NewRules(repo, nil)
	rules.max = 1
	var wg sync.WaitGroup
	for _, v := range []string{"a", "b", "c"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := rules.For(context.Background(), RulesetHashes{Vanilla: v, BattleCompanies: hB, Glossary: hG}); err != nil {
				t.Error(err)
			}
		}()
	}
	for rules.Cached() < 3 { // all three builds are in flight and none may be evicted
		runtime.Gosched()
	}
	close(release)
	wg.Wait()
}

func TestRulesWithMaxCached(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want int
	}{
		{"raised", 40, 40},
		{"lowered", 2, 2},
		{"one", 1, 1},
		{"zero keeps the default", 0, MaxCachedRuleSets},
		{"negative keeps the default", -3, MaxCachedRuleSets},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewRules(nil, nil).WithMaxCached(tt.n).max; got != tt.want {
				t.Errorf("max = %d, want %d", got, tt.want)
			}
		})
	}
}

// vanillaStatsOnly is a vanilla file with the characteristics and nothing else: a Battle Companies file's stat names are read against them.
const vanillaStatsOnly = `{"stats":[{"name":"Fight","abbreviation":"F"},{"name":"Courage","abbreviation":"C"},{"name":"","abbreviation":"|"},{"name":"Might","abbreviation":"M"},{"name":"Will","abbreviation":"W"},{"name":"Fate","abbreviation":"F"}],"data":{}}`
