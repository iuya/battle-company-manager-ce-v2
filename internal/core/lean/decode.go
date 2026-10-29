package lean

import (
	"encoding/json"
	"fmt"
)

// Decode is the one entry point: it reads a Battle Companies file of schemaVersion 2, binds it strictly, applies
// extends and defaults, resolves every name against the file and lower, and validates. Every problem lands in the report; the
// document is nil only when the bytes are not usable JSON or are another schemaVersion. The same bytes always give the same
// report. A panic inside the decoder (a bug, never expected) is reported as an issue instead of taking the caller down.
func Decode(file string, raw []byte, lower Lower) (doc *Document, ix *Index, rep Report) {
	defer func() {
		if p := recover(); p != nil {
			rep.Add(Issue{File: file, Message: fmt.Sprintf("the file could not be checked (internal error: %v)", p), Hint: "this is a bug in the checker; please report the file"})
		}
	}()
	return decode(file, raw, lower)
}

// decode is Decode without the recover, for the fuzz test.
func decode(file string, raw []byte, lower Lower) (*Document, *Index, Report) {
	var rep Report
	r := NewReporter(file, &rep)
	tree, ok := parseJSON(raw, r)
	if !ok {
		return nil, nil, rep
	}
	if issue, ok := versionOf(file, tree); !ok {
		rep.Add(issue)
		return nil, nil, rep
	}
	doc := &Document{}
	bindTree(doc, tree, r)
	Normalise(doc, r)
	ix := Resolve(doc, lower, r)
	Validate(doc, ix, lower, r)
	return doc, ix, rep
}

// versionOf checks schemaVersion before anything else, so a file of the old format gets one clear message instead of hundreds.
func versionOf(file string, tree any) (Issue, bool) {
	obj, ok := tree.(map[string]any)
	if !ok {
		return Issue{File: file, Message: fmt.Sprintf("expected an object, got %s", describe(tree))}, false
	}
	v, present := obj["schemaVersion"]
	if !present {
		return Issue{File: file, Path: "schemaVersion", Message: fmt.Sprintf("missing (this app reads %d); an older file must be regenerated or converted", SupportedSchemaVersion),
			Hint: "the Battle Companies file must say \"schemaVersion\": 2"}, false
	}
	n, isNum := v.(json.Number)
	i, err := n.Int64()
	if !isNum || err != nil {
		return Issue{File: file, Path: "schemaVersion", Message: fmt.Sprintf("expected the whole number %d, got %s", SupportedSchemaVersion, describe(v))}, false
	}
	if int(i) != SupportedSchemaVersion {
		return VersionIssue(file, int(i)), false
	}
	return Issue{}, true
}
