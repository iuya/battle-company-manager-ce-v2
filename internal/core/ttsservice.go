package core

import "context"

// TTSService gives the web layer the Tabletop Simulator army list of a company (docs/tabletop-simulator.md). The list itself is
// RuleSet.TTSList; this loads the company and the campaign's rules. Who may ask is decided by the caller, as for the company page.
type TTSService struct{ companyDeps }

// NewTTSService builds the service. rules is the loader (*Rules).
func NewTTSService(campaigns campaignRepository, companies companyRepository, rules ruleSource) *TTSService {
	return &TTSService{companyDeps{campaigns: campaigns, companies: companies, rules: rules}}
}

// TTSExport is the company as an army list. live is the company's live state in an open match (nil: the plain list): the units
// that are wounded or out of the fight then go in a second warband. ErrNotFound when the campaign has no such company.
func (s *TTSService) TTSExport(ctx context.Context, campaignID, companyID int64, live *TTSLive, opts TTSOptions) (TTSExport, error) {
	d, rs, err := s.detail(ctx, campaignID, companyID)
	if err != nil {
		return TTSExport{}, err
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
