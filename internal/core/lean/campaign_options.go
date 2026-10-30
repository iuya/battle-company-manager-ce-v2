package lean

// CampaignOption is a setting the file offers the organiser of a campaign: a rule variant (scope rules) or how the app behaves (scope app). The file is
// the single list of them; the app switches the ones it knows by key (core.KnownOption) and lists any other as unsupported, with a warning in the log.
type CampaignOption struct {
	Key         string      `json:"key" required:"true" min:"1"`
	Scope       string      `json:"scope" required:"true" enum:"rules|app"`
	Label       string      `json:"label" required:"true" min:"1"`
	Description string      `json:"description,omitempty"`
	Default     bool        `json:"default,omitempty"`
	Implemented *bool       `json:"implemented,omitempty"`
	Swaps       *OptionSwap `json:"swaps,omitempty"`
}

// OptionSwap says which path an option puts in the place of which: the rule variant of a house rule that swaps one rule for another.
type OptionSwap struct {
	Path     string `json:"path" required:"true" min:"1"`
	Replaces string `json:"replaces" required:"true" min:"1"`
}

// Oaths are the fallback oaths of the companies that have none of their own, by alignment.
type Oaths struct {
	Good *Oath `json:"good,omitempty"`
	Evil *Oath `json:"evil,omitempty"`
}

// ByAlignment is the oath for an alignment (good or evil), nil when the file has none.
func (o Oaths) ByAlignment(alignment string) *Oath {
	switch alignment {
	case "good":
		return o.Good
	case "evil":
		return o.Evil
	}
	return nil
}
