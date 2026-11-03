// Package schemacheck validates decoded JSON documents against the JSON Schema files of definitions/ (Draft 2020-12) with
// github.com/santhosh-tekuri/jsonschema/v6, the library the app's own tests already use.
package schemacheck

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/jsonx"
)

// Compile reads and compiles a schema file; the error says why it is not a valid schema.
func Compile(path string) (*jsonschema.Schema, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	name := filepath.Base(path)
	if err := c.AddResource(name, doc); err != nil {
		return nil, err
	}
	return c.Compile(name)
}

var printer = message.NewPrinter(language.English)

// Errors lists the failures of doc against schema as "path/to/value: message" (the message cut at 200 characters).
func Errors(doc any, schema *jsonschema.Schema) []string {
	err := schema.Validate(doc)
	if err == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return []string{err.Error()}
	}
	var out []string
	var walk func(*jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			out = append(out, strings.Join(e.InstanceLocation, "/")+": "+jsonx.Slice(e.ErrorKind.LocalizedString(printer), 200))
			return
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(ve)
	return out
}
