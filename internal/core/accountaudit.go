package core

import (
	"strconv"
	"time"
)

// AuditSource says where an audited action came from.
type AuditSource string

const (
	AuditWeb     AuditSource = "web"     // a signed-in person in the app (ActorID is set)
	AuditConsole AuditSource = "console" // the server's command line (make grant)
	AuditPublic  AuditSource = "public"  // somebody holding a link, not signed in
	AuditSystem  AuditSource = "system"  // the app itself (a login or an app access invalidating a recovery link)
)

// AuditAction is one of the audited account actions (table audit_events, column action).
type AuditAction string

// The kinds of audited target (audit_events.target_kind).
const (
	AuditTargetInvitation   = "account_invitation"
	AuditTargetRecoveryLink = "recovery_link"
)

const (
	AuditRoleChanged         AuditAction = "role.changed"
	AuditInvitationCreated   AuditAction = "invitation.created"
	AuditInvitationRevoked   AuditAction = "invitation.revoked"
	AuditInvitationUsed      AuditAction = "invitation.used"
	AuditRecoveryCreated     AuditAction = "recovery.created"
	AuditRecoveryRevoked     AuditAction = "recovery.revoked"
	AuditRecoveryUsed        AuditAction = "recovery.used"
	AuditRecoveryInvalidated AuditAction = "recovery.invalidated"
)

// AuditEntry is one row of audit_events, written by a repository in the same transaction as the change it records. It is
// built only by the constructors below, which take typed arguments: there is no way to put a token, a hash, a password or an
// email address in it (the detail keys are a closed list per action).
type AuditEntry struct {
	At            time.Time
	Source        AuditSource
	ActorID       *int64 // nil for console, public and system
	Action        AuditAction
	SubjectUserID *int64 // the account affected (the new account for invitation.used)
	TargetKind    string // "", AuditTargetInvitation or AuditTargetRecoveryLink
	// TargetID is the invitation or link the entry is about. Where the row does not exist yet when the service builds the entry
	// (a created invitation or link, a recovery revoked by user), the repository sets it.
	TargetID int64
	Detail   map[string]string
}

func ptr(id int64) *int64 { return &id }

// RoleChangedAudit records a change of role or of the campaign-creation flag. actorID is nil for the console.
func RoleChangedAudit(at time.Time, source AuditSource, actorID *int64, subjectID int64, fromRole, toRole Role, fromCan, toCan bool) AuditEntry {
	return AuditEntry{At: at, Source: source, ActorID: actorID, Action: AuditRoleChanged, SubjectUserID: ptr(subjectID), Detail: map[string]string{
		"from_role": string(fromRole), "to_role": string(toRole),
		"from_can_create": strconv.FormatBool(fromCan), "to_can_create": strconv.FormatBool(toCan),
	}}
}

// InvitationCreatedAudit records a new account invitation; the repository sets TargetID to its id.
func InvitationCreatedAudit(at time.Time, actorID int64, kind InvitationKind, expires time.Time, maxUses int, canCreate bool) AuditEntry {
	return AuditEntry{At: at, Source: AuditWeb, ActorID: ptr(actorID), Action: AuditInvitationCreated, TargetKind: AuditTargetInvitation, Detail: map[string]string{
		"kind": string(kind), "expires_at": expires.UTC().Format(time.RFC3339), "max_uses": strconv.Itoa(maxUses), "can_create": strconv.FormatBool(canCreate),
	}}
}

// InvitationRevokedAudit records the revoking of an invitation.
func InvitationRevokedAudit(at time.Time, actorID, invitationID int64) AuditEntry {
	return AuditEntry{At: at, Source: AuditWeb, ActorID: ptr(actorID), Action: AuditInvitationRevoked, TargetKind: AuditTargetInvitation, TargetID: invitationID, Detail: map[string]string{}}
}

// InvitationUsedAudit records an account made through an invitation (the subject is the new account).
func InvitationUsedAudit(at time.Time, newUserID, invitationID int64) AuditEntry {
	return AuditEntry{At: at, Source: AuditPublic, Action: AuditInvitationUsed, SubjectUserID: ptr(newUserID), TargetKind: AuditTargetInvitation, TargetID: invitationID, Detail: map[string]string{}}
}

// RecoveryCreatedAudit records a recovery link for the user. The repository sets TargetID to the new link's id and, once it
// knows whether it replaced an earlier live link, calls WithReplaced.
func RecoveryCreatedAudit(at time.Time, actorID, userID int64, expires time.Time) AuditEntry {
	return AuditEntry{At: at, Source: AuditWeb, ActorID: ptr(actorID), Action: AuditRecoveryCreated, SubjectUserID: ptr(userID), TargetKind: AuditTargetRecoveryLink, Detail: map[string]string{
		"expires_at": expires.UTC().Format(time.RFC3339), "replaced": "false",
	}}
}

// WithReplaced sets the replaced detail of a recovery.created entry (a copy; the original is unchanged).
func (e AuditEntry) WithReplaced(replaced bool) AuditEntry {
	d := make(map[string]string, len(e.Detail))
	for k, v := range e.Detail {
		d[k] = v
	}
	d["replaced"] = strconv.FormatBool(replaced)
	e.Detail = d
	return e
}

// RecoveryRevokedAudit records the revoking of the user's live link; the repository sets TargetID to that link's id.
func RecoveryRevokedAudit(at time.Time, actorID, userID int64) AuditEntry {
	return AuditEntry{At: at, Source: AuditWeb, ActorID: ptr(actorID), Action: AuditRecoveryRevoked, SubjectUserID: ptr(userID), TargetKind: AuditTargetRecoveryLink, Detail: map[string]string{}}
}

// RecoveryUsedAudit records the new password set through a link.
func RecoveryUsedAudit(at time.Time, userID, linkID int64) AuditEntry {
	return AuditEntry{At: at, Source: AuditPublic, Action: AuditRecoveryUsed, SubjectUserID: ptr(userID), TargetKind: AuditTargetRecoveryLink, TargetID: linkID, Detail: map[string]string{}}
}

// RecoveryInvalidatedAudit records a link made unusable by the app (reason: RecoveryReason*), source system.
func RecoveryInvalidatedAudit(at time.Time, userID, linkID int64, reason RecoveryReason) AuditEntry {
	return AuditEntry{At: at, Source: AuditSystem, Action: AuditRecoveryInvalidated, SubjectUserID: ptr(userID), TargetKind: AuditTargetRecoveryLink, TargetID: linkID, Detail: map[string]string{"reason": string(reason)}}
}
