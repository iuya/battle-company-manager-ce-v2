package core

import "context"

// TTSService gives the web layer the Tabletop Simulator army list of a company (docs/tabletop-simulator.md). The list itself is
// RuleSet.TTSList; this loads the company and the campaign's rules. Who may ask is decided by the caller, as for the company page.
type TTSService struct {
	companyDeps
	catalogue TTSCatalogue // the mod's model keys when known (nil in production unless TTS_CATALOGUE is set)
}

// WithCatalogue lets the lists fall back to the closest figure the bag has for a key it lacks.
func (s *TTSService) WithCatalogue(c TTSCatalogue) *TTSService {
	s.catalogue = c
	return s
}

// NewTTSService builds the service. rules is the loader (*Rules).
func NewTTSService(campaigns campaignRepository, companies companyRepository, rules ruleSource) *TTSService {
	return &TTSService{companyDeps: companyDeps{campaigns: campaigns, companies: companies, rules: rules}}
}

// TTSExport is the company as an army list. live is the company's live state in an open match (nil: the plain list): the units
// that are wounded or out of the fight then go in a second warband. ErrNotFound when the campaign has no such company.
func (s *TTSService) TTSExport(ctx context.Context, campaignID, companyID int64, live *TTSLive, opts TTSOptions) (TTSExport, error) {
	d, rs, err := s.detail(ctx, campaignID, companyID)
	if err != nil {
		return TTSExport{}, err
	}
	if s.catalogue != nil && opts.Catalogue == nil {
		opts.Catalogue = s.catalogue
	}
	if live != nil {
		return rs.TTSLiveList(d, *live, opts), nil
	}
	return rs.TTSList(d, opts), nil
}

// TTSNames is the ArmyNames JSON v1 of the company for the export the caller already built (so the list and the names agree, whatever
// the switches and the live state were). ErrNotFound when the campaign has no such company.
func (s *TTSService) TTSNames(ctx context.Context, campaignID, companyID int64, exp TTSExport, opts TTSNamesOptions) (TTSNamesPayload, error) {
	d, rs, err := s.detail(ctx, campaignID, companyID)
	if err != nil {
		return TTSNamesPayload{}, err
	}
	return rs.TTSNames(d, exp, opts), nil
}

// TTSAuras is what the Auras section of the dialog shows for the selection: the kinds the company has, their rings and radii, how many rings each
// switch adds, and the notes for rings left out. It counts the figures of the same export as TTSExport (opts and live as for that call), so it agrees
// with the names payload. ErrNotFound when the campaign has no such company.
func (s *TTSService) TTSAuras(ctx context.Context, campaignID, companyID int64, live *TTSLive, opts TTSOptions, sel AuraSelection) (AuraSummary, error) {
	d, rs, err := s.detail(ctx, campaignID, companyID)
	if err != nil {
		return AuraSummary{}, err
	}
	if s.catalogue != nil && opts.Catalogue == nil {
		opts.Catalogue = s.catalogue
	}
	var exp TTSExport
	if live != nil {
		exp = rs.TTSLiveList(d, *live, opts)
	} else {
		exp = rs.TTSList(d, opts)
	}
	return rs.AuraSummaryFor(d, exp, sel), nil
}
