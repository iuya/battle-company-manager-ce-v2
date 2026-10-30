package lean

// MatchInput is `campaign.matchInput`: what the players enter per match, described for the people who read the file and for tools that build a form
// from it. The app builds its own match forms from the engine's tables (the experience gains, the scenarios, Against the Odds) and does not read this
// description.
type MatchInput struct {
	Text       string         `json:"text,omitempty"`
	Aftermath  MatchAftermath `json:"aftermath,omitempty"`
	Gold       MatchGold      `json:"gold,omitempty"`
	PerCompany []InputField   `json:"perCompany,omitempty"`
	PerModel   []InputField   `json:"perModel,omitempty"`
	Result     MatchResult    `json:"result,omitempty"`
}

// MatchAftermath is the order in which the War's Aftermath is applied.
type MatchAftermath struct {
	Order   []string `json:"order,omitempty" enum:"injuries|experience|progression|influence" noun:"aftermath step"`
	Rewards string   `json:"rewards,omitempty"`
}

// MatchGold says that no scenario of the rulebook rewards Gold.
type MatchGold struct {
	Text            string `json:"text,omitempty"`
	ScenarioRewards bool   `json:"scenarioRewards,omitempty"`
}

// MatchResult is how the winner of a match is decided.
type MatchResult struct {
	DecidedBy string   `json:"decidedBy,omitempty" enum:"victoryPoints"`
	Outcomes  []string `json:"outcomes,omitempty" enum:"win|draw|loss" noun:"outcome"`
	Tie       string   `json:"tie,omitempty" enum:"draw"`
}

// InputField is one thing the players enter, per company or per model.
type InputField struct {
	Key           string   `json:"key" required:"true" min:"1"`
	Text          string   `json:"text,omitempty"`
	Type          string   `json:"type,omitempty" enum:"enum|multiSelect|select|integer|count|flag|flags|scoringObjectives"`
	Default       Free     `json:"default,omitempty"` // open: true or false for a flag, a number for a count, the chosen word for an enum
	Options       Free     `json:"options,omitempty"` // open: the list of the words, or the path of the table they come from
	When          string   `json:"when,omitempty"`
	Who           string   `json:"who,omitempty"`
	Feeds         []string `json:"feeds,omitempty" enum:"influence|experience|rerolls|rewards|itemUse|breakPoint|injuries" noun:"table"`
	Min           *int     `json:"min,omitempty" min:"0" max:"100"`
	Optional      bool     `json:"optional,omitempty"`
	Computed      bool     `json:"computed,omitempty"`
	ComputedFrom  string   `json:"computedFrom,omitempty"`
	From          string   `json:"from,omitempty"`
	Experience    string   `json:"experience,omitempty"`
	DeclaredBy    string   `json:"declaredBy,omitempty" enum:"opponent"`
	Inferred      []string `json:"inferred,omitempty"`
	MaxPerCompany int      `json:"maxPerCompany,omitempty" min:"1" max:"20"`
}
