package core

import (
	"os"
	"slices"
	"testing"
)

func TestBCBookTTSModels(t *testing.T) {
	content, err := os.ReadFile("../../definitions/battle_companies/battle_companies.json")
	if err != nil {
		t.Fatal(err)
	}
	book, err := ParseBattleCompanies(content)
	if err != nil {
		t.Fatal(err)
	}
	m := book.TTSModels()
	if len(m.StandIns) != 7 {
		t.Fatalf("stand-ins = %d, want 7", len(m.StandIns))
	}
	if i := slices.IndexFunc(m.StandIns, func(s TTSStandIn) bool { return s.Name == "Wandering Marksman" }); i < 0 || m.StandIns[i].Model != "Ruffian: Bow" || m.StandIns[i].Kind != "wanderer" {
		t.Errorf("Wandering Marksman = %+v", m.StandIns)
	}
	if i := slices.IndexFunc(m.Mounts, func(x TTSMount) bool { return x.Gear == "Warg" }); i < 0 || m.Mounts[i].Model != "Fell Warg" || m.Mounts[i].Attacks != 1 {
		t.Errorf("Warg mount = %+v", m.Mounts)
	}
	states := map[string]bool{}
	for _, r := range m.Riders {
		states[r.Unit] = r.States
	}
	if len(states) != 18 || !states["Rider of Rohan"] || states["Serpent Rider"] {
		t.Errorf("riders = %v", states)
	}
	if i := slices.IndexFunc(m.Riders, func(r TTSRider) bool { return r.Unit == "Serpent Rider" }); m.Riders[i].FootModel != "Serpent Guard" {
		t.Errorf("Serpent Rider = %+v", m.Riders[i])
	}
	if len(m.Names) == 0 || m.Names[0].Unit != "Isengard Orc Warrior" || m.Names[0].To != "Bow" {
		t.Errorf("names = %+v", m.Names)
	}
}

func TestParseTTSModelsWithoutTheSection(t *testing.T) {
	got, err := ParseTTSModels([]byte(`{}`))
	if err != nil || len(got.StandIns)+len(got.Mounts)+len(got.Riders)+len(got.Names) != 0 {
		t.Errorf("got %+v, %v", got, err)
	}
	if _, err := ParseTTSModels([]byte(`{`)); err == nil {
		t.Error("invalid JSON must fail")
	}
}
