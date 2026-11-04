package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

//go:generate go tool mockgen -typed -source=archiver.go -destination=mock_archiver_test.go -package=core

// ErrArchived is returned by every change to a campaign that has been archived: ending a campaign turns it into a read-only document
// (docs/archive.md) and what the change would store is gone.
var ErrArchived = errors.Join(ErrRefused, errors.New("This campaign is an archive: it has ended and can no longer be changed."))

// CampaignHeader is what the archive needs of the campaign row itself.
type CampaignHeader struct {
	ID                int64
	Name, Description string
	Status            CampaignStatus
	Round, Rounds     int
	Settings          CampaignSettings
	Hashes            RulesetHashes
	Archived          bool
}

// ArchiveMeta describes a stored archive: the figures that sit next to the document in its row.
type ArchiveMeta struct {
	Format             int
	RawSize            int // bytes of the JSON document before it was packed
	Companies, Matches int
	Files              []RulesetInfo // the three files the campaign was played with
}

// archiveRepository keeps the archives of ended campaigns.
type archiveRepository interface {
	// Save stores the packed document of a finished campaign that is not archived yet and, in the same transaction, deletes the rows the
	// document replaces (the units, equipment, cemetery, perks and aftermaths of its companies, its matches with their live data and
	// history, the grants and the invitation links), releases the pins on its three rules files and marks the campaign archived. The
	// campaign row, its members and its companies' identities stay. ErrNotFound for an unknown campaign; ErrRefused when it is not
	// finished; ErrArchived when it already is.
	Save(ctx context.Context, campaignID int64, packed []byte, meta ArchiveMeta) error
	// Unarchived lists the finished campaigns that have no archive (the ones ended before archives existed, or whose archiving failed).
	Unarchived(ctx context.Context) ([]int64, error)
}

// Archiver makes the archive of a finished campaign (docs/archive.md). Ending a campaign calls it once the status is final; it can be
// called again for a campaign it failed on (the console command "server archive").
type Archiver struct {
	campaigns campaignRepository
	companies companyRepository
	matches   matchRepository
	aftermath aftermathRepository
	rulesets  rulesetRepository
	rules     ruleSource
	archives  archiveRepository
	now       func() time.Time
	log       *slog.Logger
}

// NewArchiver builds the service. rules is the loader (*Rules).
func NewArchiver(campaigns campaignRepository, companies companyRepository, matches matchRepository, aftermath aftermathRepository, rulesets rulesetRepository, rules ruleSource, archives archiveRepository, log *slog.Logger) *Archiver {
	if log == nil {
		log = slog.Default()
	}
	return &Archiver{campaigns: campaigns, companies: companies, matches: matches, aftermath: aftermath, rulesets: rulesets, rules: rules, archives: archives, now: time.Now, log: log}
}

