package core

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func endFixture(t *testing.T) *matchesFixture {
	t.Helper()
	f := newMatchesFixture(t)
	f.match.Scenario = &MatchScenario{Kind: ScenarioCustom, Name: "x"}
	f.match.Result = MatchResult{Decided: true, Winner: 1, Sides: make([]SideResult, 2)}
	return f
}

func playerOf(id int64, name string) User { return User{ID: id, Role: RoleUser, DisplayName: name} }

func TestProposingToEndWaitsForTheOtherOwners(t *testing.T) {
	f := endFixture(t)
	m, err := f.svc.ProposeEnd(f.ctx, playerOf(10, "Hador"), 5, 1, 3)
	if err != nil || m.Ended || m.EndProposal == nil {
		t.Fatalf("%+v %v", m, err)
	}
	p := m.EndProposal
	if len(p.Companies) != 3 || !p.Companies[0].Confirmed || p.Companies[0].By != "Hador" || len(p.Waiting()) != 2 {
		t.Errorf("the proposer's company is confirmed, the others wait: %+v", p.Companies)
	}
	if _, err := f.svc.ProposeEnd(f.ctx, playerOf(10, "Hador"), 5, 1, m.Revision); !errors.Is(err, ErrRefused) {
		t.Errorf("proposed twice: %v", err)
	}
	// 20 owns Raiders only; 30 owns Wolves
	m, err = f.svc.ConfirmEnd(f.ctx, playerOf(20, "Gor"), 5, 1, m.Revision)
	if err != nil || m.Ended || len(m.EndProposal.Waiting()) != 1 {
		t.Fatalf("one of two left: %+v %v", m.EndProposal, err)
	}
	if _, err := f.svc.ConfirmEnd(f.ctx, playerOf(20, "Gor"), 5, 1, m.Revision); !errors.Is(err, ErrRefused) {
		t.Errorf("nothing left to confirm: %v", err)
	}
	m, err = f.svc.ConfirmEnd(f.ctx, playerOf(30, "Wolf"), 5, 1, m.Revision)
	if err != nil || !m.Ended || m.EndProposal != nil || m.EndedOn == nil {
		t.Fatalf("the last confirmation ends the match: %+v %v", m, err)
	}
	if f.match.EndDetail != "confirmed by Rangers, Raiders, Wolves" {
		t.Errorf("history detail %q", f.match.EndDetail)
	}
}

func TestEndAtOnceWhenNobodyWaits(t *testing.T) {
	f := endFixture(t)
	for i := range f.match.Sides {
		for k := range f.match.Sides[i].Companies {
			f.match.Sides[i].Companies[k].OwnerID = 10
		}
	}
	m, err := f.svc.ProposeEnd(f.ctx, playerOf(10, "Hador"), 5, 1, 3)
	if err != nil || !m.Ended || m.EndProposal != nil {
		t.Fatalf("a single owner ends it at once: %+v %v", m, err)
	}
}

func TestProposalRunsCanEndAndNeedsAccess(t *testing.T) {
	f := newMatchesFixture(t)
	if _, err := f.svc.ProposeEnd(f.ctx, playerOf(10, "H"), 5, 1, 3); !errors.Is(err, ErrRefused) {
		t.Errorf("no scenario or result: %v", err)
	}
	f.match.Scenario = &MatchScenario{Kind: ScenarioCustom, Name: "x"}
	f.match.Result = MatchResult{Decided: true, Winner: 1, Sides: make([]SideResult, 2)}
	if _, err := f.svc.ProposeEnd(f.ctx, playerOf(7, "Stranger"), 5, 1, 3); !errors.Is(err, ErrRefused) {
		t.Errorf("a stranger: %v", err)
	}
	if _, err := f.svc.ProposeEnd(f.ctx, playerOf(10, "H"), 5, 1, 2); !errors.Is(err, ErrMatchChanged) {
		t.Errorf("stale revision: %v", err)
	}
	// the organiser owns no company: nothing is confirmed for them, everyone waits
	m, err := f.svc.ProposeEnd(f.ctx, User{ID: matchOrganiser, Role: RoleUser, DisplayName: "Org"}, 5, 1, 3)
	if err != nil || len(m.EndProposal.Waiting()) != 3 {
		t.Fatalf("%+v %v", m.EndProposal, err)
	}
}

