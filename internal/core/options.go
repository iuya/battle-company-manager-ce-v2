package core

import (
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
)

// CampaignOption is a setting a Battle Companies file offers the organiser of a campaign (campaign.options[] of the file):
// a rule variant ("Heroes may discard any wargear") or how the app behaves ("Shake the table"). The file is the single list of
// them; the app only offers the ones it knows (see KnownOption) and implements.
type CampaignOption struct {
	Key         string
	Scope       string // "rules" or "app"
	Label       string
	Description string
	Default     bool
	Unsupported bool // the app does not know the option, or the file says it is not implemented: it is listed but cannot be switched on
}

// settingFields maps the keys of the options the app knows to the setting each one switches.
var settingFields = map[string]func(*CampaignSettings) *bool{
	"heroesDropProfileGear": func(s *CampaignSettings) *bool { return &s.HeroesDropProfileGear },
	"shakeTheTable":         func(s *CampaignSettings) *bool { return &s.ShakeTheTable },
	"manualRolls":           func(s *CampaignSettings) *bool { return &s.ManualRolls },
	"customOnTheirOwnPath":  func(s *CampaignSettings) *bool { return &s.CustomOnTheirOwnPath },
}

// KnownOption says whether the app has a setting for the option key.
func KnownOption(key string) bool { _, ok := settingFields[key]; return ok }

// Set switches an option on or off, and says whether the app knows it.
func (s *CampaignSettings) Set(key string, on bool) bool {
	f, ok := settingFields[key]
	if ok {
		*f(s) = on
	}
	return ok
}

// Enabled says whether an option is on.
func (s CampaignSettings) Enabled(key string) bool {
	f, ok := settingFields[key]
	return ok && *f(&s)
}

// ParseCampaignOptions reads the options of a Battle Companies file. An option the app does not know (a newer file, or one
// somebody made up) or has not implemented is still listed, marked Unsupported so the form can show it as such, with a warning in the log.
func ParseCampaignOptions(content []byte) ([]CampaignOption, error) {
	var doc struct {
		Campaign struct {
			Options []struct {
				Key         string `json:"key"`
				Scope       string `json:"scope"`
				Label       string `json:"label"`
				Description string `json:"description"`
				Default     bool   `json:"default"`
				Implemented *bool  `json:"implemented"`
			} `json:"options"`
		} `json:"campaign"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return nil, err
	}
	var out []CampaignOption
	for _, o := range doc.Campaign.Options {
		unsupported := !KnownOption(o.Key) || (o.Implemented != nil && !*o.Implemented)
		if unsupported {
			slog.Warn("campaign option the app does not support: it cannot be switched on", "option", o.Key)
		}
		out = append(out, CampaignOption{Key: o.Key, Scope: o.Scope, Label: o.Label, Description: o.Description, Default: o.Default, Unsupported: unsupported})
	}
	return out, nil
}

// ComposeObjectiveText is the sentence a scoring objective shows: the instruction (what a player notes down at the start of a
// secret objective), then the sentence of each option in the order the file prints them, joined by a space. The sentences are stored
// once, as options (a "for each" objective has one); there is no prose copy to keep in step.
func ComposeObjectiveText(instruction string, sentences []string) string {
	parts := slices.Clone(sentences)
	if instruction != "" {
		parts = append([]string{instruction}, parts...)
	}
	return strings.Join(parts, " ")
}
