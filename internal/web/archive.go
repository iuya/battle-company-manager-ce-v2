package web

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// An ended campaign is an archive (docs/archive.md): a read-only document. Its pages are the ones that only read what a campaign was; the
// ones that need a live campaign (the armoury, recruiting, level-ups, live matches, aftermaths, the creation of a match or a company, the
// Tabletop Simulator export) are not part of it. The services refuse every change (core.ErrArchived); this is the same rule at the door of
// the pages, with an answer that says why.

// archivePages are the pages an archived campaign has, after /campaigns/{id}.
var archivePages = regexp.MustCompile(`^(|/rankings|/matches|/matches/\d+|/matches/\d+/scenario|/companies|/companies/\d+|/my-company|/my-company/\d+|/glossary|/glossary/units|/concepts/.+)$`)

// archiveAnswers says whether an archived campaign answers the request: only reads, and only of the pages above.
func archiveAnswers(method, path string, id int64) bool {
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	rest, ok := strings.CutPrefix(path, "/campaigns/"+strconv.FormatInt(id, 10))
	return ok && archivePages.MatchString(rest)
}

// gateArchive lets a request for a campaign go on, or answers it when the campaign is an archive that does not have the page. It reports
// whether the caller may go on.
func (s *Server) gateArchive(w http.ResponseWriter, r *http.Request, c core.CampaignDetail) bool {
	if !c.Archived || archiveAnswers(r.Method, r.URL.Path, c.ID) {
		return true
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.refuse(w, r, http.StatusForbidden, reason(core.ErrArchived))
		return false
	}
	s.refuse(w, r, http.StatusNotFound, "This campaign has ended and is kept as an archive: this page is not part of it.")
	return false
}
