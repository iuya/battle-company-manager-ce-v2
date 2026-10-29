package core

import "time"

// PracticeNothingToRoll is the line of the aftermath of a practice match and the reason every write to it is refused.
const PracticeNothingToRoll = "Practice match: nothing to roll."

// ErrPracticeNothingToRoll is the refusal of every aftermath write on a practice match (docs/practice-match-spec.md, section 3).
var ErrPracticeNothingToRoll = Refuse("%s", PracticeNothingToRoll)

// practiceAftermath is the aftermath of a company in a practice match: done, empty, never stored. The repository derives the same status.
func practiceAftermath(e *amEnv) Aftermath {
	a := Aftermath{MatchID: e.matchID, CompanyID: e.companyID, Status: AftermathDone, Revision: 1, Outcome: e.outcome, Fled: e.fled,
		Access: e.access, Practice: true}
	var on time.Time
	if e.match.EndedOn != nil {
		on = *e.match.EndedOn
	}
	a.Lines = []AftermathLine{{Kind: "note", Text: PracticeNothingToRoll, Match: e.match.Number, MatchID: e.matchID, On: on}}
	return a
}
