package postgres

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// Archiving a finished campaign releases the rules files it pinned: the usage list still works, the files can be removed, the campaign and its
// companies can no longer be changed, and a campaign that is not finished is refused.
func TestSaveArchiveReleasesTheFilesAndFreezesTheCampaign(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Campaigns()
	org := testUser(t, s, ctx, "arc-org")
	nc := testNewCampaign(t, s, ctx, "Archived")
	camp, err := repo.Create(ctx, org, nc)
	if err != nil {
		t.Fatal(err)
	}
	// the migrations' down tests share this database, and 00030 refuses to go down while an archive exists
	t.Cleanup(func() { _, _ = s.pool.Exec(ctx, `DELETE FROM campaigns WHERE id = $1`, camp.ID) })
	meta := core.ArchiveMeta{Format: 1, RawSize: 10, Files: []core.RulesetInfo{{Kind: core.KindVanilla, Name: "v", Hash: nc.Vanilla}}}
	if err := s.Archives().Save(ctx, camp.ID, []byte("x"), meta); !errors.Is(err, core.ErrRefused) {
		t.Fatalf("a campaign that is not finished: %v, want a refusal", err)
	}
	if err := repo.SetStatus(ctx, camp.ID, func(int64, string, core.CampaignStatus) (core.CampaignStatus, error) {
		return core.CampaignFinished, nil
	}); err != nil {
		t.Fatal(err)
	}
	if ids, err := s.Archives().Unarchived(ctx); err != nil || !slices.Contains(ids, camp.ID) {
		t.Fatalf("unarchived = %v, %v", ids, err)
	}
	if err := s.Archives().Save(ctx, camp.ID, []byte("x"), meta); err != nil {
		t.Fatal(err)
	}
	if err := s.Archives().Save(ctx, camp.ID, []byte("x"), meta); !errors.Is(err, core.ErrArchived) {
		t.Fatalf("a second archive: %v", err)
	}
	h, err := repo.Header(ctx, camp.ID)
	if err != nil || !h.Archived || h.Hashes.Vanilla != "archive:"+fmt.Sprint(camp.ID)+":vanilla" {
		t.Fatalf("header = %+v, %v", h, err)
	}
	usage, err := s.Rulesets().ListUsage(ctx)
	if err != nil {
		t.Fatalf("the usage list of the rules files: %v", err)
	}
	for _, u := range usage {
		if u.Hash == nc.Vanilla && len(u.Campaigns) != 0 {
			t.Errorf("the archived campaign still uses %s: %+v", u.Hash, u.Campaigns)
		}
	}
	if err := s.Rulesets().Delete(ctx, nc.Vanilla); err != nil {
		t.Errorf("the file the archive released cannot be removed: %v", err)
	}
	if err := repo.HandOver(ctx, camp.ID, org.ID, 10, func(int64) error { return nil }); !errors.Is(err, core.ErrArchived) {
		t.Errorf("hand-over of an archive: %v", err)
	}
	if _, err := s.Matches().Create(ctx, camp.ID, org, core.NewMatch{}); !errors.Is(err, core.ErrArchived) {
		t.Errorf("a match in an archive: %v", err)
	}
	if _, err := s.Companies().Create(ctx, camp.ID, org, core.NewCompany{Name: "Late", Faction: "Rohan", Cap: 1}); !errors.Is(err, core.ErrArchived) {
		t.Errorf("a company in an archive: %v", err)
	}
}
