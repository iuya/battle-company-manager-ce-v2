package core

import (
	"errors"
	"fmt"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// RulesetIssue is one problem of an uploaded rules file, as plain data for the web layer.
type RulesetIssue struct {
	File    string // one of the three uploaded files
	Path    string // JSON path inside the file
	Message string
	Hint    string
	Closest string // the closest known name, when a name did not resolve
	Warn    bool   // a warning does not block the upload
}

// String is the section 7 line: file, path and message, the closest match, the hint on a second line.
func (i RulesetIssue) String() string {
	sev := lean.Error
	if i.Warn {
		sev = lean.Warn
	}
	return lean.Issue{File: i.File, Path: i.Path, Message: i.Message, Hint: i.Hint, Closest: i.Closest, Severity: sev}.String()
}

// RulesetReport is the error of a rejected upload or load: every problem of the file, not only the first. It wraps
// ErrInvalidRuleset, so callers that only test errors.Is(err, ErrInvalidRuleset) keep working; the web layer reads Issues.
type RulesetReport struct {
	issues    []RulesetIssue
	truncated int
	summary   string // the one-line text of Error when it is not built from the issues (schema failures keep their wording)
}

// NewRulesetReport converts the report of the lean decoder.
func newRulesetReport(rep lean.Report) *RulesetReport {
	r := &RulesetReport{truncated: rep.Truncated}
	for _, i := range rep.Issues {
		r.issues = append(r.issues, RulesetIssue{File: i.File, Path: i.Path, Message: i.Message, Hint: i.Hint, Closest: i.Closest, Warn: i.Severity == lean.Warn})
	}
	return r
}

// NewRulesetReport builds a report from plain issues (truncated counts the ones beyond the cap that are not listed), for a
// consumer test that needs a report without running a decoder.
func NewRulesetReport(issues []RulesetIssue, truncated int) *RulesetReport {
	return &RulesetReport{issues: issues, truncated: truncated}
}

// Issues lists the problems in report order, warnings included.
func (r RulesetReport) Issues() []RulesetIssue { return r.issues }

// Truncated counts the problems beyond the cap that are not listed.
func (r RulesetReport) Truncated() int { return r.truncated }

// Errors counts the issues that block the upload.
func (r RulesetReport) Errors() int {
	n := 0
	for _, i := range r.issues {
		if !i.Warn {
			n++
		}
	}
	return n
}

// Warnings returns the non-blocking issues.
func (r RulesetReport) Warnings() []RulesetIssue {
	var out []RulesetIssue
	for _, i := range r.issues {
		if i.Warn {
			out = append(out, i)
		}
	}
	return out
}

// Error is a one-line summary followed by the first issues; the full list is in Issues.
func (r RulesetReport) Error() string {
	if r.summary != "" {
		return r.summary
	}
	var b strings.Builder
	n := r.Errors()
	fmt.Fprintf(&b, "the rules file has %d %s", n, Plural(n, "problem", "problems"))
	if r.truncated > 0 {
		fmt.Fprintf(&b, " (and %d more not listed)", r.truncated)
	}
	shown := 0
	for _, i := range r.issues {
		if i.Warn {
			continue
		}
		if shown == 3 {
			b.WriteString("; ...")
			break
		}
		b.WriteString("; ")
		b.WriteString(strings.SplitN(i.String(), "\n", 2)[0])
		shown++
	}
	return b.String()
}

// Unwrap makes errors.Is(err, ErrInvalidRuleset) true.
func (r RulesetReport) Unwrap() error { return ErrInvalidRuleset }

// JoinRulesetReports merges the errors of several files into one report (the campaign form checks the three files together);
// it returns nil when every error is nil, the plain join when one of them is not a report.
func JoinRulesetReports(errs ...error) error {
	merged := &RulesetReport{}
	var other []error
	count := 0
	for _, err := range errs {
		if err == nil {
			continue
		}
		count++
		var rep *RulesetReport
		if errors.As(err, &rep) {
			merged.issues = append(merged.issues, rep.issues...)
			merged.truncated += rep.truncated
			continue
		}
		other = append(other, err)
	}
	switch {
	case count == 0:
		return nil
	case len(other) > 0:
		return errors.Join(errs...)
	case count == 1:
		for _, err := range errs {
			if err != nil {
				return err
			}
		}
	}
	return merged
}
