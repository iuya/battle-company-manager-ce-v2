package core

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

// A practice match has a complete, empty aftermath that is derived, never stored: the page reads it, every write is refused, and the
// company is never touched.
func TestPracticeAftermathIsDoneAndEmpty(t *testing.T) {
	f := newAmFixture(t)
	ended := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	f.match.Practice, f.match.EndedOn = true, &ended
	before, rec := f.state.Clone(), f.rec

	a, err := f.svc.Aftermath(f.ctx, f.owner(), 5, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Practice || a.Status != AftermathDone || a.Status.Locks() || a.Revision != 1 || len(a.Input.Models) != 0 || len(a.Casualties) != 0 {
		t.Fatalf("aftermath = %+v", a)
	}
	if len(a.Lines) != 1 || a.Lines[0].Text != PracticeNothingToRoll || !a.Lines[0].On.Equal(ended) || a.Lines[0].Match != 3 {
		t.Errorf("lines = %+v", a.Lines)
	}
	writes := map[string]func() error{
		"save": func() error {
			_, err := f.svc.SaveAftermath(f.ctx, f.owner(), 5, 1, 1, 0, AftermathInput{})
			return err
		},
		"start": func() error {
			_, err := f.svc.StartAftermath(f.ctx, f.owner(), 5, 1, 1, 0, AftermathInput{})
			return err
		},
		"roll": func() error {
			_, err := f.svc.RollCasualty(f.ctx, f.owner(), 5, 1, 1, 6, []int{3, 4}, false)
			return err
		},
		"reroll": func() error {
			_, err := f.svc.RerollCasualty(f.ctx, f.owner(), 5, 1, 1, 6, []int{3, 4}, false)
			return err
		},
		"decide": func() error { _, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{}); return err },
		"finish": func() error { _, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, 0); return err },
		"reopen": func() error { _, err := f.svc.ReopenSheet(f.ctx, f.owner(), 5, 1, 1, 0); return err },
		"waive": func() error {
			_, err := f.svc.WaiveAftermath(f.ctx, User{ID: amOrganiser}, 5, 1, 1, "nothing")
			return err
		},
	}
	for name, run := range writes {
		if err := run(); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), PracticeNothingToRoll) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if !reflect.DeepEqual(before, f.state) || !reflect.DeepEqual(rec, f.rec) {
		t.Error("a practice aftermath must not touch the company or the record")
	}
}

func TestPracticeAftermathOfAnOpenMatchStillSaysEndFirst(t *testing.T) {
	f := newAmFixture(t)
	f.match.Practice, f.match.Ended = true, false
	if _, err := f.svc.Aftermath(f.ctx, f.owner(), 5, 1, 1); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "End the match first") {
		t.Errorf("open practice match: %v", err)
	}
}

func TestCreateMatchPassesThePracticeFlag(t *testing.T) {
	f := newMatchesFixture(t)
	want := NewMatch{Sides: []NewSide{{Name: "Side 1", Companies: []int64{1}}, {Name: "Side 2", Companies: []int64{2}}}, Practice: true}
	f.matches.EXPECT().Create(gomock.Any(), int64(5), f.player(), want).Return(Match{ID: 1, Practice: true}, nil)
	m, err := f.svc.CreateMatch(f.ctx, f.player(), 5, NewMatch{Sides: []NewSide{{Companies: []int64{1}}, {Companies: []int64{2}}}, Practice: true})
	if err != nil || !m.Practice {
		t.Fatalf("CreateMatch = %+v, %v", m, err)
	}
}

// A same-player fight (two companies of one owner) as a practice match: the owner reads both aftermaths, both are done and empty.
func TestPracticeMatchOfTwoCompaniesOfOneOwner(t *testing.T) {
	f := newAmFixture(t)
	f.match.Practice = true
	now := time.Now()
	f.match.EndedOn = &now
	f.match.Sides[1].Companies[0].OwnerID = amOwner // Raiders belong to the same player
	for _, id := range []int64{1, 2} {
		co := f.match.Sides[id-1].Companies[0]
		if AftermathAccessFor(f.owner(), f.match, co, amOrganiser) != AftermathEdit {
			t.Fatalf("company %d: the owner reads and would edit it", id)
		}
	}
	a, err := f.svc.Aftermath(f.ctx, f.owner(), 5, 1, 1)
	if err != nil || !a.Practice || a.Status.Locks() {
		t.Fatalf("aftermath = %+v, %v", a, err)
	}
}

func TestReopenOfAPracticeMatchIgnoresTheAftermathStartedRule(t *testing.T) {
	f := newMatchesFixture(t)
	admin := User{ID: 5, Role: RoleAdmin}
	f.match.Ended, f.match.Practice = true, true
	f.svc.AftermathStarted = func(context.Context, int64, int64) (bool, error) { return true, nil }
	m, err := f.svc.ReopenMatch(f.ctx, admin, 5, 1, 3)
	if err != nil || m.Ended || !m.Practice {
		t.Fatalf("ReopenMatch = %+v, %v", m, err)
	}
}

func TestAgainstTheOddsRewardBonusesAreDisabledInAPracticeMatch(t *testing.T) {
	f := newATOFixture(t)
	f.match.Practice = true
	for _, o := range f.card(t, 10).Options {
		reward := o.Key == "influence" || o.Key == "experience" || o.Key == "reroll"
		if reward != (o.Disabled && o.Reason == ATOPracticeReason) {
			t.Errorf("%s: disabled %v, reason %q", o.Key, o.Disabled, o.Reason)
		}
		if !reward && o.Reason == ATOPracticeReason {
			t.Errorf("%s is a table effect", o.Key)
		}
	}
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 3, ATOSelection{Bonuses: []string{"reroll"}}); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), ATOPracticeReason) {
		t.Errorf("a reward bonus cannot be newly taken: %v", err)
	}
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 3, ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer"}}); err != nil {
		t.Errorf("a kit is a table effect: %v", err)
	}
	// a selection made before stays as entered, with the reason shown
	f.rec = ATORecord{MatchID: 1, CompanyID: 1, Selection: ATOSelection{Bonuses: []string{"influence"}}}
	for _, o := range f.card(t, 10).Options {
		if o.Key == "influence" && (!o.Taken || o.Disabled || o.Reason != ATOPracticeReason) {
			t.Errorf("influence: %+v", o)
		}
	}
	// a real match is unchanged
	f.match.Practice = false
	f.rec = ATORecord{}
	for _, o := range f.card(t, 10).Options {
		if o.Reason == ATOPracticeReason {
			t.Errorf("%s: no practice reason in a real match", o.Key)
		}
	}
}
