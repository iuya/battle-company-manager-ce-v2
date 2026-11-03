// Package repo finds the repository root for the tools, which run from any folder inside it.
package repo

import (
	"os"
	"path/filepath"
)

// Root is the nearest folder, from the working directory up, that holds go.mod. It falls back to the working directory.
func Root() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	for d := wd; ; {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return wd
		}
		d = parent
	}
}
