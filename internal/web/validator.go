package web

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The online validator (/tools/validate) checks the three rules files of a set exactly as "Check this file" does, for any signed-in
// player, and keeps nothing: no campaign, no ruleset row, no file, no hash. It is open to every account, so it is limited: signed
// in only, 16 MB per file, validatorPerMinute checks per account a minute and validatorSlots checks running at once.
const (
	validatorPerMinute = 6
	validatorSlots     = 2
)

// validatorGate holds the limits of the validator: a per-account sliding window and a global cap of checks running at once.
type validatorGate struct {
	mu    sync.Mutex
	hits  map[int64][]time.Time
	now   func() time.Time
	slots chan struct{}
}

func newValidatorGate() *validatorGate {
	return &validatorGate{hits: map[int64][]time.Time{}, now: time.Now, slots: make(chan struct{}, validatorSlots)}
}

// allow records a check by this account and says whether it is within the per-minute limit; the seconds to wait are returned
// when it is not.
func (g *validatorGate) allow(user int64) (ok bool, retryAfter int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	cutoff := now.Add(-time.Minute)
	kept := g.hits[user][:0]
	for _, at := range g.hits[user] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) >= validatorPerMinute {
		g.hits[user] = kept
		return false, int(kept[0].Add(time.Minute).Sub(now).Seconds()) + 1
	}
	g.hits[user] = append(kept, now)
	if len(g.hits) > 10_000 { // accounts that stopped checking would stay for ever: sweep the expired ones
		for k, v := range g.hits {
			if len(v) == 0 || !v[len(v)-1].After(cutoff) {
				delete(g.hits, k)
			}
		}
	}
	return true, 0
}

// acquire takes a running slot without waiting; release gives it back.
func (g *validatorGate) acquire() bool {
	select {
	case g.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (g *validatorGate) release() { <-g.slots }

// validatorSource says where each file of the last check came from.
type validatorSource struct {
	Label string
	Mine  bool   // the person's own file
	Name  string // the shipped file's name and version
}

// validatorResult is what the check swaps in: a report, or a one-line reason when there is no report to show.
type validatorResult struct {
	Report  *reportView
	Msg     string
	Sources []validatorSource
}

type validatorView struct {
	pageBase
	Vanilla  core.RulesetInfo // the shipped files, named on the form
	Glossary core.RulesetInfo
	Result   *validatorResult
}

func (s *Server) validatorView(r *http.Request, res *validatorResult) (validatorView, error) {
	v := validatorView{pageBase: s.base(r), Result: res}
	var err error
	if v.Vanilla, err = s.rulesets.Default(r.Context(), core.KindVanilla); err != nil {
		return v, err
	}
	if v.Glossary, err = s.rulesets.Default(r.Context(), core.KindGlossary); err != nil {
		return v, err
	}
	return v, nil
}

func (s *Server) handleValidatorPage(w http.ResponseWriter, r *http.Request) {
	view, err := s.validatorView(r, nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render.page(w, r, http.StatusOK, "validator", view)
}

// validatorAnswer sends the result: the fragment under the button for htmx, the whole page otherwise.
func (s *Server) validatorAnswer(w http.ResponseWriter, r *http.Request, status int, res *validatorResult) {
	if r.Header.Get("HX-Request") == "true" {
		s.render.partial(w, status, "validator", "validator_result", res)
		return
	}
	view, err := s.validatorView(r, res)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render.page(w, r, status, "validator", view)
}

// handleValidatorCheck checks the files of the form. Nothing is stored whatever the answer.
func (s *Server) handleValidatorCheck(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	if ok, wait := s.validator.allow(user.ID); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(wait))
		s.refuse(w, r, http.StatusTooManyRequests, "You have checked files "+strconv.Itoa(validatorPerMinute)+" times in the last minute. Wait "+strconv.Itoa(wait)+" seconds and try again.")
		return
	}
	if !s.validator.acquire() {
		w.Header().Set("Retry-After", "5")
		s.refuse(w, r, http.StatusTooManyRequests, "The checker is busy with other files right now. Try again in a few seconds.")
		return
	}
	defer s.validator.release()

	const tooBig = "Choose .json files of at most 16 MB each."
	r.Body = http.MaxBytesReader(w, r.Body, 3*maxUpload+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		s.validatorAnswer(w, r, http.StatusUnprocessableEntity, &validatorResult{Msg: tooBig})
		return
	}
	defer r.MultipartForm.RemoveAll() // big parts spill to disk: nothing is kept
	var set core.RulesetSet
	res := &validatorResult{}
	for _, f := range []struct {
		field string
		dst   *[]byte
		kind  core.RulesetKind
	}{{"vanilla", &set.Vanilla, core.KindVanilla}, {"glossary", &set.Glossary, core.KindGlossary}, {"battle_companies", &set.BattleCompanies, core.KindBattleCompanies}} {
		file, _, err := r.FormFile(f.field)
		if errors.Is(err, http.ErrMissingFile) {
			continue
		}
		if err != nil {
			s.validatorAnswer(w, r, http.StatusUnprocessableEntity, &validatorResult{Msg: tooBig})
			return
		}
		content, err := io.ReadAll(io.LimitReader(file, maxUpload+1))
		file.Close()
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		if len(content) > maxUpload {
			s.validatorAnswer(w, r, http.StatusUnprocessableEntity, &validatorResult{Msg: tooBig})
			return
		}
		*f.dst = append([]byte{}, content...)
	}
	if set.BattleCompanies == nil {
		s.validatorAnswer(w, r, http.StatusUnprocessableEntity, &validatorResult{Msg: "Choose your Battle Companies file. The vanilla file and the glossary are optional."})
		return
	}
	res.Sources = []validatorSource{{Label: core.KindVanilla.Label(), Mine: set.Vanilla != nil}, {Label: core.KindGlossary.Label(), Mine: set.Glossary != nil}, {Label: core.KindBattleCompanies.Label(), Mine: true}}
	rep, err := s.rulesets.CheckSet(r.Context(), set)
	switch {
	case err == nil:
		res.Report = newReportView(rep)
		res.Report.OK = true
		res.Report.Note = "Nothing was stored and no campaign was created."
		s.validatorAnswer(w, r, http.StatusOK, res)
	case errors.Is(err, core.ErrInvalidRuleset):
		if len(rep.Issues()) > 0 {
			res.Report = newReportView(rep)
			res.Report.Note = "Nothing was stored and no campaign was created."
		} else {
			res.Msg = reason(err)
		}
		s.validatorAnswer(w, r, http.StatusUnprocessableEntity, res)
	default:
		s.serverError(w, r, err)
	}
}
