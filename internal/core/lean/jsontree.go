package lean

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// MaxDepth is the deepest nesting of arrays and objects the decoder reads (safety limit, design plan section 6).
const MaxDepth = 64

// Reporter appends issues of one file to a Report.
type Reporter struct {
	File   string
	Report *Report
}

// NewReporter returns a Reporter for file.
func NewReporter(file string, r *Report) *Reporter { return &Reporter{File: file, Report: r} }

// Errorf adds an error at path.
func (r *Reporter) Errorf(path, format string, a ...any) {
	r.Report.Add(Issue{File: r.File, Path: path, Message: fmt.Sprintf(format, a...)})
}

// Warnf adds a warning at path.
func (r *Reporter) Warnf(path, format string, a ...any) {
	r.Report.Add(Issue{File: r.File, Path: path, Message: fmt.Sprintf(format, a...), Severity: Warn})
}

// Add adds an issue, stamping the file when the issue has none.
func (r *Reporter) Add(i Issue) {
	if i.File == "" {
		i.File = r.File
	}
	r.Report.Add(i)
}

// Join appends a key to a JSON path: `a.b`, and `a["odd key"]` for a key that is not a plain word.
func Join(path, key string) string {
	if key == "" || !plainKey(key) {
		return path + "[" + strconv.Quote(key) + "]"
	}
	if path == "" {
		return key
	}
	return path + "." + key
}

// Idx appends a list index to a JSON path.
func Idx(path string, i int) string { return path + "[" + strconv.Itoa(i) + "]" }

func plainKey(k string) bool {
	for _, c := range k {
		if !(c == '_' || c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

// parseJSON reads raw into the generic tree (objects are map[string]any, numbers json.Number) with three checks the standard
// library does not make: a key written twice is an error at its path, nesting beyond MaxDepth stops the read, and a syntax error
// is reported with its line and column. ok is false when the tree is unusable (syntax or depth); duplicate keys keep the last.
func parseJSON(raw []byte, rep *Reporter) (v any, ok bool) {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	fail := func(err error) (any, bool) {
		off := int64(len(raw))
		var se *json.SyntaxError
		if errors.As(err, &se) {
			off = se.Offset
		} else if off2 := dec.InputOffset(); off2 > 0 && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			off = off2
		}
		line, col := lineCol(raw, off)
		msg := err.Error()
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(msg, "unexpected end of JSON input") {
			msg = "unexpected end of the file"
		}
		rep.Add(Issue{File: rep.File, Message: fmt.Sprintf("not valid JSON at line %d, column %d: %s", line, col, msg), Hint: "check commas, quotes and brackets near that position"})
		return nil, false
	}
	tok, err := dec.Token()
	if err != nil {
		return fail(err)
	}
	var depthErr bool
	var build func(tok json.Token, at string, depth int) (any, error)
	build = func(tok json.Token, at string, depth int) (any, error) {
		d, isDelim := tok.(json.Delim)
		if !isDelim {
			return tok, nil
		}
		if depth >= MaxDepth {
			depthErr = true
			return nil, fmt.Errorf("nested more than %d levels", MaxDepth)
		}
		switch d {
		case '{':
			m := map[string]any{}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, _ := kt.(string)
				vt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				child := Join(at, key)
				val, err := build(vt, child, depth+1)
				if err != nil {
					return nil, err
				}
				if _, dup := m[key]; dup {
					rep.Errorf(child, "key %q is written twice; the last one would win silently", key)
				}
				m[key] = val
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return m, nil
		case '[':
			list := []any{}
			for dec.More() {
				vt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				val, err := build(vt, Idx(at, len(list)), depth+1)
				if err != nil {
					return nil, err
				}
				list = append(list, val)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return list, nil
		}
		return nil, fmt.Errorf("unexpected %q", d)
	}
	v, err = build(tok, "", 0)
	if err != nil {
		if depthErr {
			rep.Add(Issue{File: rep.File, Message: fmt.Sprintf("the file is nested more than %d levels deep", MaxDepth)})
			return nil, false
		}
		return fail(err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("unexpected data after the top-level value")
		}
		return fail(err)
	}
	return v, true
}

func lineCol(raw []byte, off int64) (line, col int) {
	if off > int64(len(raw)) {
		off = int64(len(raw))
	}
	line, col = 1, 1
	for _, b := range raw[:off] {
		if b == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return
}
