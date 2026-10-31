package core

import (
	"slices"
	"testing"
)

// TestAuditDetailKeysAreAClosedList: each constructor yields exactly the documented keys, so nothing secret can ride along.
func TestAuditDetailKeysAreAClosedList(t *testing.T) {
	allowed := map[AuditAction][]string{
		AuditRoleChanged:         {"from_role", "to_role", "from_can_create", "to_can_create"},
		AuditInvitationCreated:   {"kind", "expires_at", "max_uses", "can_create"},
		AuditInvitationRevoked:   {},
		AuditInvitationUsed:      {},
		AuditRecoveryCreated:     {"expires_at", "replaced"},
		AuditRecoveryRevoked:     {},
		AuditRecoveryUsed:        {},
		AuditRecoveryInvalidated: {"reason"},
	}
	actor := int64(1)
	entries := []AuditEntry{
		RoleChangedAudit(t0, AuditWeb, &actor, 7, RoleUser, RoleAdmin, false, true),
		InvitationCreatedAudit(t0, 1, InvitationNamed, t0.Add(1), 1, false),
		InvitationRevokedAudit(t0, 1, 2),
		InvitationUsedAudit(t0, 7, 2),
		RecoveryCreatedAudit(t0, 1, 7, t0.Add(1)),
		RecoveryCreatedAudit(t0, 1, 7, t0.Add(1)).WithReplaced(true),
		RecoveryRevokedAudit(t0, 1, 7),
		RecoveryUsedAudit(t0, 7, 3),
		RecoveryInvalidatedAudit(t0, 7, 3, RecoveryByLogin),
	}
	seen := map[AuditAction]bool{}
	for _, e := range entries {
		seen[e.Action] = true
		var keys []string
		for k := range e.Detail {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		want := slices.Clone(allowed[e.Action])
		slices.Sort(want)
		if !slices.Equal(keys, want) {
			t.Errorf("%s: keys %v, want %v", e.Action, keys, want)
		}
		if e.Source == AuditWeb && e.ActorID == nil {
			t.Errorf("%s: a web entry needs an actor", e.Action)
		}
		if e.Source != AuditWeb && e.ActorID != nil && e.Action != AuditRoleChanged {
			t.Errorf("%s: only web entries have an actor", e.Action)
		}
	}
	if len(seen) != len(allowed) {
		t.Errorf("not every action is covered: %v", seen)
	}
	orig := RecoveryCreatedAudit(t0, 1, 7, t0)
	_ = orig.WithReplaced(true)
	if orig.Detail["replaced"] != "false" {
		t.Error("WithReplaced must not change the original")
	}
}

func TestAuditSources(t *testing.T) {
	if e := InvitationUsedAudit(t0, 1, 2); e.Source != AuditPublic || e.TargetKind != AuditTargetInvitation {
		t.Errorf("%+v", e)
	}
	if e := RecoveryInvalidatedAudit(t0, 1, 2, RecoveryByAccess); e.Source != AuditSystem || e.Detail["reason"] != "access" {
		t.Errorf("%+v", e)
	}
	if e := RoleChangedAudit(t0, AuditConsole, nil, 7, RoleSuperuser, RoleUser, false, false); e.Source != AuditConsole || e.ActorID != nil {
		t.Errorf("%+v", e)
	}
}
