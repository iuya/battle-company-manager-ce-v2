package lean

import (
	"strconv"
	"strings"
)

// Severity of an Issue: an Error blocks the upload, a Warn is shown with the report.
type Severity int

const (
	Error Severity = iota
	Warn
)

func (s Severity) String() string {
	if s == Warn {
		return "warn"
	}
	return "error"
}

// MaxIssues is the safety cap of a Report (design plan 3.2): the rest is only counted.
const MaxIssues = 1000

// Issue is one problem of an uploaded file (design section 7).
type Issue struct {
	File     string // one of the three uploaded files, as the user knows it
	Path     string // JSON path inside the file: companies[2].start.units[1].unit
	Message  string
	Hint     string // one line telling how to fix it
	Closest  string // the closest known name, for a name or word that does not resolve
	Severity Severity
}

// String is the section 7 line: `<file>: <path>: <message>` (a warning starts with `warn: `), the closest match when the message
// does not name it already, and the hint on a second line.
func (i Issue) String() string {
	var b strings.Builder
	if i.Severity == Warn {
		b.WriteString("warn: ")
	}
	if i.File != "" {
		b.WriteString(i.File)
		b.WriteString(": ")
	}
	if i.Path != "" {
		b.WriteString(i.Path)
		b.WriteString(": ")
	}
	b.WriteString(i.Message)
	if i.Closest != "" && !strings.Contains(i.Message, strconv.Quote(i.Closest)) {
		b.WriteString("; closest: ")
		b.WriteString(strconv.Quote(i.Closest))
	}
	if i.Hint != "" {
		b.WriteString("\n  hint: ")
		b.WriteString(i.Hint)
	}
	return b.String()
}

// Report is the ordered list of issues of one validation. The order is the order of discovery, which is deterministic: the
// document is walked in the order of the model's fields and sorted keys, never by map iteration.
type Report struct {
	Issues    []Issue
	Truncated int // issues beyond MaxIssues, counted but not kept
}

// Add appends an issue, or counts it once the cap is reached.
func (r *Report) Add(i Issue) {
	if len(r.Issues) >= MaxIssues {
		r.Truncated++
		return
	}
	r.Issues = append(r.Issues, i)
}

// Errorf adds an error at path of file.
func (r *Report) Errorf(file, path, msg string) { r.Add(Issue{File: file, Path: path, Message: msg}) }

// Warnf adds a warning at path of file.
func (r *Report) Warnf(file, path, msg string) {
	r.Add(Issue{File: file, Path: path, Message: msg, Severity: Warn})
}

// Merge appends the issues of other.
func (r *Report) Merge(other Report) {
	for _, i := range other.Issues {
		r.Add(i)
	}
	r.Truncated += other.Truncated
}

// Errors returns the issues that block the upload.
func (r Report) Errors() []Issue { return r.of(Error) }

// Warnings returns the issues that do not block.
func (r Report) Warnings() []Issue { return r.of(Warn) }

func (r Report) of(s Severity) []Issue {
	var out []Issue
	for _, i := range r.Issues {
		if i.Severity == s {
			out = append(out, i)
		}
	}
	return out
}

// OK reports that nothing blocks the upload. A truncated report is never OK: the issues that were not kept may be errors.
func (r Report) OK() bool { return len(r.Errors()) == 0 && r.Truncated == 0 }

// String is every issue on its own line(s), then "and N more" when the report was truncated.
func (r Report) String() string {
	var b strings.Builder
	for _, i := range r.Issues {
		b.WriteString(i.String())
		b.WriteByte('\n')
	}
	if r.Truncated > 0 {
		b.WriteString("and " + strconv.Itoa(r.Truncated) + " more\n")
	}
	return b.String()
}
