package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
)

// VanillaAmbiguity is a reviewed duplicate of the vanilla file, as the Battle Companies file lists it in
// data.vanillaAmbiguities: the vanilla file defines Name (a unit, a piece of gear or a keyword) more than once.
type VanillaAmbiguity struct {
	Kind       string // unit, gear or keyword
	Name       string
	Status     string // equivalent, overridden, firstWins or pinned
	Occurrence int    // pinned: the 1-based occurrence that wins
}

// duplicates decides which occurrence of a repeated vanilla name is the one the book keeps.
type duplicates struct {
	reviewed map[string]VanillaAmbiguity
	seen     map[string]*dupState
	order    []string
	log      *slog.Logger
	// pinsOnly: the Battle Companies file is a lean one, whose only statement about a duplicate is a pin (first wins otherwise), so a
	// duplicate that differs and has no pin is not reported.
	pinsOnly bool
}

type dupState struct {
	kind, name string
	count      int
	first      string // canonical form of the first occurrence
	differs    bool
}

func newDuplicates(reviewed []VanillaAmbiguity, log *slog.Logger) *duplicates {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	d := &duplicates{reviewed: map[string]VanillaAmbiguity{}, seen: map[string]*dupState{}, log: log}
	for _, a := range reviewed {
		d.reviewed[dupKey(a.Kind, a.Name)] = a
	}
	return d
}

func dupKey(kind, name string) string { return kind + "\x00" + NameKey(name) }

// wins counts one more occurrence of the name and says whether the book should (re)store it: the first occurrence is always
// stored, a later one only when a pinned entry names it. occurrence is whatever identifies the content (compared for equality).
func (d *duplicates) wins(kind, name string, occurrence any) bool {
	key := dupKey(kind, name)
	st := d.seen[key]
	if st == nil {
		st = &dupState{kind: kind, name: name}
		d.seen[key] = st
		d.order = append(d.order, key)
	}
	st.count++
	sig := canonical(occurrence)
	if st.count == 1 {
		st.first = sig
		return true
	}
	if sig != st.first {
		st.differs = true
	}
	a, ok := d.reviewed[key]
	return ok && a.Status == "pinned" && a.Occurrence == st.count
}

// canonical is a comparable form of an occurrence (map keys are sorted by encoding/json).
func canonical(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// report logs what the loader could not settle by itself: unreviewed duplicates that differ, and pins that point nowhere.
func (d *duplicates) report() {
	for _, key := range slices.Clone(d.order) {
		st := d.seen[key]
		a, reviewed := d.reviewed[key]
		switch {
		case reviewed && a.Status == "pinned" && (a.Occurrence < 1 || a.Occurrence > st.count):
			d.log.Warn("vanilla duplicate pinned to an occurrence that does not exist; the first one is used",
				"kind", st.kind, "name", st.name, "occurrence", a.Occurrence, "occurrences", st.count)
		case st.count > 1 && st.differs && !reviewed:
			level := slog.LevelWarn
			if d.pinsOnly { // a lean file reviews duplicates by pinning only: the others keep the first occurrence, noted for the audit
				level = slog.LevelDebug
			}
			d.log.Log(context.Background(), level, "vanilla defines a name more than once with different content and nobody reviewed it (vanilla.pins); the first occurrence is used",
				"kind", st.kind, "name", st.name, "occurrences", st.count)
		}
	}
}
