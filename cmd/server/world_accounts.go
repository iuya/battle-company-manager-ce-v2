package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// worldMarker is the first campaign of the world: its presence means the world is loaded.
const worldMarker = "Brand new campaign: no companies yet"

// worldAccount is a demo account and what it shows.
type worldAccount struct {
	key, email, name string
	role             core.Role
	canCreate        bool
	shows            string
}

var worldAccounts = []worldAccount{
	{"admin", "admin@example.com", "Gandalf", core.RoleSuperuser, true, "Superuser: /admin, reopen matches, waive aftermaths, every campaign action"},
	{"steward", "steward@example.com", "Denethor", core.RoleAdmin, true, "Administrator (not superuser): administers campaigns; sees Users and Invitations and the Organiser switch; no rules-file or recovery-link powers"},
	{"you", "you@example.com", "You", core.RoleUser, true, "Organiser of most campaigns (grants, matches, invites); also plays in a few"},
	{"faramir", "faramir@example.com", "Faramir", core.RoleUser, true, "Can create campaigns; organises one campaign and, after the handover, another"},
	{"eowyn", "eowyn@example.com", "Éowyn", core.RoleUser, false, "Player in many campaigns; cannot create campaigns"},
	{"sauron", "sauron@example.com", "Sauron", core.RoleUser, false, "Player; the dark side of the matches"},
	{"aragorn", "aragorn@example.com", "Aragorn", core.RoleUser, false, "Player; joined the brand new campaign but has no company there yet"},
	{"gimli", "gimli@example.com", "Gimli", core.RoleUser, false, "Player; the Dwarf companies and the limits campaign"},
	{"bilbo", "bilbo@example.com", "Bilbo", core.RoleUser, false, "Player; Hobbit and Wanderers companies"},
	{"galadriel", "galadriel@example.com", "Galadriel", core.RoleUser, false, "Player; Elf companies, the Sorcerer path"},
	{"radagast", "radagast@example.com", "Radagast", core.RoleUser, false, "Player; creatures and wanderers"},
	{"boromir", "boromir@example.com", "Boromir", core.RoleUser, false, "Closed account at the end of the script: shows as \"Former player\" in the campaigns he played"},
	{"frodo", "frodo@example.com", "Frodo", core.RoleUser, true, "Signed up, in no campaign: the empty campaign list"},
	{"pippin", "pippin@example.com", "Pippin", core.RoleUser, false, "Cannot create campaigns (no New campaign button); plays in a few"},
}

// accounts creates the fixed accounts. The first four of demoUsers (admin, you, eowyn, sauron) are the accounts `make seed`
// makes too, so they keep their address and password.
func (w *world) accounts() {
	for _, a := range worldAccounts {
		w.account(a.key, a.email, a.name, a.role, a.canCreate)
	}
}

func (w *world) printAccounts() {
	fmt.Fprintf(w.out, "\nDemo accounts (password %q for all):\n", w.pw)
	fmt.Fprintf(w.out, "  %-24s %-10s %s\n", "email", "role", "what it shows")
	for _, a := range worldAccounts {
		role := string(a.role)
		if a.canCreate && a.role == core.RoleUser {
			role += "+create"
		}
		fmt.Fprintf(w.out, "  %-24s %-10s %s\n", a.email, role, a.shows)
	}
	for _, a := range adminStateAccounts {
		fmt.Fprintf(w.out, "  %-24s %-10s %s\n", a[0], "user", a[1])
	}
	fmt.Fprintf(w.out, "  %-24s %-10s %s\n", "<company>@example.com", "user", "Gallery: one player per Battle Company (e.g. minas-tirith@example.com, goblin-town@example.com)")
	fmt.Fprintln(w.out, "\nSign in at http://localhost:8080 and read docs/demo-world.md for what to click.")
}

func (w *world) printLinks() {
	printLinkList(w.out, "Invite links", w.links)
	printLinkList(w.out, "Account invitation links", w.accountLinks)
}

func printLinkList(out io.Writer, heading string, links []string) {
	if len(links) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%s (the secret is shown only now; prefix with http://localhost:8080):\n", heading)
	for _, l := range links {
		fmt.Fprintln(out, "  "+l)
	}
}

func (w *world) printGaps() {
	if len(w.gaps) == 0 {
		return
	}
	fmt.Fprintln(w.out, "\nStates the services cannot reach (docs/demo-world.md, \"Gaps\"):")
	for _, g := range w.gaps {
		fmt.Fprintln(w.out, "  -", g)
	}
}

// slug turns a company name into the local part of its gallery account.
func slug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}
