package lean

// AgainstTheOdds is `campaign.againstTheOdds`: the bonuses the lower rated company may take after the scenario roll, and how the budget and the
// eligibility work. core.LeanATO reads the bonuses, their kits and the budget; the prose keys are shown to the players or kept for the reader of the file.
type AgainstTheOdds struct {
	Text       string      `json:"text,omitempty"`
	Bonuses    []ATOBonus  `json:"bonuses,omitempty"`
	Budget     ATOBudget   `json:"budget,omitempty"`
	Eligible   ATOEligible `json:"eligible,omitempty"`
	SelectedAt string      `json:"selectedAt,omitempty" enum:"afterScenarioRollBeforeSetup"`
}

// ATOBudget is the rule that the bonuses may not take the lower company above its opponent's Rating.
type ATOBudget struct {
	Text      string `json:"text,omitempty"`
	Inclusive bool   `json:"inclusive,omitempty"`
	Rule      string `json:"rule,omitempty" enum:"ratingAfterBonusesNotAboveOpponent"`
}

// ATOEligible is who may choose bonuses.
type ATOEligible struct {
	Text                string `json:"text,omitempty"`
	Company             string `json:"company,omitempty" enum:"lowerRating"`
	Compare             string `json:"compare,omitempty" enum:"strictlyLower"`
	RatingBeforeBonuses bool   `json:"ratingBeforeBonuses,omitempty"`
}

// ATOBonus is one Against the Odds bonus. The keys influence, experience and reroll are plain switches (gains, count); toolKit (kits, purchase),
// wanderer and ambush are structured bonuses the app has its own windows for.
type ATOBonus struct {
	Key         string      `json:"key" required:"true" min:"1"`
	Name        string      `json:"name" required:"true" min:"1"`
	Text        string      `json:"text,omitempty"`
	Rating      int         `json:"rating" required:"true" min:"0" max:"1000"`
	AppliesIn   []string    `json:"appliesIn,omitempty" enum:"setup|game|aftermath|scenarioChoice" noun:"phase"`
	Gains       ATOGains    `json:"gains,omitempty"`
	Count       int         `json:"count,omitempty" min:"1" max:"20"`
	Affects     string      `json:"affects,omitempty" enum:"singleDieOrSingleResult"`
	CarriesOver bool        `json:"carriesOver,omitempty"`
	Expires     string      `json:"expires,omitempty" enum:"endOfMatchAftermath|endOfMatch"`
	UsableIn    []string    `json:"usableIn,omitempty" enum:"game|warsAftermath" noun:"phase"`
	Temporary   bool        `json:"temporary,omitempty"`
	Grants      ATOGrant    `json:"grants,omitempty"`
	Kits        []ATOKit    `json:"kits,omitempty"`
	Purchase    ATOPurchase `json:"purchase,omitempty"`
}

// ATOGains are the rewards a bonus adds after the match.
type ATOGains struct {
	Influence          *ATOReward `json:"influence,omitempty"`
	ExperiencePerModel *ATOReward `json:"experiencePerModel,omitempty"`
}

// ATOReward is a reward by result: Win for a Victory, Other for a Draw or a Defeat.
type ATOReward struct {
	Win   int `json:"win" required:"true" min:"0" max:"20"`
	Other int `json:"other" required:"true" min:"0" max:"20"`
}

// ATOGrant is what a structured bonus gives: loaned equipment (Tool Kit), a temporary model (Wanderer) or the choice of the scenario (Ambush!).
// The app reads the bonus by its key; these keys state the rules in data for the reader of the file.
type ATOGrant struct {
	Kind                           string   `json:"kind,omitempty" enum:"loanedEquipment|temporaryModel|chooseScenario"`
	AddsRating                     bool     `json:"addsRating,omitempty"`
	AssignAt                       string   `json:"assignAt,omitempty" enum:"beforeSetup"`
	AssignTo                       string   `json:"assignTo,omitempty" enum:"anyModelOfTheMatch"`
	CarriesOver                    bool     `json:"carriesOver,omitempty"`
	Expires                        string   `json:"expires,omitempty" enum:"endOfMatchAftermath|endOfMatch"`
	Ignores                        []string `json:"ignores,omitempty" enum:"capacity|alignment" noun:"rule"`
	OncePerKit                     bool     `json:"oncePerKit,omitempty"`
	StillApplies                   []string `json:"stillApplies,omitempty" enum:"heroOnly" noun:"restriction"`
	UsableByTemporaryWanderer      bool     `json:"usableByTemporaryWanderer,omitempty"`
	Count                          int      `json:"count,omitempty" min:"1" max:"20"`
	CountsTowardsBreakPoint        bool     `json:"countsTowardsBreakPoint,omitempty"`
	CountsTowardsMaxModels         bool     `json:"countsTowardsMaxModels,omitempty"`
	ExclusiveWithPermanentWanderer bool     `json:"exclusiveWithPermanentWanderer,omitempty"`
	From                           string   `json:"from,omitempty" min:"1"`
	GainsExperience                bool     `json:"gainsExperience,omitempty"`
	GoldCost                       int      `json:"goldCost,omitempty" min:"0" max:"1000"`
	InfluenceCost                  int      `json:"influenceCost,omitempty" min:"0" max:"1000"`
	PersistsAfterMatch             bool     `json:"persistsAfterMatch,omitempty"`
	RatingCost                     string   `json:"ratingCost,omitempty" enum:"bonusRating"`
	RollsInjuries                  bool     `json:"rollsInjuries,omitempty"`
	Rules                          string   `json:"rules,omitempty"`
	TakesRosterSlot                bool     `json:"takesRosterSlot,omitempty"`
	ChosenBy                       string   `json:"chosenBy,omitempty" enum:"lowerRatedCompany"`
	Duration                       string   `json:"duration,omitempty" enum:"thisMatch"`
	Replaces                       string   `json:"replaces,omitempty" enum:"scenarioTableRoll"`
	RolesBy                        string   `json:"rolesBy,omitempty" enum:"scenarioRules"`
}

// ATOKit is a kit of the Tool Kit bonus.
type ATOKit struct {
	Name  string       `json:"name" required:"true" min:"1"`
	Items []ATOKitItem `json:"items" required:"true" min:"1"`
}

// ATOKitItem is an item of a kit and how many copies the kit holds.
type ATOKitItem struct {
	Equipment string `json:"equipment" required:"true" min:"1"`
	Count     int    `json:"count,omitempty" min:"1" max:"20"`
}

// ATOPurchase is how many kits the Tool Kit bonus may be bought for.
type ATOPurchase struct {
	Chooses      string `json:"chooses,omitempty" enum:"oneKit"`
	DistinctKits bool   `json:"distinctKits,omitempty"`
	MaxPurchases int    `json:"maxPurchases,omitempty" min:"1" max:"20"`
	RatingPerKit bool   `json:"ratingPerKit,omitempty"`
}
