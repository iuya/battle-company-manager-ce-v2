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
