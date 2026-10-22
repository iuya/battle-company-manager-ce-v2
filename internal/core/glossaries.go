package core

import (
	"context"
	"sync"
)

// Glossaries gives the glossary to display, already linked. It implements web.glossaries: the campaign's glossary comes from
// its rule set (the glossary file plus the terms of its vanilla and Battle Companies files); the default one and "another
// glossary" are a glossary file on its own. Files are content addressed, so a built glossary is kept per hash.
type Glossaries struct {
	campaigns campaignRepository
	rules     ruleSource
	rulesets  rulesetRepository

	mu    sync.Mutex
	built map[string]Glossary // glossary file hash -> the file alone
}

// NewGlossaries builds the service. rules is the loader (*Rules).
func NewGlossaries(campaigns campaignRepository, rules ruleSource, rulesets rulesetRepository) *Glossaries {
	return &Glossaries{campaigns: campaigns, rules: rules, rulesets: rulesets, built: map[string]Glossary{}}
}

// Default is the glossary the app ships with.
func (g *Glossaries) Default(ctx context.Context) (Glossary, error) {
	info, err := g.rulesets.Default(ctx, KindGlossary)
	if err != nil {
		return Glossary{}, err
	}
	return g.ByHash(ctx, info.Hash)
}

// ByHash is the glossary file with this hash alone, without any campaign's extras; ErrNotFound for an unknown hash or a file
// that is not a glossary.
func (g *Glossaries) ByHash(ctx context.Context, hash string) (Glossary, error) {
	g.mu.Lock()
	built, ok := g.built[hash]
	g.mu.Unlock()
	if ok {
		return built, nil
	}
	file, err := g.rulesets.Get(ctx, hash)
	if err != nil {
		return Glossary{}, err
	}
	if file.Kind != KindGlossary {
		return Glossary{}, ErrNotFound
	}
	if built, err = BuildGlossary(file.RulesetInfo, file.Content); err != nil {
		return Glossary{}, err
	}
	g.mu.Lock()
	g.built[hash] = built
	g.mu.Unlock()
	return built, nil
}

// ForCampaign is the glossary the campaign is played with: its glossary file plus the terms of its rules files. ErrNotFound for
// an unknown campaign.
func (g *Glossaries) ForCampaign(ctx context.Context, campaignID int64) (Glossary, error) {
	h, _, err := g.campaigns.Hashes(ctx, campaignID)
	if err != nil {
		return Glossary{}, err
	}
	rs, err := g.rules.For(ctx, h)
	if err != nil {
		return Glossary{}, err
	}
	return rs.Glossary, nil
}