// Archive reads a finished campaign, prunes its rules, checks that the pruned rules build the same companies as the full ones, and stores
// the document in place of its rows. An archived campaign is left alone (nil). A campaign that is not finished is ErrRefused.
func (a *Archiver) Archive(ctx context.Context, campaignID int64) error {
	h, err := a.campaigns.Header(ctx, campaignID)
	if err != nil {
		return err
	}
	switch {
	case h.Archived:
		return nil
	case h.Status != CampaignFinished:
		return Refuse("Only an ended campaign is archived.")
	}
	rs, err := a.rules.For(ctx, h.Hashes)
	if err != nil {
		return fmt.Errorf("the rules of the campaign: %w", err)
	}
	src := ArchiveSource{CampaignID: h.ID, Name: h.Name, Description: h.Description, Round: h.Round, Rounds: h.Rounds, Settings: h.Settings, EndedAt: a.now().UTC(),
		Events: map[int64][]MatchEvent{}, Grants: map[int64][]Grant{}, History: map[int64][]ArchivedLine{}}
	for _, f := range []struct {
		kind RulesetKind
		hash string
		into *[]byte
	}{{KindVanilla, h.Hashes.Vanilla, &src.Raw.Vanilla}, {KindBattleCompanies, h.Hashes.BattleCompanies, &src.Raw.BattleCompanies}, {KindGlossary, h.Hashes.Glossary, &src.Raw.Glossary}} {
		stored, err := a.rulesets.Get(ctx, f.hash)
		if err != nil {
			return fmt.Errorf("the %s file: %w", f.kind.Label(), err)
		}
		*f.into = stored.Content
		src.Files = append(src.Files, stored.RulesetInfo)
	}
	companies, err := a.campaigns.Companies(ctx, campaignID)
	if err != nil {
		return err
	}
	for _, c := range companies {
		st, err := a.companies.Read(ctx, campaignID, c.ID)
		if err != nil {
			return fmt.Errorf("company %d: %w", c.ID, err)
		}
		src.States = append(src.States, st)
		grants, err := a.matches.Grants(ctx, campaignID, c.ID)
		if err != nil {
			return fmt.Errorf("grants of company %d: %w", c.ID, err)
		}
		if len(grants) > 0 {
			src.Grants[c.ID] = grants
		}
		lines, err := a.aftermath.History(ctx, campaignID, c.ID, 1<<20)
		if err != nil {
			return fmt.Errorf("aftermath log of company %d: %w", c.ID, err)
		}
		for _, l := range lines {
			src.History[c.ID] = append(src.History[c.ID], ArchivedLine{AftermathLine: l, MatchID: l.MatchID, On: l.On})
		}
	}
	if src.Matches, err = a.matches.List(ctx, campaignID); err != nil {
		return err
	}
	for _, m := range src.Matches {
		ev, err := a.matches.Events(ctx, campaignID, m.ID)
		if err != nil {
			return fmt.Errorf("history of match %d: %w", m.ID, err)
		}
		if len(ev) > 0 {
			src.Events[m.ID] = ev
		}
	}
	doc, err := BuildArchive(rs, src)
	if err != nil {
		return err
	}
	// the pruned rules must give every company the roster the full rules give it, or the archive would show something else than was played
	mini, err := doc.LoadPruned()
	if err != nil {
		return err
	}
	for _, st := range doc.Companies {
		full, small := BuildDetail(rs, h.Settings, st), BuildDetail(mini, h.Settings, st)
		if len(full.Roster) != len(small.Roster) {
			return fmt.Errorf("company %q: the pruned rules give %d units, the full rules %d", st.Company.Name, len(small.Roster), len(full.Roster))
		}
		for i := range full.Roster {
			if a, b := digestUnit(full.Roster[i]), digestUnit(small.Roster[i]); a != b {
				at := 0
				for at < len(a) && at < len(b) && a[at] == b[at] {
					at++
				}
				return fmt.Errorf("company %q: unit %s is not the same with the pruned rules, from byte %d: full %q, pruned %q", st.Company.Name, full.Roster[i].Type, at,
					a[max(0, at-30):min(len(a), at+60)], b[max(0, at-30):min(len(b), at+60)])
			}
		}
		if full.Points != small.Points {
			return fmt.Errorf("company %q: the pruned rules give %d points, the full rules %d", st.Company.Name, small.Points, full.Points)
		}
	}
	raw, err := doc.rawJSON()
	if err != nil {
		return err
	}
	packed, err := doc.Pack()
	if err != nil {
		return err
	}
	meta := ArchiveMeta{Format: ArchiveFormat, RawSize: len(raw), Companies: len(doc.Companies), Matches: len(doc.Matches), Files: src.Files}
	if err := a.archives.Save(ctx, campaignID, packed, meta); err != nil {
		return err
	}
	a.log.Info("campaign archived", "campaign", campaignID, "raw_bytes", len(raw), "stored_bytes", len(packed), "companies", meta.Companies, "matches", meta.Matches)
	return nil
}

// ArchiveFinished archives every finished campaign that has no archive. It goes on after a failure and returns how many were archived and
// the failures, one per campaign.
func (a *Archiver) ArchiveFinished(ctx context.Context) (done int, failed map[int64]error, err error) {
	ids, err := a.archives.Unarchived(ctx)
	if err != nil {
		return 0, nil, err
	}
	failed = map[int64]error{}
	for _, id := range ids {
		if err := a.Archive(ctx, id); err != nil {
			failed[id] = err
			continue
		}
		done++
	}
	return done, failed, nil
}
