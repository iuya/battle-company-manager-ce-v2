package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// insertAudit writes one audit_events row inside the caller's transaction, so the entry exists exactly when the change it
// records does. The time is the entry's own (the caller's clock). The detail is the closed list of keys that the core
// constructors fill; it never holds a secret.
func insertAudit(ctx context.Context, tx pgx.Tx, e core.AuditEntry) error {
	detail := e.Detail
	if detail == nil {
		detail = map[string]string{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	var kind *string
	var target *int64
	if e.TargetKind != "" {
		kind, target = &e.TargetKind, &e.TargetID
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (at, source, actor_id, action, subject_user_id, target_kind, target_id, detail)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		e.At, string(e.Source), e.ActorID, string(e.Action), e.SubjectUserID, kind, target, raw)
	return err
}
