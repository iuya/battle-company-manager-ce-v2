package core

// Level is one of the four access levels of an account. Stored as role and can_create_campaigns: a plain user is
// role=user without the flag, an organiser role=user with it (the global flag, not the per-campaign role), an administrator
// role=admin (the flag is kept as stored but ignored), a superuser role=superuser.
type Level int

const (
	LevelPlain Level = iota
	LevelOrganiser
	LevelAdministrator
	LevelSuperuser
)

// Actor is who asks for a role change: one of the four levels or the console (make grant, no account).
type Actor int

const (
	ActorPlain Actor = iota
	ActorOrganiser
	ActorAdministrator
	ActorSuperuser
	ActorConsole
)

// LevelOf is the access level of an account with that role and flag.
func LevelOf(role Role, canCreateCampaigns bool) Level {
	switch {
	case role == RoleSuperuser:
		return LevelSuperuser
	case role == RoleAdmin:
		return LevelAdministrator
	case canCreateCampaigns:
		return LevelOrganiser
	}
	return LevelPlain
}

// ActorOf is the actor a signed-in user is.
func ActorOf(u User) Actor { return Actor(LevelOf(u.Role, u.CanCreateCampaigns)) }

// RoleChangeAllowed is the permission matrix, a pure function: nil when the actor may change an account from level `from`
// to level `to` (or when that changes nothing), else ErrForbidden joined with a reason fit to show. Nobody but the console
// creates, changes or demotes a superuser; only a superuser changes an administrator; an administrator moves accounts
// between plain and organiser; plain users and organisers change nothing.
func RoleChangeAllowed(actor Actor, from, to Level) error {
	switch actor {
	case ActorConsole:
		return nil
	case ActorSuperuser:
		if from == LevelSuperuser {
			return Forbid("Superusers can only be changed from the console (make grant)")
		}
		if to == LevelSuperuser {
			return Forbid("Superusers are set from the console (make grant)")
		}
		return nil
	case ActorAdministrator:
		switch {
		case from == LevelSuperuser:
			return Forbid("Superusers can only be changed from the console (make grant)")
		case to == LevelSuperuser:
			return Forbid("Superusers are set from the console (make grant)")
		case from == LevelAdministrator || to == LevelAdministrator:
			return Forbid("Only a superuser can change an administrator")
		}
		return nil
	}
	return Forbid("Only administrators can change permissions")
}
