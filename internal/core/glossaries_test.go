package core

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestGlossariesService(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	campaigns, rulesets := NewMockcampaignRepository(ctrl), NewMockrulesetRepository(ctrl)
	rs := shippedRules(t)
	g := NewGlossaries(campaigns, staticRules{rs}, rulesets)

	campaigns.EXPECT().Hashes(ctx, int64(5)).Return(rs.Hashes, CampaignSettings{}, nil)
	campaigns.EXPECT().Hashes(ctx, int64(404)).Return(RulesetHashes{}, CampaignSettings{}, ErrNotFound)
	got, err := g.ForCampaign(ctx, 5)
	if err != nil || len(got.Terms) != len(rs.Glossary.Terms) {
		t.Fatalf("ForCampaign = %d terms, %v", len(got.Terms), err)
	}
	if _, err := g.ForCampaign(ctx, 404); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown campaign: %v", err)
	}

	info := rs.Glossary.Info
	stored := StoredRuleset{RulesetInfo: info, Content: readDef(t, "glossary/mesbg_glossary.json")}
	rulesets.EXPECT().Default(ctx, KindGlossary).Return(info, nil)
	rulesets.EXPECT().Get(ctx, info.Hash).Return(stored, nil).Times(1) // built once per hash
	for range 2 {
		if d, err := g.Default(ctx); err != nil || len(d.Terms) == 0 {
			t.Fatalf("Default = %d terms, %v", len(d.Terms), err)
		}
		if _, err := g.ByHash(ctx, info.Hash); err != nil {
			t.Fatal(err)
		}
		rulesets.EXPECT().Default(ctx, KindGlossary).Return(info, nil).MaxTimes(1)
	}
	rulesets.EXPECT().Get(ctx, "bc").Return(StoredRuleset{RulesetInfo: RulesetInfo{Kind: KindBattleCompanies}}, nil)
	rulesets.EXPECT().Get(ctx, "none").Return(StoredRuleset{}, ErrNotFound)
	for _, hash := range []string{"bc", "none"} {
		if _, err := g.ByHash(ctx, hash); !errors.Is(err, ErrNotFound) {
			t.Errorf("ByHash(%s) = %v, want ErrNotFound", hash, err)
		}
	}
}