func TestWithdrawAndCancel(t *testing.T) {
	f := endFixture(t)
	m, _ := f.svc.ProposeEnd(f.ctx, playerOf(10, "Hador"), 5, 1, 3)
	m, err := f.svc.ConfirmEnd(f.ctx, playerOf(20, "Gor"), 5, 1, m.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.WithdrawEnd(f.ctx, playerOf(30, "Wolf"), 5, 1, m.Revision); !errors.Is(err, ErrRefused) {
		t.Errorf("not confirmed: %v", err)
	}
	m, err = f.svc.WithdrawEnd(f.ctx, playerOf(10, "Hador"), 5, 1, m.Revision)
	if err != nil || m.EndProposal == nil || m.EndProposal.Companies[0].Confirmed || !m.EndProposal.Companies[1].Confirmed {
		t.Fatalf("the proposal stays with the other confirmation: %+v %v", m.EndProposal, err)
	}
	m, err = f.svc.WithdrawEnd(f.ctx, playerOf(20, "Gor"), 5, 1, m.Revision)
	if err != nil || m.EndProposal != nil {
		t.Fatalf("nobody confirms: the proposal goes: %+v %v", m.EndProposal, err)
	}
	m, _ = f.svc.ProposeEnd(f.ctx, playerOf(10, "Hador"), 5, 1, m.Revision)
	if _, err := f.svc.CancelEnd(f.ctx, playerOf(7, "Stranger"), 5, 1, m.Revision); !errors.Is(err, ErrRefused) {
		t.Errorf("a stranger cannot cancel: %v", err)
	}
	m, err = f.svc.CancelEnd(f.ctx, playerOf(30, "Wolf"), 5, 1, m.Revision) // "Keep playing"
	if err != nil || m.EndProposal != nil || m.Ended {
		t.Fatalf("%+v %v", m, err)
	}
	if _, err := f.svc.CancelEnd(f.ctx, playerOf(30, "Wolf"), 5, 1, m.Revision); !errors.Is(err, ErrRefused) {
		t.Errorf("nothing to cancel: %v", err)
	}
}

func TestOverrideEnd(t *testing.T) {
	f := endFixture(t)
	org := User{ID: matchOrganiser, Role: RoleUser, DisplayName: "Org"}
	m, _ := f.svc.ProposeEnd(f.ctx, playerOf(10, "Hador"), 5, 1, 3)
	if _, err := f.svc.OverrideEnd(f.ctx, playerOf(10, "Hador"), 5, 1, m.Revision, "gone"); !errors.Is(err, ErrRefused) {
		t.Errorf("a participant cannot override: %v", err)
	}
	for _, note := range []string{"", "  ", strings.Repeat("x", 201)} {
		if _, err := f.svc.OverrideEnd(f.ctx, org, 5, 1, m.Revision, note); !errors.Is(err, ErrInvalidMatch) {
			t.Errorf("note %q: %v", note, err)
		}
	}
	m, err := f.svc.OverrideEnd(f.ctx, org, 5, 1, m.Revision, "  left the table ")
	if err != nil || !m.Ended || m.EndProposal != nil {
		t.Fatalf("%+v %v", m, err)
	}
	if f.match.EndDetail != "overriding Raiders, Wolves: left the table" {
		t.Errorf("history detail %q", f.match.EndDetail)
	}
	// without a proposal there is nothing to override
	g := endFixture(t)
	if _, err := g.svc.OverrideEnd(g.ctx, org, 5, 1, 3, "x"); !errors.Is(err, ErrRefused) {
		t.Errorf("no proposal: %v", err)
	}
}

func TestAnOrganiserWhoPlaysConfirmsOwnCompanyInsteadOfBeingOverridden(t *testing.T) {
	f := endFixture(t)
	f.match.Sides[1].Companies[1].OwnerID = matchOrganiser // Wolves belong to the organiser
	m, _ := f.svc.ProposeEnd(f.ctx, playerOf(10, "Hador"), 5, 1, 3)
	org := User{ID: matchOrganiser, Role: RoleUser, DisplayName: "Org"}
	if _, err := f.svc.OverrideEnd(f.ctx, org, 5, 1, m.Revision, "Raiders left"); err != nil {
		t.Fatal(err)
	}
	if f.match.EndDetail != "overriding Raiders: Raiders left" {
		t.Errorf("only the other company is overridden: %q", f.match.EndDetail)
	}
}

func TestChangesResetTheConfirmationsExceptTheProposers(t *testing.T) {
	f := endFixture(t)
	m, _ := f.svc.ProposeEnd(f.ctx, playerOf(10, "Hador"), 5, 1, 3)
	m, _ = f.svc.ConfirmEnd(f.ctx, playerOf(20, "Gor"), 5, 1, m.Revision)
	r := f.match.Result
	r.Note = "a word"
	m, err := f.svc.SaveResult(f.ctx, playerOf(10, "Hador"), 5, 1, m.Revision, r)
	if err != nil {
		t.Fatal(err)
	}
	p := m.EndProposal
	if p == nil || !p.Companies[0].Confirmed || p.Companies[1].Confirmed || p.Notice != "Hador changed the result; confirm again" {
		t.Fatalf("%+v", p)
	}
	for name, change := range map[string]func(rev int) (Match, error){
		"scenario": func(rev int) (Match, error) { return f.svc.ClearScenario(f.ctx, playerOf(10, "Hador"), 5, 1, rev) },
		"roles": func(rev int) (Match, error) {
			return f.svc.SetSideRoles(f.ctx, playerOf(10, "Hador"), 5, 1, rev, []string{"", ""})
		},
	} {
		m, _ = f.svc.ConfirmEnd(f.ctx, playerOf(20, "Gor"), 5, 1, f.match.Revision)
		m, err = change(f.match.Revision)
		if err != nil || m.EndProposal == nil || m.EndProposal.Companies[1].Confirmed || !strings.Contains(m.EndProposal.Notice, "changed the") {
			t.Errorf("%s: %+v %v", name, m.EndProposal, err)
		}
	}
}

func TestEndAccessForTable(t *testing.T) {
	m := testMatch()
	cos := matchCompanies(m)
	pending := testMatch()
	pending.EndProposal = &EndProposal{ProposedByID: 10, Companies: []EndConfirmation{
		{CompanyID: 1, OwnerID: 10, Confirmed: true}, {CompanyID: 2, OwnerID: 20}, {CompanyID: 3, OwnerID: 30, Confirmed: true}}}
	ended := testMatch()
	ended.Ended = true
	tests := []struct {
		name string
		u    User
		m    Match
		want EndAccess
	}{
		{"participant, no proposal", playerOf(10, "a"), m, EndAccess{CanPropose: true}},
		{"organiser, no proposal", playerOf(matchOrganiser, "o"), m, EndAccess{CanPropose: true}},
		{"viewer", playerOf(7, "v"), m, EndAccess{Reason: "Only the players in this match can change it."}},
		{"waiting owner", playerOf(20, "w"), pending, EndAccess{CanConfirm: true, CanCancel: true}},
		{"confirmed owner", playerOf(10, "c"), pending, EndAccess{CanWithdraw: true, CanCancel: true}},
		{"organiser sees the override", playerOf(matchOrganiser, "o"), pending, EndAccess{CanCancel: true, CanOverride: true}},
		{"admin sees the override", User{ID: 5, Role: RoleAdmin}, pending, EndAccess{CanCancel: true, CanOverride: true}},
		{"viewer, pending", playerOf(7, "v"), pending, EndAccess{Reason: "Only the players in this match can change it."}},
		{"ended", playerOf(10, "a"), ended, EndAccess{Reason: "This match has ended."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EndAccessFor(tt.u, tt.m, cos, matchOrganiser); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCancelPendingEndForTheLiveMode(t *testing.T) {
	f := endFixture(t)
	var got []EndChange
	f.svc.EndNotify = func(_ context.Context, ch EndChange) { got = append(got, ch) }
	if ok, err := f.svc.CancelPendingEnd(f.ctx, playerOf(10, "Hador"), 5, 1, "x"); ok || err != nil {
		t.Fatalf("nothing pending: %v %v", ok, err)
	}
	if _, err := f.svc.ProposeEnd(f.ctx, playerOf(10, "Hador"), 5, 1, 3); err != nil {
		t.Fatal(err)
	}
	ok, err := f.svc.CancelPendingEnd(f.ctx, playerOf(20, "Gor"), 5, 1, "Ending cancelled: Aldor wounded Grashnak")
	if !ok || err != nil || f.match.EndProposal != nil {
		t.Fatalf("%v %v %+v", ok, err, f.match.EndProposal)
	}
	if len(got) != 2 || got[0].Kind != "endProposed" || got[1].Kind != "endCancelled" || got[1].Text != "Ending cancelled: Aldor wounded Grashnak" {
		t.Errorf("notifications: %+v", got)
	}
}

// Live integration: the log tells about the end, a game action cancels it, an end check does not, Start turn is refused.
func TestLivePlayDuringAPendingEnd(t *testing.T) {
	f := newLiveFixture(t)
	f.match.Scenario = &MatchScenario{Kind: ScenarioCustom, Name: "x"}
	f.match.Result = MatchResult{Decided: true, Winner: 1, Sides: make([]SideResult, 2)}
	ms := f.matchesFixture.svc
	ms.EndNotify = func(ctx context.Context, ch EndChange) { f.svc.RecordEnd(ctx, ch) }
	f.svc.WithEnds(ms)
	mine, err := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	theirs, _ := f.svc.Live(f.ctx, f.user(liveOwner2), 5, 2)
	unit, enemy := firstUnit(mine), firstUnit(theirs)

	kinds := func() []LiveKind {
		log, _ := f.svc.Log(f.ctx, f.user(liveOwner1), 5, 1, 0, 50)
		var out []LiveKind
		for i := len(log) - 1; i >= 0; i-- {
			out = append(out, log[i].Action.Kind)
		}
		return out
	}
	m, err := ms.ProposeEnd(f.ctx, f.user(liveOwner1), 5, 1, 3)
	if err != nil || m.EndProposal == nil {
		t.Fatalf("%+v %v", m, err)
	}
	if got := kinds(); len(got) != 1 || got[0] != LiveEndProposed {
		t.Fatalf("the proposal is in the log: %v", got)
	}
	log, _ := f.svc.Log(f.ctx, f.user(liveOwner1), 5, 1, 0, 50)
	if log[0].CanUndo || log[0].CompanyID != 1 {
		t.Errorf("informational lines are not undoable and carry the company: %+v", log[0])
	}
	// the turn is refused while the end is pending
	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveTurn, Turn: 1}); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "Cancel the ending first") {
		t.Errorf("Start turn while pending: %v", err)
	}
	// an end check (an MVP vote of an opponent) keeps the proposal
	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner2), 5, 1, 1, LiveAction{Kind: LiveMVPVote, UnitID: unit}); err != nil {
		t.Fatal(err)
	}
	if f.match.EndProposal == nil {
		t.Fatal("an end check does not cancel the end")
	}
	// a wound cancels it, with a line in the log
	_, entry, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveWound, UnitID: unit, Targets: []WoundTarget{{UnitID: enemy, Caused: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if f.match.EndProposal != nil {
		t.Fatal("a game action cancels the pending end")
	}
	log, _ = f.svc.Log(f.ctx, f.user(liveOwner1), 5, 1, 0, 50)
	if log[0].Action.Kind != LiveEndCancelled || log[0].Text != "Ending cancelled: "+entry.Text {
		t.Errorf("the cancellation line: %+v", log[0])
	}
	// an undo cancels a pending end too, but the app's own lines cannot be undone
	if _, err := ms.ProposeEnd(f.ctx, f.user(liveOwner1), 5, 1, f.match.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Undo(f.ctx, f.user(liveOwner1), 5, 1, log[0].ID, 1); !errors.Is(err, ErrRefused) {
		t.Errorf("informational lines stay: %v", err)
	}
	if _, err := f.svc.Undo(f.ctx, f.user(liveOwner1), 5, 1, entry.ID, 1); err != nil {
		t.Fatal(err)
	}
	if f.match.EndProposal != nil {
		t.Error("undoing cancels a pending end")
	}
	// the fold ignores the lines: the state is what the wound and its undo left
	if got, _ := f.svc.Live(f.ctx, f.user(liveOwner2), 5, 2); got.Units[0].Wounds.Current != theirs.Units[0].Wounds.Current {
		t.Errorf("informational entries do not touch the state: %+v", got.Units[0])
	}
	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveEndConfirmed}); !errors.Is(err, ErrInvalidLive) {
		t.Errorf("a player cannot write the app's lines: %v", err)
	}
}

func TestEndCheckKinds(t *testing.T) {
	for k, want := range map[LiveKind]bool{LiveObjective: true, LiveMVPVote: true, LiveSecretDone: true, LiveSecretNominate: true, LiveEndCancelled: true,
		LiveWound: false, LiveStatChange: false, LiveTurn: false, LiveItemUsed: false, LiveAllIsLost: false} {
		if k.EndCheckKind() != want {
			t.Errorf("%s: %v", k, !want)
		}
	}
}
