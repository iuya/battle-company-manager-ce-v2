package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The Invitations page of the administration (docs/admin.md, "Invitations"): the creation card, then the list of every account
// invitation with its state. The creation and revoke buttons swap their own card or row; the secret link is in the answer to the
// creation only, never in the list.

const invitationPageSize = 25

var invitationStates = []struct{ Value, Label string }{{"", "All"}, {"pending", "Pending"}, {"used", "Used"}, {"expired", "Expired"}, {"revoked", "Revoked"}}

type adminInvitationsView struct {
	adminBase
	Create invitationCard
	List   invitationList
}

// invitationCard is the creation form: what was typed (kept after a refusal), its messages and, after a success, the link just made.
type invitationCard struct {
	Kind    string // "named" (the default) or "anonymous"
	Email   string
	Days    int
	MaxUses string
	Can     bool
	Nonce   string // see adminUserRow.Nonce
	Err     string
	Errors  map[string]string
	Link    *shownLink
}

type invitationList struct {
	State string
	Rows  []invitationRow
	Total int
	Next  int
}

// NextURL is the "Show more" request.
func (l invitationList) NextURL() string {
	return "/admin/invitations?state=" + url.QueryEscape(l.State) + "&offset=" + strconv.Itoa(l.Next)
}

// States feeds the filter chips.
func (invitationList) States() []struct{ Value, Label string } { return invitationStates }

type invitationRow struct {
	I   core.AccountInvitation
	Err string
}

// Pending says the invitation still works (it offers Revoke).
func (r invitationRow) Pending() bool { return r.I.State == core.InvitationPending }

func newInvitationCard() invitationCard {
	return invitationCard{Kind: "named", Days: 7, MaxUses: "10", Nonce: nonce()}
}

func (s *Server) invitationList(r *http.Request, state string, offset int) (invitationList, error) {
	f := core.InvitationFilter{State: core.InvitationState(state)}
	rows, total, err := s.invitations.List(r.Context(), *userFrom(r.Context()), f, invitationPageSize, offset)
	if err != nil {
		return invitationList{}, err
	}
	l := invitationList{State: state, Total: total}
	for _, i := range rows {
		l.Rows = append(l.Rows, invitationRow{I: i})
	}
	if next := offset + len(rows); len(rows) > 0 && next < total {
		l.Next = next
	}
	return l, nil
}

func (s *Server) handleAdminInvitations(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if !slicesHasState(state) {
		state = ""
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	list, err := s.invitationList(r, state, max(offset, 0))
	if s.fail(w, r, err) {
		return
	}
	// The filter chips and "Show more" swap the list alone; everything else is the whole page.
	if t := r.Header.Get("HX-Target"); t == "invitation-list" || t == "invitation-more" {
		w.Header().Add("Vary", "HX-Target")
		name := "admin_invitation_list"
		if offset > 0 {
			name = "admin_invitation_page"
		}
		s.render.partial(w, http.StatusOK, "admin_invitations", name, list)
		return
	}
	s.render.page(w, r, http.StatusOK, "admin_invitations", adminInvitationsView{adminBase: s.adminBase(r, "invitations"), Create: newInvitationCard(), List: list})
}

func slicesHasState(v string) bool {
	for _, s := range invitationStates {
		if s.Value == v {
			return true
		}
	}
	return false
}

// handleAdminInvitationCreate makes an invitation and answers the creation card, with the message to copy on success.
func (s *Server) handleAdminInvitationCreate(w http.ResponseWriter, r *http.Request) {
	me := *userFrom(r.Context())
	card := newInvitationCard()
	card.Kind = r.PostFormValue("kind")
	if card.Kind != "anonymous" {
		card.Kind = "named"
	}
	card.Email, card.MaxUses, card.Can = strings.TrimSpace(r.PostFormValue("email")), strings.TrimSpace(r.PostFormValue("max_uses")), r.PostFormValue("can_create") == "on"
	expiry := time.Duration(0)
	if v := strings.TrimSpace(r.PostFormValue("days")); v != "" {
		d, err := strconv.Atoi(v)
		if err != nil || d < 0 {
			d = -1 // the service answers with its range message
		}
		card.Days = d
		expiry = time.Duration(max(min(d, maxClampUnits), -1)) * 24 * time.Hour
	}
	in := core.NewAccountInvitation{Kind: core.InvitationKind(card.Kind), Email: card.Email, CanCreateCampaigns: card.Can, Expiry: expiry}
	if card.Kind == "anonymous" {
		n, err := strconv.Atoi(card.MaxUses)
		if err != nil {
			card.Errors = map[string]string{"max_uses": "Enter a number of accounts from 1 to 50."}
			s.render.partial(w, http.StatusUnprocessableEntity, "admin_invitations", "admin_invitation_create", card)
			return
		}
		in.MaxUses = n
	}
	created, err := s.invitations.Create(r.Context(), me, in)
	switch {
	case errors.Is(err, core.ErrRefused), errors.Is(err, core.ErrInvalidInput):
		card.Err = strings.Join(reasons(err), ". ") + "."
		s.render.partial(w, http.StatusUnprocessableEntity, "admin_invitations", "admin_invitation_create", card)
		return
	case s.fail(w, r, err):
		return
	}
	noStore(w)
	w.Header().Set("HX-Trigger", "invitations-changed")
	fresh := newInvitationCard()
	fresh.Link = &shownLink{Message: invitationMessage(r, created), Expires: created.ExpiresAt}
	s.render.partial(w, http.StatusOK, "admin_invitations", "admin_invitation_create", fresh)
}

// invitationMessage is the text the administrator pastes to the person. A named link carries ?email= for the person to read: the
// registration page ignores it and shows the address the invitation holds.
func invitationMessage(r *http.Request, c core.CreatedAccountInvitation) string {
	link := absoluteURL(r, "/register/"+url.PathEscape(c.Token))
	when := c.ExpiresAt.In(locationFrom(r.Context())).Format("2 January 2006, 15:04")
	if c.Kind == core.InvitationNamed {
		return "Open this link to create your account: " + link + "?email=" + url.QueryEscape(c.Email) + "\nIt works once and expires on " + when + "."
	}
	who := strconv.Itoa(c.MaxUses) + " people"
	if c.MaxUses == 1 {
		who = "1 person"
	}
	return "Open this link to create your account: " + link + "\nIt works for up to " + who + " and expires on " + when + "."
}

// handleAdminInvitationRevoke revokes a pending invitation and answers its row. A refusal (already used, expired or revoked) is a
// 422 with the reason in the row's message slot.
func (s *Server) handleAdminInvitationRevoke(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.notFound(w, r)
		return
	}
	inv, err := s.invitations.Revoke(r.Context(), *userFrom(r.Context()), id)
	if errors.Is(err, core.ErrRefused) {
		w.Header().Set("HX-Retarget", "#invitation-"+strconv.FormatInt(id, 10)+"-msg")
		w.Header().Set("HX-Reswap", "innerHTML")
		s.render.partial(w, http.StatusUnprocessableEntity, "admin_invitations", "admin_invitation_error", reason(err))
		return
	}
	if s.fail(w, r, err) {
		return
	}
	s.render.partial(w, http.StatusOK, "admin_invitations", "admin_invitation_row", invitationRow{I: inv})
}
