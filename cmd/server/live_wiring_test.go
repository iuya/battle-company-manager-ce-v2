package main

import (
	"context"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The facade keeps every method of the live service the web layer will declare (docs/live-match.md "Go contracts"): the services are
// embedded, so a name two of them share would silently drop out of the method set.
var _ interface {
	Live(ctx context.Context, user core.User, id, companyID int64) (core.LiveCompany, error)
	Apply(ctx context.Context, user core.User, id, matchID, companyID int64, a core.LiveAction) (core.LiveCompany, core.LiveEntry, error)
	Undo(ctx context.Context, user core.User, id, matchID, entryID, companyID int64) (core.LiveCompany, error)
	Log(ctx context.Context, user core.User, id, matchID, before int64, limit int) ([]core.LiveEntry, error)
	EndChecks(ctx context.Context, user core.User, id, matchID int64) (core.EndChecks, error)
	SetCampaignStatus(ctx context.Context, actor core.User, campaignID int64, to core.CampaignStatus, confirm string) error
	InvalidateInvites(ctx context.Context, actor core.User, campaignID int64) (int, error)
	RegenerateInvite(ctx context.Context, actor core.User, campaignID int64) (core.Invite, error)
	SetSideRoles(ctx context.Context, user core.User, id, matchID int64, revision int, roles []string) (core.Match, error)
	SwapSideRoles(ctx context.Context, user core.User, id, matchID int64, revision int) (core.Match, error)
	SecretObjectives(ctx context.Context, user core.User, id, matchID int64) (core.SecretView, error)
	SecretScore(ctx context.Context, user core.User, id, matchID, companyID int64) (int, []core.SecretLine, bool, error)
	Subscribe(ctx context.Context, user core.User, id, matchID int64) (core.LiveStream, error)
} = (*campaignsFacade)(nil)
