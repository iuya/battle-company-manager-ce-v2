package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"github.com/iuya/battle-company-manager-ce-v2/internal/postgres"
	"github.com/iuya/battle-company-manager-ce-v2/internal/web"
)

// TestLiveActionsSurviveExtremeValues (robustness): the sweep above puts the same hostile value into every field at once, so
// a request is almost always refused at the first field. This one sends every live match POST (the action kinds of
// {company}/actions, undo, courage roll, the end checks, the secret objectives, the end-of-match votes) with a valid form in which ONE
// field at a time takes an extreme but individually plausible value (2^31 wounds, negative counts, the unit of another company, match
// or campaign, a wrong zone, an ended match, ...), also repeated, against the live state of a real seeded match. Every answer is a 2xx,
// 3xx or 4xx, never a 5xx or a panic, and after every request the folded state of both companies is sound: every tracker within
// 0..start, no negative or overflowing Break Point counter. SWEEP_FULL=1 tries more values per field.
func TestLiveActionsSurviveExtremeValues(t *testing.T) {
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	if testing.Short() {
		t.Skip("slow")
	}
	t.Parallel() // its own database (a copy of the seeded template), no state shared with the other tests
	ctx := context.Background()
	dsn := seededDatabase(ctx, t, base)
	store, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	b, err := newBackendWithDice(ctx, store, &demoDice{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	logs := captureLogs(t)
	srv, err := web.NewServer(web.Config{}, b.campaigns, b.rulesets, b.glossaries, b.auth)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetAccountLinks(b.invitations, b.recovery)
	h := srv.Handler()

	ints := func(q string, args ...any) []int64 {
		rows, err := conn.Query(ctx, q, args...)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var n int64
			if err := rows.Scan(&n); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, n)
		}
		return ids
	}
	one := func(q string, args ...any) int64 {
		ids := ints(q, args...)
		if len(ids) == 0 {
			t.Fatalf("no row for %s", q)
		}
		return ids[0]
	}
	camp := one(`SELECT id FROM campaigns WHERE name LIKE 'Matches:%'`)
	// the first open match with two companies and a scenario-less setup; every company of it has live units
	open := ints(`SELECT m.id FROM matches m WHERE m.campaign_id = $1 AND NOT m.ended ORDER BY m.id`, camp)
	ended := ints(`SELECT m.id FROM matches m WHERE m.campaign_id = $1 AND m.ended ORDER BY m.id`, camp)
	if len(open) < 2 || len(ended) == 0 {
		t.Fatalf("the demo world has %d open and %d ended matches", len(open), len(ended))
	}
	mt, other, endedMatch := open[0], open[1], ended[0]
	cos := ints(`SELECT company_id FROM match_side_companies WHERE match_id = $1 ORDER BY position, slot`, mt)
	otherCos := ints(`SELECT company_id FROM match_side_companies WHERE match_id = $1 ORDER BY position, slot`, other)
	endedCos := ints(`SELECT company_id FROM match_side_companies WHERE match_id = $1 ORDER BY position, slot`, endedMatch)
	finished := one(`SELECT id FROM campaigns WHERE name LIKE 'Finished campaign%'`)

	tok, _, admin, err := b.auth.Login(ctx, "admin@example.com", "demo-password")
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: "bcm_session", Value: tok}
	unitsOf := func(company int64) []core.LiveUnit {
		lc, err := b.campaigns.Live(ctx, admin, camp, company)
		if err != nil {
			t.Fatalf("live %d: %v", company, err)
		}
		return lc.Units
	}
	a, c := cos[0], cos[1]
	au, cu := unitsOf(a), unitsOf(c)
	if len(au) < 3 || len(cu) < 2 {
		t.Fatalf("units %d / %d", len(au), len(cu))
	}
	otherUnit := unitsOf(otherCos[0])[0].UnitID
	foreign := one(`SELECT min(id) FROM company_units WHERE company_id IN (SELECT id FROM companies WHERE campaign_id = $1)`, finished)
	endedUnit := one(`SELECT min(id) FROM company_units WHERE company_id = $1`, endedCos[0])
	hero := au[0].UnitID // the Leader: has Might, Will and Fate
	str := func(n int64) string { return strconv.FormatInt(n, 10) }

	full := os.Getenv("SWEEP_FULL") != ""
	numbers := []string{"", "0", "-1", "1", "2", "9", "10", "100", " 3 ", "+3", "3.5", "0x10", "2147483647", "2147483648", "-2147483649",
		"4294967296", "9223372036854775807", "9223372036854775808", "-9223372036854775808", "99999999999999999999999"}
	texts := []string{"", " ", "x", "courage", "left", "start", "casualties", "wounds", "might", "will", "fate", "Wounds", "mount", "rider",
		strings.Repeat("n", 201), "\x00", "a\x00b", "\xff\xfe", "<b>x</b>'; --", "ß‮"}
	bools := []string{"", "0", "1", "2", "-1", "true", "on"}
	if !full {
		numbers = []string{"", "0", "-1", "2", "10", "2147483648", "-2147483649", "9223372036854775807", "9223372036854775808", "-9223372036854775808"}
		texts = []string{"", "courage", "x", strings.Repeat("n", 201), "\x00", "\xff\xfe"}
		bools = []string{"", "0", "1", "2"}
	}
	ids := func(own ...int64) []string {
		v := append([]string{}, numbers...)
		for _, n := range own {
			v = append(v, str(n))
		}
		return v
	}
	unitIDs := ids(cu[0].UnitID, otherUnit, foreign, endedUnit, -a, -c, 1)

	type kind struct {
		fields map[string]string   // a valid form
		vary   map[string][]string // field -> extreme values, one at a time
	}
	stat := func(name string) kind {
		return kind{map[string]string{"kind": "stat", "unit": str(hero), "stat": name, "delta": "-1"},
			map[string][]string{"unit": unitIDs, "stat": append([]string{"wounds", "might", "will", "fate", "mountwounds", "WOUNDS"}, texts...), "delta": numbers, "set": numbers}}
	}
	kinds := map[string]kind{
		"stat wounds": stat("wounds"), "stat might": stat("might"), "stat will": stat("will"), "stat fate": stat("fate"),
		"wound": {map[string]string{"kind": "wound", "unit": str(au[1].UnitID), "target": str(cu[0].UnitID), "part": "rider", "caused": "1", "saved": "0"},
			map[string][]string{"unit": unitIDs, "target": unitIDs, "part": append([]string{"rider", "mount"}, texts...), "caused": numbers, "saved": numbers}},
		"fate":    {map[string]string{"kind": "fate", "unit": str(hero), "count": "1"}, map[string][]string{"unit": unitIDs, "count": numbers}},
		"retreat": {map[string]string{"kind": "retreat", "unit": str(au[2].UnitID), "why": "left", "note": "gone"}, map[string][]string{"unit": unitIDs, "why": texts, "note": texts}},
		"died":    {map[string]string{"kind": "died", "unit": str(au[2].UnitID), "note": "x"}, map[string][]string{"unit": unitIDs, "note": texts}},
		"mount":   {map[string]string{"kind": "mount", "unit": str(hero), "mounted": "0"}, map[string][]string{"unit": unitIDs, "mounted": bools}},
		"return":  {map[string]string{"kind": "return", "unit": str(au[2].UnitID)}, map[string][]string{"unit": unitIDs}},
		"broken":  {map[string]string{"kind": "broken", "unit": str(hero), "standFast": "1"}, map[string][]string{"unit": unitIDs, "standFast": bools}},
		"turn":    {map[string]string{"kind": "turn", "turn": "1"}, map[string][]string{"turn": numbers}},
		"breakAdjust": {map[string]string{"kind": "breakAdjust", "field": "start", "delta": "1"},
			map[string][]string{"field": texts, "delta": numbers}},
		"courageSkip": {map[string]string{"kind": "courageSkip", "unit": str(hero), "reason": "engaged"}, map[string][]string{"unit": unitIDs, "reason": append([]string{"standFast", "engaged"}, texts...)}},
		"allIsLost":   {map[string]string{"kind": "allIsLost", "flee": "0"}, map[string][]string{"flee": bools}},
		"itemUsed":    {map[string]string{"kind": "itemUsed", "unit": str(hero), "item": "Healing Potion", "index": "0"}, map[string][]string{"unit": unitIDs, "item": texts, "index": numbers}},
		"unknown":     {map[string]string{"kind": "objective", "unit": str(hero)}, map[string][]string{"kind": append([]string{"objective", "mvpVote", "secretNominate", "endProposed", "endConfirmed", "endOverride"}, texts...)}},
	}

	var failures []string
	var requests int
	statuses := map[int]int{}
	check := func(what string) {
		for _, co := range append(append([]int64{}, cos...), otherCos[0]) {
			lc, err := b.campaigns.Live(ctx, admin, camp, co)
			if err != nil {
				failures = append(failures, fmt.Sprintf("after %s: Live(%d): %v", what, co, err))
				continue
			}
			for _, u := range lc.Units {
				for name, st := range map[string]core.LiveStat{"wounds": u.Wounds, "might": u.Might, "will": u.Will, "fate": u.Fate, "mount": u.MountWounds} {
					if st.Current < 0 || st.Current > st.Start || st.Start < 0 || st.Start > 1000 {
						failures = append(failures, fmt.Sprintf("after %s: unit %d %s is %d of %d", what, u.UnitID, name, st.Current, st.Start))
					}
				}
			}
			br := lc.Break
			if br.StartingModels < 0 || br.Casualties < 0 || br.Threshold100 < 0 || br.Percent < 0 || br.StartingModels > 10000 || br.Casualties > 10000 || lc.Turn < 0 || lc.Turn > 1000 {
				failures = append(failures, fmt.Sprintf("after %s: company %d break %+v turn %d", what, co, br, lc.Turn))
			}
		}
		if _, err := b.campaigns.Log(ctx, admin, camp, mt, 0, 50); err != nil {
			failures = append(failures, fmt.Sprintf("after %s: Log: %v", what, err))
		}
	}
	send := func(path string, form map[string]string) {
		f := url.Values{}
		for k, v := range form {
			f.Set(k, v)
		}
		req := httptest.NewRequest("POST", path, strings.NewReader(f.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		func() {
			defer func() {
				if p := recover(); p != nil {
					failures = append(failures, fmt.Sprintf("PANIC escaped the middleware: %s: %v", path, p))
				}
			}()
			h.ServeHTTP(rec, req)
		}()
		requests++
		statuses[rec.Code]++
		if rec.Code >= 500 {
			v := f.Encode()
			if len(v) > 100 {
				v = v[:100] + "..."
			}
			failures = append(failures, fmt.Sprintf("%d POST %s <- %s", rec.Code, path, v))
		}
		if rec.Code >= 400 && rec.Code < 500 && strings.TrimSpace(rec.Body.String()) == "" {
			failures = append(failures, fmt.Sprintf("%d POST %s answers without a message <- %s", rec.Code, path, f.Encode()))
		}
	}
	actions := func(camp, match, company int64) string {
		return fmt.Sprintf("/campaigns/%d/matches/%d/live/%d/actions", camp, match, company)
	}
	names := make([]string, 0, len(kinds))
	for n := range kinds {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		k := kinds[name]
		send(actions(camp, mt, a), k.fields)
		send(actions(camp, mt, a), k.fields) // the same submit twice
		fields := make([]string, 0, len(k.vary))
		for f := range k.vary {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		for _, f := range fields {
			for _, v := range k.vary[f] {
				form := map[string]string{}
				for kk, vv := range k.fields {
					form[kk] = vv
				}
				form[f] = v
				send(actions(camp, mt, a), form)
			}
		}
		check("kind " + name)
		// the same valid form against the wrong place
		for _, p := range []string{
			actions(camp, mt, c),                   // a unit of another company of the match
			actions(camp, mt, otherCos[0]),         // a company of another match
			actions(camp, other, a),                // a match the company is not in
			actions(camp, endedMatch, endedCos[0]), // an ended match
			actions(camp, endedMatch, a),
			actions(finished, mt, a), // another (finished) campaign
			actions(finished, other, otherCos[0]),
			actions(camp, mt, 0), actions(camp, mt, 1<<62),
			actions(camp, 1<<62, a),
		} {
			send(p, k.fields)
		}
		check("kind " + name + " in the wrong place")
	}
	// an unfinished game whose units all left: actions on retreated and dead units
	for _, u := range au {
		for _, kd := range []string{"died", "retreat", "return", "wound", "fate", "broken", "mount"} {
			form := map[string]string{}
			for kk, vv := range kinds[kd].fields {
				form[kk] = vv
			}
			form["unit"] = str(u.UnitID)
			send(actions(camp, mt, a), form)
		}
	}
	check("units out of the field")

	// undo: every entry, a second time, a foreign entry, hostile ids
	entries := ints(`SELECT id FROM match_live_log WHERE match_id = $1 ORDER BY id DESC`, mt)
	foreignEntry := ints(`SELECT id FROM match_live_log WHERE match_id <> $1 ORDER BY id LIMIT 1`, mt)
	entries = append(entries, foreignEntry...)
	entries = append(entries, 0, -1, 1<<62)
	for _, e := range entries {
		for _, cp := range []string{str(a), str(c), "", "0", "-1", "99999999999999999999", str(otherCos[0])} {
			for _, from := range []string{"", "log", "card", "end", "endcard", "secret", "x"} {
				send(fmt.Sprintf("/campaigns/%d/matches/%d/live/log/%d/undo", camp, mt, e), map[string]string{"company": cp, "from": from})
				if !full {
					break
				}
			}
		}
	}
	for _, e := range entries[:min(len(entries), 3)] { // the wrong match and campaign
		send(fmt.Sprintf("/campaigns/%d/matches/%d/live/log/%d/undo", camp, other, e), map[string]string{"company": str(a)})
		send(fmt.Sprintf("/campaigns/%d/matches/%d/live/log/%d/undo", finished, mt, e), map[string]string{"company": str(a)})
	}
	check("undo")

	// the courage roll
	for _, u := range []int64{hero, cu[0].UnitID, otherUnit, foreign, endedUnit, 0, -1, 1 << 62} {
		for _, r1 := range numbers {
			p := fmt.Sprintf("/campaigns/%d/matches/%d/live/%d/units/%d/courage/roll", camp, mt, a, u)
			send(p, map[string]string{"roll": r1, "roll2": "3"})
			send(p, map[string]string{"roll": "3", "roll2": r1})
		}
	}
	check("courage roll")

	// the end checks: objective, mvp vote, secret objectives
	for _, co := range []int64{a, c, otherCos[0], endedCos[0], 0, -1, 1 << 62} {
		for _, m := range []int64{mt, other, endedMatch, 0, -1} {
			for _, u := range unitIDs {
				send(fmt.Sprintf("/campaigns/%d/matches/%d/live/%d/objective", camp, m, co), map[string]string{"unit": u, "on": "1", "page": str(a)})
				send(fmt.Sprintf("/campaigns/%d/matches/%d/live/%d/mvp", camp, m, co), map[string]string{"unit": u, "page": str(a)})
				if !full {
					break
				}
			}
			for _, on := range bools {
				send(fmt.Sprintf("/campaigns/%d/matches/%d/live/%d/objective", camp, m, co), map[string]string{"unit": str(hero), "on": on})
			}
			for _, v := range numbers {
				send(fmt.Sprintf("/campaigns/%d/matches/%d/live/%d/mvp", camp, m, co), map[string]string{"unit": v, "page": v})
				secret := fmt.Sprintf("/campaigns/%d/matches/%d/secret/%d", camp, m, co)
				send(secret, map[string]string{"kind": "done", "key": "x", "tier": v})
				send(secret, map[string]string{"kind": "nominate", "key": "x", "unit": v})
			}
			for _, v := range texts {
				secret := fmt.Sprintf("/campaigns/%d/matches/%d/secret/%d", camp, m, co)
				send(secret, map[string]string{"kind": "nominate", "key": v, "note": "a tree"})
				send(secret, map[string]string{"kind": "nominate", "key": "x", "note": v})
				send(secret, map[string]string{"kind": v, "key": "x"})
			}
		}
	}
	check("end checks and secrets")

	// the pending end of a match
	for _, step := range []string{"propose", "confirm", "withdraw", "cancel", "override"} {
		for _, m := range []int64{mt, other, endedMatch, 0, -1, 1 << 62} {
			for _, v := range numbers {
				send(fmt.Sprintf("/campaigns/%d/matches/%d/end/%s", camp, m, step), map[string]string{"revision": v, "company": v, "page": v, "note": strings.Repeat("n", 300)})
			}
		}
	}
	check("end of match")

	// a long run of repeated valid actions must stay sound: wounds up to the end of the game, then back
	for i := 0; i < 40; i++ {
		send(actions(camp, mt, a), map[string]string{"kind": "stat", "unit": str(hero), "stat": "wounds", "delta": "-1"})
		send(actions(camp, mt, a), map[string]string{"kind": "stat", "unit": str(hero), "stat": "wounds", "delta": "1"})
		send(actions(camp, mt, a), map[string]string{"kind": "breakAdjust", "field": "casualties", "delta": "9"})
		send(actions(camp, mt, a), map[string]string{"kind": "breakAdjust", "field": "start", "delta": "9"})
	}
	check("repeated submits")

	sort.Strings(failures)
	failures = dedupeStrings(failures)
	for _, f := range failures {
		t.Error(f)
	}
	if strings.Contains(logs(), "level=ERROR msg=panic") {
		t.Errorf("a handler panicked (recovered by the middleware); first lines:\n%s", firstLines(logs(), "msg=panic", 5))
	}
	seen := map[string]bool{}
	for _, l := range strings.Split(logs(), "\n") {
		if strings.Contains(l, "level=ERROR") {
			if i := strings.Index(l, "stack="); i > 0 {
				l = l[:i]
			}
			if len(l) > 300 {
				l = l[:300]
			}
			if !seen[l] {
				seen[l] = true
				t.Log(l)
			}
		}
	}
	if statuses[200] < 20 { // the sweep must reach the state, not only be refused at the door
		t.Errorf("only %d requests were accepted: the valid forms no longer work", statuses[200])
	}
	t.Logf("%d requests, answers %v", requests, statuses)
}

func dedupeStrings(in []string) []string {
	out := in[:0]
	for i, s := range in {
		if i == 0 || s != in[i-1] {
			out = append(out, s)
		}
	}
	return out
}
