package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The last-campaign memory (docs/multi-company-spec.md, section 8): a cookie holding three ids, "<user>.<campaign>[.<company>]". It is
// unsigned and authorises nothing: GET / and the My Company alias validate it against the database on every read, and it is cleared when
// it is malformed, another user's, finished or no longer the user's.

const (
	lastCookie    = "bcm-last"
	lastMaxLen    = 64 // three int64s and two dots fit in 59 characters; anything longer is not ours
	lastMaxAge    = 90 * 24 * 60 * 60
	lastCookieEnd = lastCookie + "="
)

// lastVisit is the content of the bcm-last cookie. CompanyID is 0 when only the campaign was opened.
type lastVisit struct{ UserID, CampaignID, CompanyID int64 }

// parseLastVisit reads the cookie strictly. It reports false for an absent or malformed value (wrong length, parts that are not plain
// positive base-10 integers, a number of parts other than two or three).
func parseLastVisit(r *http.Request) (lastVisit, bool) {
	c, err := r.Cookie(lastCookie)
	if err != nil || c.Value == "" || len(c.Value) > lastMaxLen {
		return lastVisit{}, false
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return lastVisit{}, false
	}
	var ids [3]int64
	for i, p := range parts {
		if len(p) == 0 || len(p) > 18 {
			return lastVisit{}, false
		}
		for _, ch := range p {
			if ch < '0' || ch > '9' {
				return lastVisit{}, false
			}
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n <= 0 {
			return lastVisit{}, false
		}
		ids[i] = n
	}
	return lastVisit{UserID: ids[0], CampaignID: ids[1], CompanyID: ids[2]}, true
}

func (v lastVisit) value() string {
	s := strconv.FormatInt(v.UserID, 10) + "." + strconv.FormatInt(v.CampaignID, 10)
	if v.CompanyID > 0 {
		s += "." + strconv.FormatInt(v.CompanyID, 10)
	}
	return s
}

// setLastCookie writes the cookie, replacing a bcm-last that an earlier step of the same response already set.
func (s *Server) setLastCookie(w http.ResponseWriter, value string, maxAge int) {
	h := w.Header()
	if old := h.Values("Set-Cookie"); len(old) > 0 {
		kept := make([]string, 0, len(old))
		for _, c := range old {
			if !strings.HasPrefix(c, lastCookieEnd) {
				kept = append(kept, c)
			}
		}
		h.Del("Set-Cookie")
		for _, c := range kept {
			h.Add("Set-Cookie", c)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: lastCookie, Value: value, Path: "/", MaxAge: maxAge, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.cfg.SecureCookies})
}

// forgetVisit clears the cookie.
func (s *Server) forgetVisit(w http.ResponseWriter) { s.setLastCookie(w, "", -1) }

// fullPage is whether the request renders a whole page for the browser: a GET that is not an htmx fragment nor an event stream.
func fullPage(r *http.Request) bool {
	return r.Method == http.MethodGet && r.Header.Get("HX-Request") != "true" && !strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

// rememberVisit records the campaign (and the company, when the page is one of the viewer's own companies) the signed-in user opened.
// Only full-page GETs of a campaign that is not finished write it; the cookie is renewed each time (90 days, sliding). Without a company
// it keeps the one remembered for the same campaign while that is still the viewer's own.
func (s *Server) rememberVisit(w http.ResponseWriter, r *http.Request, d core.CampaignDetail, companyID int64) {
	u := userFrom(r.Context())
	if u == nil || !fullPage(r) || d.Status == core.CampaignFinished {
		return
	}
	v := lastVisit{UserID: u.ID, CampaignID: d.ID, CompanyID: companyID}
	if companyID == 0 {
		if old, ok := parseLastVisit(r); ok && old.UserID == u.ID && old.CampaignID == d.ID && d.Owns(old.CompanyID) {
			v.CompanyID = old.CompanyID
		}
	}
	s.setLastCookie(w, v.value(), lastMaxAge)
}

// lastCompany is the company the alias opens: the remembered one when it is still one of the viewer's own in this campaign, else the
// first (oldest) one. ok is false when the viewer has no company.
func lastCompany(r *http.Request, d core.CampaignDetail) (id int64, ok bool) {
	if len(d.Companies) == 0 {
		return 0, false
	}
	if v, ok := parseLastVisit(r); ok && userFrom(r.Context()) != nil && v.UserID == userFrom(r.Context()).ID && v.CampaignID == d.ID && d.Owns(v.CompanyID) {
		return v.CompanyID, true
	}
	return d.Companies[0].ID, true
}
