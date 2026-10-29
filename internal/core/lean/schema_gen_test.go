package lean

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// The JSON Schema of the lean format (definitions/battle_companies/battle_companies.schema.json) is GENERATED from the Go
// model by this file, never written by hand: model.go and campaign.go are the contract, and a hand-kept copy would drift. The
// committed file exists for the Python validator, editors and the curator; schema_test.go fails when it differs from what this
// generator makes from the model, and checks the model against the file again with a second, independent reading.
//
//	go test ./internal/core/lean -run TestSchemaUpToDate -update-schema    rewrites the file
var updateSchema = flag.Bool("update-schema", false, "rewrite definitions/battle_companies/battle_companies.schema.json from the Go model")

const schemaPath = "../../../definitions/battle_companies/battle_companies.schema.json"

// schemaDefaults are the defaults the Normalise pass applies from a constant. The key is "Type.jsonName".
// TestSchemaDefaults runs Normalise and checks each one, so the table cannot drift from defaults.go either.
var schemaDefaults = map[string]any{
	"Path.kind":        defaultPathKind,
	"Scenario.side":    defaultScenarioSide,
	"Scoring.side":     defaultScenarioSide,
	"Target.appliesTo": defaultTargetApply,
	"Target.match":     defaultTargetMatch,
	"OwnPath.path":     defaultOwnPath,
	"StartUnit.count":  1,
	"ChartRow.count":   1,
}

// polymorphic are the types that read several forms in BindJSON; their schema is written here, once, next to the form list in
// types.go. The rest of the schema is derived from the struct tags.
var polymorphic = map[reflect.Type]map[string]any{
	reflect.TypeOf(Roll{}): {
		"description": "A chart roll: a whole number, a range \"lo-hi\" or a pair [lo, hi].",
		"oneOf": []any{
			map[string]any{"type": "integer", "minimum": 0, "maximum": maxRollValue},
			map[string]any{"type": "string", "pattern": `^\s*([0-9]{1,3}|1000)\s*(-\s*([0-9]{1,3}|1000)\s*)?$`},
			map[string]any{"type": "array", "minItems": 2, "maxItems": 2, "items": map[string]any{"type": "integer", "minimum": 0, "maximum": maxRollValue}},
		},
	},
	reflect.TypeOf(DiceSpec{}): {
		"description": "A dice specification [N]dM, N 1.." + strconv.Itoa(MaxDiceCount) + ", M " + strconv.Itoa(MinDiceSides) + ".." + strconv.Itoa(MaxDiceSides) + "; d66 is refused.",
		"type":        "string",
		"pattern":     `^([1-9]|10)?[dD]([2-9]|[1-9][0-9]|100)$`,
		"not":         map[string]any{"pattern": `[dD]66$`},
	},
	reflect.TypeOf(Rating{}): {
		"description": "A rating: one number for warrior and hero alike, or {warrior, hero}.",
		"oneOf": []any{
			map[string]any{"type": "integer", "minimum": 0, "maximum": 1000},
			map[string]any{
				"type": "object", "additionalProperties": false, "required": []any{"warrior", "hero"},
				"properties": map[string]any{
					"warrior": map[string]any{"type": "integer", "minimum": 0, "maximum": 1000},
					"hero":    map[string]any{"type": "integer", "minimum": 0, "maximum": 1000},
				},
			},
		},
	},
	reflect.TypeOf(RuleRef{}): {
		"description": "A rule name, or a rule definition object that the loader hoists.",
		"oneOf": []any{
			map[string]any{"type": "string", "pattern": `\S`},
			map[string]any{"$ref": "#/$defs/Rule"},
		},
	},
	reflect.TypeOf(AdvanceTo{}): {
		"description": "A promotion target: a unit name, or {unit, with} when the promotion also gives wargear.",
		"oneOf": []any{
			map[string]any{"type": "string", "pattern": `\S`},
			map[string]any{
				"type": "object", "additionalProperties": false, "required": []any{"unit"},
				"properties": map[string]any{
					"unit": map[string]any{"type": "string", "minLength": 1},
					"with": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
			},
		},
	},
	reflect.TypeOf(Scalar("")): {
		"description": "A string or a number read as text (the rulebook version).",
		"oneOf": []any{
			map[string]any{"type": "string", "minLength": 1},
			map[string]any{"type": "number"},
		},
	},
}

// fieldDoc is the description of the key name of the struct typeName: its own, else the one every struct shares.
func fieldDoc(typeName, name string) string {
	if d, ok := fieldDocs[typeName+"."+name]; ok {
		return d
	}
	return commonDocs[name]
}

// polyName is the $defs key of a polymorphic type.
func polyName(t reflect.Type) string { return t.Name() }

// tagNum reads a numeric tag as a JSON number.
func tagNum(sf reflect.StructField, key string) (any, bool) {
	s, ok := sf.Tag.Lookup(key)
	if !ok {
		return nil, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		panic("bad " + key + " tag on " + sf.Name)
	}
	if f == float64(int64(f)) {
		return int64(f), true
	}
	return f, true
}

// typeSchema is the schema of a value of type t carrying the tags of sf.
//
// A pointer to a scalar (*string, *bool) is the model's "explicitly none" (Path.heroicAction: null, Who.hero): the schema allows
// null there and nowhere else. The binder also ignores null on any other optional key; the schema is stricter on purpose, a null
// that means nothing is a mistake the curator should see.
func typeSchema(t reflect.Type, sf reflect.StructField, defs map[reflect.Type]bool) map[string]any {
	nullable := false
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
		nullable = t.Kind() == reflect.String || t.Kind() == reflect.Bool
	}
	if nullable {
		out := typeSchema(t, sf, defs)
		out["type"] = []any{out["type"], "null"}
		if e, ok := out["enum"].([]any); ok {
			out["enum"] = append(e, nil)
		}
		return out
	}
	if _, ok := polymorphic[t]; ok {
		return map[string]any{"$ref": "#/$defs/" + polyName(t)}
	}
	out := map[string]any{}
	set := func(key, tag string) {
		if v, ok := tagNum(sf, tag); ok {
			out[key] = v
		}
	}
	switch t.Kind() {
	case reflect.String:
		out["type"] = "string"
		set("minLength", "min")
		set("maxLength", "max")
		if e := sf.Tag.Get("enum"); e != "" {
			out["enum"] = anyStrings(strings.Split(e, "|"))
		}
	case reflect.Bool:
		out["type"] = "boolean"
	case reflect.Int, reflect.Int64:
		out["type"] = "integer"
		set("minimum", "min")
		set("maximum", "max")
	case reflect.Float64:
		out["type"] = "number"
		set("minimum", "min")
		set("maximum", "max")
	case reflect.Interface:
		return map[string]any{} // Free: anything, typed by a later slice
	case reflect.Slice:
		out["type"] = "array"
		set("minItems", "min")
		set("maxItems", "max")
		elem := reflect.StructField{Tag: reflect.StructTag("")}
		var tags []string
		for _, k := range []string{"enum", "emin", "emax"} {
			if v, ok := sf.Tag.Lookup(k); ok {
				key := k
				if k == "emin" {
					key = "min"
				} else if k == "emax" {
					key = "max"
				}
				tags = append(tags, key+":"+strconv.Quote(v))
			}
		}
		elem.Tag = reflect.StructTag(strings.Join(tags, " "))
		out["items"] = typeSchema(t.Elem(), elem, defs)
	case reflect.Map:
		out["type"] = "object"
		out["additionalProperties"] = typeSchema(t.Elem(), reflect.StructField{}, defs)
	case reflect.Struct:
		defs[t] = true
		return map[string]any{"$ref": "#/$defs/" + t.Name()}
	default:
		panic("lean schema: unsupported kind " + t.Kind().String())
	}
	return out
}

