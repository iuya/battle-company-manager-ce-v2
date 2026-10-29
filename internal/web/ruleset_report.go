package web

import (
	"errors"
	"io"
	"net/http"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// maxIssuesShown is how many problems the report lists; the rest is counted ("and N more"). The core caps a report itself
// (RulesetReport.Truncated); this keeps the page small whatever the core sends.
const maxIssuesShown = 200

// issueFile is the problems of one file of an upload, in report order.
type issueFile struct {
	File   string
	Issues []core.RulesetIssue
}

// reportView is a RulesetReport as the page draws it: grouped by file, at most maxIssuesShown rows, the rest counted.
type reportView struct {
	Files    []issueFile
	Errors   int // blocking problems found
	Warnings int
	More     int    // problems not listed (beyond the cap here and the core's own cap)
	Note     string // what did not happen: "Nothing was stored."
	OK       bool   // a dry run that found nothing blocking
}

// rulesetIssues reads the report out of an upload error (errors.As also finds the merged report of the campaign form). It is
// nil for an error that is not a report; the caller then shows reason(err).
func rulesetIssues(err error) *reportView {
	var rep *core.RulesetReport
	if err == nil || !errors.As(err, &rep) || rep == nil {
		return nil
	}
	return newReportView(*rep)
}

func newReportView(rep core.RulesetReport) *reportView {
	v := &reportView{Errors: rep.Errors(), Warnings: len(rep.Warnings()), More: rep.Truncated(), Note: "Nothing was stored."}
	issues := rep.Issues()
	if len(issues) > maxIssuesShown {
		v.More += len(issues) - maxIssuesShown
		issues = issues[:maxIssuesShown]
	}
	at := map[string]int{} // file -> index in v.Files
	for _, i := range issues {
		k, ok := at[i.File]
		if !ok {
			k = len(v.Files)
			at[i.File] = k
			v.Files = append(v.Files, issueFile{File: i.File})
		}
		v.Files[k].Issues = append(v.Files[k].Issues, i)
	}
	return v
}

// handleAdminRulesetCheck is the "Check this file" dry run: it validates the chosen file and stores nothing. The answer is the
// report fragment only, swapped under the button.
func (s *Server) handleAdminRulesetCheck(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+1<<20)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		s.render.partial(w, http.StatusUnprocessableEntity, "admin_rulesets", "ruleset_check_result", checkResult{Msg: "Choose a .json file of at most 16 MB."})
		return
	}
	kind := core.RulesetKind(r.PostFormValue("kind"))
	if !kind.Valid() {
		s.render.partial(w, http.StatusUnprocessableEntity, "admin_rulesets", "ruleset_check_result", checkResult{Msg: "Choose the kind of file."})
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		s.render.partial(w, http.StatusUnprocessableEntity, "admin_rulesets", "ruleset_check_result", checkResult{Msg: "Choose a .json file."})
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rep, err := s.rulesets.CheckSet(r.Context(), setOf(kind, content))
	switch {
	case err == nil:
		v := newReportView(rep)
		v.OK = true
		s.render.partial(w, http.StatusOK, "admin_rulesets", "ruleset_check_result", checkResult{Report: v})
	case errors.Is(err, core.ErrInvalidRuleset):
		v := newReportView(rep)
		if len(rep.Issues()) == 0 {
			v = nil
		}
		s.render.partial(w, http.StatusUnprocessableEntity, "admin_rulesets", "ruleset_check_result", checkResult{Report: v, Msg: reason(err)})
	default:
		s.fail(w, r, err)
	}
}

// checkResult is what the Check button swaps in: a report, or a one-line reason when there is no report to show.
type checkResult struct {
	Report *reportView
	Msg    string
}

// setOf is a set holding one file of the given kind (a nil content stays "not given": an empty upload is sent as an empty slice).
func setOf(kind core.RulesetKind, content []byte) core.RulesetSet {
	if content == nil {
		content = []byte{}
	}
	switch kind {
	case core.KindVanilla:
		return core.RulesetSet{Vanilla: content}
	case core.KindGlossary:
		return core.RulesetSet{Glossary: content}
	}
	return core.RulesetSet{BattleCompanies: content}
}
