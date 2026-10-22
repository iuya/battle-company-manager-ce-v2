package core

// ScenarioTable is the table (1-3) the first D6 of the scenario selection picks: 1-2 the first, 3-4 the second, 5-6 the third.
// It is 0 for a die outside 1-6.
func ScenarioTable(d int) int {
	if d < 1 || d > 6 {
		return 0
	}
	return (d + 1) / 2
}

// ScenarioByRolls maps the two D6 of the scenario selection (campaign.scenarioSelection) onto a scenario of the list: the
// first die picks the table, the second the scenario on it (its Roll). It reports false when a die is outside 1-6 or the
// rules file places no scenario there.
func ScenarioByRolls(list []ScenarioInfo, first, second int) (ScenarioInfo, bool) {
	table := ScenarioTable(first)
	if table == 0 || second < 1 || second > 6 {
		return ScenarioInfo{}, false
	}
	for _, sc := range list {
		if sc.Table == table && sc.Roll == second {
			return sc, true
		}
	}
	return ScenarioInfo{}, false
}

// Rating is the Battle Company Rating of a side: the points of its companies (injured units counted at full value when the
// company knows it).
func (s MatchSide) Rating() int {
	n := 0
	for _, c := range s.Companies {
		if c.FullPoints > 0 {
			n += c.FullPoints
		} else {
			n += c.Points
		}
	}
	return n
}

// TopDog says which side of a two-sided match has the higher rating (the Top Dog, 0-based index) and which the Underdog.
// A tie, or a match that does not have exactly two sides, has neither: both are -1.
func (m Match) TopDog() (top, under int) {
	if len(m.Sides) != 2 || m.Sides[0].Rating() == m.Sides[1].Rating() {
		return -1, -1
	}
	if m.Sides[0].Rating() > m.Sides[1].Rating() {
		return 0, 1
	}
	return 1, 0
}