func anyStrings(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// structSchema is the object schema of a model struct: closed unless it has a rest map.
func structSchema(t reflect.Type, defs map[reflect.Type]bool) map[string]any {
	props := map[string]any{}
	var required []any
	open := false
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.Tag.Get("rest") == "true" {
			open = true
			continue
		}
		name, _, _ := strings.Cut(sf.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		p := typeSchema(sf.Type, sf, defs)
		if d, ok := schemaDefaults[t.Name()+"."+name]; ok {
			p = withDefault(p, d)
		}
		if t == reflect.TypeOf(Document{}) && name == "schemaVersion" {
			p["const"] = SupportedSchemaVersion
		}
		if d := fieldDoc(t.Name(), name); d != "" {
			p["description"] = d
		}
		props[name] = p
		if sf.Tag.Get("required") == "true" {
			required = append(required, name)
		}
	}
	out := map[string]any{"type": "object", "properties": props, "additionalProperties": open}
	if d, ok := typeDocs[t.Name()]; ok {
		out["description"] = d
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

// withDefault puts a default on a property; a $ref cannot carry siblings in older drafts, so it is wrapped in allOf.
func withDefault(p map[string]any, d any) map[string]any {
	p["default"] = d
	return p
}

// GenerateSchema is the schema of the lean format made from the model.
func generateSchema() map[string]any {
	defs := map[reflect.Type]bool{}
	root := structSchema(reflect.TypeOf(Document{}), defs)
	root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	root["$id"] = "battle_companies.schema.json"
	root["title"] = "Battle Companies rules file, lean format (schemaVersion 2)"
	root["description"] = typeDocs["Document"] + " Generated from internal/core/lean/model.go and campaign.go by `go test ./internal/core/lean -run TestSchemaUpToDate -update-schema`; never edit by hand. The Go decoder (lean.Decode) is the authority: it also checks names, charts and cross references, which a schema cannot."
	all := map[string]any{}
	// close the set of struct types reachable from the document
	for done := map[reflect.Type]bool{}; ; {
		progressed := false
		for t := range defs {
			if done[t] {
				continue
			}
			done[t] = true
			progressed = true
			all[t.Name()] = structSchema(t, defs)
		}
		if !progressed {
			break
		}
	}
	for t, s := range polymorphic {
		all[polyName(t)] = s
	}
	root["$defs"] = all
	return root
}

func schemaBytes() []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(generateSchema()); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// TestSchemaUpToDate: the committed file is exactly what the model generates (or rewrites it with -update-schema).
func TestSchemaUpToDate(t *testing.T) {
	want := schemaBytes()
	if *updateSchema {
		if err := os.WriteFile(filepath.FromSlash(schemaPath), want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(filepath.FromSlash(schemaPath))
	if err != nil {
		t.Fatalf("%v (run: go test ./internal/core/lean -run TestSchemaUpToDate -update-schema)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("battle_companies.schema.json is stale or hand-edited: the Go model is the contract.\nRun: go test ./internal/core/lean -run TestSchemaUpToDate -update-schema")
	}
}
