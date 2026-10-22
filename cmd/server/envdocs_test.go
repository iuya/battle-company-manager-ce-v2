package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// envNames collects the env tag names of a struct, nested structs included.
func envNames(t reflect.Type, out map[string]bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Type.Kind() == reflect.Struct && f.Tag.Get("env") == "" {
			envNames(f.Type, out)
			continue
		}
		if tag := f.Tag.Get("env"); tag != "" {
			out[strings.Split(tag, ",")[0]] = true
		}
	}
}

// TestEveryEnvVarIsDocumented fails when the server gains an environment variable that the compose files and .env.example do not list
// (as an active or a commented line). Run it with make check-env-docs.
func TestEveryEnvVarIsDocumented(t *testing.T) {
	names := map[string]bool{}
	envNames(reflect.TypeOf(config{}), names)
	if len(names) < 10 {
		t.Fatalf("found only %d variables in the config structs: %v", len(names), names)
	}
	for _, file := range []string{"../../compose.yaml", "../../compose.dev.yaml", "../../.env.example"} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(b)
		for name := range names {
			if !strings.Contains(text, name) {
				t.Errorf("%s does not mention %s: add it (a commented line with its default and a short explanation)", file, name)
			}
		}
	}
}
