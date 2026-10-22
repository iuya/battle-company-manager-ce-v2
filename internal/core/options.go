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

// ParseScenarios reads the scenarios of a Battle Companies file in the order of its list: name, place on the scenario
// selection tables, the text of its Victory Point conditions (one entry per group of conditions) and its objectives: the
// rewards of data.scenarioDetails[] whose condition the players tick (companyFlag, and modelFlag shown as one tick per
// company), each with the reward of its campaign.aftermathEffects[] entry. Rewards that follow from the result (victory,
// leaderNotRemoved) are not ticks and are not listed.
func ParseScenarios(content []byte) ([]ScenarioInfo, error) {
	var doc struct {
		Scenarios       []string `json:"scenarios"`
		ScenarioScoring []struct {
			Name   string `json:"name"`
			Points [][]struct {
				Text string `json:"text"`
			} `json:"points"`
		} `json:"scenarioScoring"`
		Campaign struct {
			Selection struct {
				Tables []struct {
					Scenarios []string `json:"scenarios"`
				} `json:"tables"`
			} `json:"scenarioSelection"`
			AftermathEffects []struct {
				Key    string `json:"key"`
				Kind   string `json:"kind"`
				Amount int    `json:"amount"`
				Text   string `json:"text"`
			} `json:"aftermathEffects"`
		} `json:"campaign"`
		Data struct {
			ScenarioDetails []struct {
				Name              string `json:"name"`
				Description       string `json:"description"`
				Outline           string `json:"outline"`
				Armies            string `json:"armies"`
				Layout            string `json:"layout"`
				StartingPositions string `json:"startingPositions"`
				Objectives        string `json:"objectives"`
				SpecialRules      []struct {
					Name       string `json:"name"`
					Definition string `json:"definition"`
					Table      *struct {
						Lookup  string   `json:"lookup"`
						Columns []string `json:"columns"`
						Rows    []struct {
							Min   int      `json:"min"`
							Max   *int     `json:"max"`
							Cells []string `json:"cells"`
						} `json:"rows"`
					} `json:"table"`
				} `json:"specialRules"`
				UsesRules         []string `json:"usesRules"`
				ScoringObjectives []struct {
					Key         string `json:"key"`
					Text        string `json:"text"`
					Side        string `json:"side"`
					Kind        string `json:"kind"`
					Instruction string `json:"instruction"`
					Secret      bool   `json:"secret"`
					MaxPoints   int    `json:"maxPoints"`
					Nomination  *struct {
						From        string `json:"from"`
						AllowLeader string `json:"allowLeader"`
						Where       string `json:"where"`
					} `json:"nomination"`
					Options []struct {
						Text   string `json:"text"`
						Points int    `json:"points"`
					} `json:"options"`
				} `json:"scoringObjectives"`
				Roles []struct {
					Name string `json:"name"`
				} `json:"roles"`
				Rewards []struct {
					Effect    string `json:"effect"`
					Name      string `json:"name"`
					Who       string `json:"who"`
					Text      string `json:"text"`
					Condition struct {
						Kind  string `json:"kind"`
						Key   string `json:"key"`
						Label string `json:"label"`
					} `json:"condition"`
				} `json:"rewards"`
			} `json:"scenarioDetails"`
		} `json:"data"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return nil, err
	}
	var out []ScenarioInfo
	for _, name := range doc.Scenarios {
		info := ScenarioInfo{Name: name}
		for t, table := range doc.Campaign.Selection.Tables {
			if i := slices.Index(table.Scenarios, name); i >= 0 {
				info.Table, info.Roll = t+1, i+1
			}
		}
		for _, sc := range doc.ScenarioScoring {
			if sc.Name != name {
				continue
			}
			for _, group := range sc.Points {
				texts := make([]string, len(group))
				for i, p := range group {
					texts[i] = p.Text
				}
				info.Scoring = append(info.Scoring, strings.Join(texts, " "))
			}
		}
		for _, d := range doc.Data.ScenarioDetails {
			if d.Name != name {
				continue
			}
			info.About = ScenarioAbout{Outline: d.Outline, Description: d.Description}
			for _, sec := range []struct{ title, text string }{{"Armies", d.Armies}, {"Layout", d.Layout}, {"Starting positions", d.StartingPositions}, {"How the game ends", d.Objectives}} {
				if strings.TrimSpace(sec.text) != "" {
					info.About.Sections = append(info.About.Sections, ScenarioText{Title: sec.title, Text: sec.text})
				}
			}
			for _, r := range d.SpecialRules {
				text := ScenarioText{Title: r.Name, Text: r.Definition}
				if r.Table != nil {
					t := &ScenarioRuleTable{Lookup: r.Table.Lookup, Columns: r.Table.Columns}
					for _, row := range r.Table.Rows {
						max := -1
						if row.Max != nil {
							max = *row.Max
						}
						t.Rows = append(t.Rows, ScenarioTableRow{Min: row.Min, Max: max, Cells: row.Cells})
					}
					text.Table = t
				}
				info.About.Rules = append(info.About.Rules, text)
			}
			info.About.Uses = d.UsesRules
			for _, r := range d.Roles {
				info.About.Roles = append(info.About.Roles, r.Name)
			}
			for _, o := range d.ScoringObjectives {
				so := ScoringObjective{Label: ObjectiveLabel(o.Key), Key: o.Key, Text: o.Text, Side: o.Side, Kind: o.Kind, Instruction: o.Instruction, Secret: o.Secret, MaxPoints: o.MaxPoints}
				if o.Nomination != nil {
					so.Nomination = &ObjectiveNomination{From: o.Nomination.From, AllowLeader: o.Nomination.AllowLeader, Where: o.Nomination.Where}
				}
				for _, opt := range o.Options {
					so.Options = append(so.Options, ScoringTier{Text: opt.Text, Points: opt.Points})
				}
				info.ScoringObjectives = append(info.ScoringObjectives, so)
			}
			for _, rw := range d.Rewards {
				if rw.Condition.Kind != "companyFlag" && rw.Condition.Kind != "modelFlag" { // a reward that follows from the result: shown, not ticked
					who := map[string]string{"victor": "The winner", "eachCompanyWhoseLeaderSurvives": "Each company whose Leader is not removed"}[rw.Who]
					info.Prizes = append(info.Prizes, ScenarioPrize{Name: rw.Name, Who: who, Text: rw.Text})
					continue
				}
				if rw.Condition.Key == "" {
					continue
				}
				reward := ScenarioReward{Kind: "other", Text: rw.Text}
				for _, e := range doc.Campaign.AftermathEffects {
					if e.Key != rw.Effect {
						continue
					}
					reward.Text = e.Text
					switch e.Kind {
					case "influence", "experience":
						reward.Kind, reward.Amount = e.Kind, e.Amount
					case "freeEquipment":
						reward.Kind = "equipment"
					}
				}
				info.Objectives = append(info.Objectives, ScenarioObjective{Key: rw.Condition.Key, Label: rw.Condition.Label, Max: 1, Model: rw.Condition.Kind == "modelFlag", Rewards: []ScenarioReward{reward}})
			}
		}
		out = append(out, info)
	}
	slices.SortStableFunc(out, func(a, b ScenarioInfo) int {
		if a.Table != b.Table {
			if a.Table == 0 || b.Table == 0 { // unplaced scenarios last
				return b.Table - a.Table
			}
			return a.Table - b.Table
		}
		return a.Roll - b.Roll
	})
	return out, nil
}
