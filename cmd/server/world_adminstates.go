package main

// The administration states: every state an account invitation and a password recovery link can be in, made through the real
// services with injected clocks (core.WithClock), so there is no raw SQL. docs/demo-world.md lists them.

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// Addresses of the accounts of this step (e2e flows and docs/demo-world.md name them).
const (
	invitePendingEmail   = "invitee@example.com"         // pending named invitation: no account yet
	inviteUsedEmail      = "newcomer@example.com"        // used named invitation: the account it made
	inviteExpiredEmail   = "latecomer@example.com"       // expired named invitation: no account
	inviteRevokedEmail   = "withdrawn@example.com"       // revoked named invitation: no account
	inviteGuestOneEmail  = "guest.one@example.com"       // made through the anonymous invitation
	inviteGuestTwoEmail  = "guest.two@example.com"       // made through the anonymous invitation
	recoveryPendingEmail = "recover-pending@example.com" // the account whose recovery link stays pending
)

// recoveryStates are the dedicated accounts of the recovery links, one per state. Nothing else (script or e2e flow) signs in
// as these, because signing in or opening the app invalidates a pending link.
var recoveryStates = []struct {
	state core.RecoveryState
	key   string // the part of the address after "recover-"
	name  string
}{
	{core.RecoveryPending, "pending", "Recover: pending link"},
	{core.RecoveryUsed, "used", "Recover: used link"},
	{core.RecoveryExpired, "expired", "Recover: expired link"},
	{core.RecoveryRevoked, "revoked", "Recover: revoked link"},
	{core.RecoveryInvalidated, "login", "Recover: invalidated by sign-in"},
	{core.RecoveryInvalidated, "access", "Recover: invalidated by access"},
	{core.RecoveryPending, "replaced", "Recover: second link replaced the first"},
}

func recoveryEmail(key string) string { return "recover-" + key + "@example.com" }

// adminStates makes the account invitations and the recovery links in every state.
func (w *world) adminStates() {
	admin, steward := w.user("admin"), w.user("steward")
	past := func() time.Time { return time.Now().Add(-10 * 24 * time.Hour) }
	invites := w.b.invitations
	pastInvites := core.NewAccountInvitations(w.b.store.AccountInvitations(), core.WithClock(past))
	recovery := w.b.recovery
	pastRecovery := core.NewRecovery(w.b.store.Recovery(), w.b.auth, core.WithClock(past))

	pending, _, err := invites.List(w.ctx, admin, core.InvitationFilter{State: core.InvitationPending}, 200, 0)
	w.must("list invitations", err)
	if slices.ContainsFunc(pending, func(i core.AccountInvitation) bool { return i.Email == invitePendingEmail }) {
		fmt.Fprintln(w.out, "  The administration states are there already; nothing to do.")
		return
	}

	// Account invitations
	named, err := invites.Create(w.ctx, admin, core.NewAccountInvitation{Kind: core.InvitationNamed, Email: invitePendingEmail, CanCreateCampaigns: true})
	w.must("pending named invitation", err)
	anon, err := invites.Create(w.ctx, steward, core.NewAccountInvitation{Kind: core.InvitationAnonymous, MaxUses: 5})
	w.must("anonymous invitation", err)
	for i, e := range []string{inviteGuestOneEmail, inviteGuestTwoEmail} {
		_, err := invites.Register(w.ctx, anon.Token, core.Registration{Email: e, DisplayName: []string{"Guest One", "Guest Two"}[i], Password: w.pw})
		w.must("register "+e+" through the anonymous invitation", err)
	}
	used, err := invites.Create(w.ctx, admin, core.NewAccountInvitation{Kind: core.InvitationNamed, Email: inviteUsedEmail})
	w.must("used named invitation", err)
	_, err = invites.Register(w.ctx, used.Token, core.Registration{DisplayName: "Newcomer", Password: w.pw})
	w.must("register "+inviteUsedEmail, err)
	_, err = pastInvites.Create(w.ctx, admin, core.NewAccountInvitation{Kind: core.InvitationNamed, Email: inviteExpiredEmail, Expiry: 24 * time.Hour})
	w.must("expired invitation", err)
	revoked, err := invites.Create(w.ctx, steward, core.NewAccountInvitation{Kind: core.InvitationNamed, Email: inviteRevokedEmail})
	w.must("revoked invitation", err)
	_, err = invites.Revoke(w.ctx, admin, revoked.ID)
	w.must("revoke the invitation", err)

	w.accountLinks = append(w.accountLinks,
		"pending named      /register/"+named.Token+"?email="+invitePendingEmail+"   (no account yet; may create campaigns)",
		"anonymous, 2 of 5 used  /register/"+anon.Token+"   (two accounts made: "+inviteGuestOneEmail+", "+inviteGuestTwoEmail+")")

	// Recovery links, each on its own account
	ids := map[string]core.User{}
	for _, s := range recoveryStates {
		email := recoveryEmail(s.key)
		u, err := w.b.auth.CreateUser(w.ctx, email, s.name, w.pw)
		w.must("account "+email, err)
		ids[s.key] = u
	}
	gen := func(r *core.Recovery, key string, ttl time.Duration) core.RecoveryLink {
		l, err := r.Generate(w.ctx, admin, ids[key].ID, ttl)
		w.must("recovery link of "+key, err)
		return l
	}
	l := gen(recovery, "pending", 0)
	w.accountLinks = append(w.accountLinks,
		"pending recovery   /recover/"+l.Token+"   ("+recoveryPendingEmail+"; signing in or opening the app as this account invalidates it)")
	l = gen(recovery, "used", 0)
	w.must("use the recovery link", recovery.SetPassword(w.ctx, l.Token, w.pw)) // the password stays the demo one: the account still signs in
	gen(pastRecovery, "expired", core.RecoveryMinTTL)
	gen(recovery, "revoked", 0)
	w.must("revoke the recovery link", recovery.Revoke(w.ctx, admin, ids["revoked"].ID))
	// invalidated by sign-in: the link exists, then the person signs in
	gen(recovery, "login", 0)
	_, _, _, err = w.b.auth.Login(w.ctx, recoveryEmail("login"), w.pw)
	w.must("sign in as the recover-login account", err)
	// invalidated by access: the session comes first (signing in would invalidate the link), then the link, then a request with the
	// session. No cache entry exists for the session yet, so the repository answers with the pending flag and the request invalidates.
	session, _, _, err := w.b.auth.Login(w.ctx, recoveryEmail("access"), w.pw)
	w.must("sign in as the recover-access account", err)
	gen(recovery, "access", 0)
	_, _, err = w.b.auth.Authenticate(w.ctx, session)
	w.must("open the app as the recover-access account", err)
	// replaced: a second link in a row
	first := gen(recovery, "replaced", 0)
	gen(recovery, "replaced", 0)
	if err := recovery.Preview(w.ctx, first.Token); !errors.Is(err, core.ErrLinkUnusable) {
		w.must("the replaced recovery link should be dead", fmt.Errorf("preview answered %v", err))
	}
}

// adminStateAccounts are the accounts of adminStates the accounts table prints (address, what it shows).
var adminStateAccounts = [][2]string{
	{invitePendingEmail, "Pending named account invitation: no account yet (open the printed link)"},
	{inviteUsedEmail, "Made through a named invitation (used)"},
	{inviteGuestOneEmail + ", " + inviteGuestTwoEmail, "Made through the anonymous invitation (2 of 5 used, 3 left)"},
	{"recover-<state>@example.com", "One recovery link per state: pending (never sign in as it), used, expired, revoked, login, access, replaced"},
}
