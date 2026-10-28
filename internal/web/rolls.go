package web

import (
	"net/http"
	"strconv"
)

// maxRollFields is how many dice a form can carry (a dice spec has at most 10 dice: docs/definitions-lean-design.md 12.1).
const maxRollFields = 10

// enteredRolls reads the numbers a roll form carries as roll, roll2, roll3 ...: the dice (or sums) the player typed or the browser
// threw. nil when every one is left on "Roll for me" (0, empty or absent). The count and the range of each are the core's to check
// against the rules file, which knows the dice.
func enteredRolls(r *http.Request) []int {
	_ = r.ParseForm()
	var out []int
	typed := false
	for i := 1; i <= maxRollFields; i++ {
		name := "roll"
		if i > 1 {
			name += strconv.Itoa(i)
		}
		v, ok := r.PostForm[name]
		if !ok || len(v) == 0 {
			break
		}
		n, _ := strconv.Atoi(v[0])
		out = append(out, n)
		typed = typed || n > 0
	}
	if !typed {
		return nil
	}
	return out
}
