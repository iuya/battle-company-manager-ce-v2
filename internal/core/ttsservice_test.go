package core

import "testing"

func TestTTSServiceListsTheCompanyAndTheLiveOne(t *testing.T) {
	f := newCompanyFixture(t)
	f.update()
	s := NewTTSService(f.campaigns, f.companies, staticRules{shippedRules(t)})
	d := f.detail(t)
	plain, err := s.TTSExport(f.ctx, 5, 1, nil, TTSOptions{FootVersions: true, MountFigures: true})
	if err != nil || plain.Text == "" || len(plain.Warbands) != 1 {
		t.Fatalf("the company is a list in one warband: %v %+v", err, plain)
	}
	live, err := s.TTSExport(f.ctx, 5, 1, &TTSLive{Absent: []int64{d.Roster[len(d.Roster)-1].ID}}, TTSOptions{})
	if err != nil || len(live.Warbands) != 2 || !live.Warbands[1].Fallen {
		t.Errorf("the absent unit goes in a second warband: %v %+v", err, live.Warbands)
	}
}

func TestTTSServiceNamesMatchTheExportItWasGiven(t *testing.T) {
	f := newCompanyFixture(t)
	f.update()
	s := NewTTSService(f.campaigns, f.companies, staticRules{shippedRules(t)})
	exp, err := s.TTSExport(f.ctx, 5, 1, nil, TTSOptions{FootVersions: true, MountFigures: true})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.TTSNames(f.ctx, 5, 1, exp, TTSNamesOptions{Rules: TTSRulesShort})
	if err != nil || p.Version != 1 || len(p.Figures) != exp.Models+exp.ExtraFigures {
		t.Fatalf("one entry per figure of the list: %v %d figures for %d+%d", err, len(p.Figures), exp.Models, exp.ExtraFigures)
	}
	if js, err := p.JSON(); err != nil || len(js) != p.Size {
		t.Errorf("the size is the JSON's: %v %d %d", err, len(js), p.Size)
	}
}
