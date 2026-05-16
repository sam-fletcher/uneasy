package handler

import (
	"context"
	"fmt"
	"strings"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// system_posts.go — convenience emitters for the unified chat log.
//
// Each helper wraps EmitSystemPost (in posts.go) with the right
// system_code, severity, and id anchors for one event class. Call them
// alongside the existing broadcastEvent() at canonical emission sites
// so the chat feed and the WebSocket stream stay in lockstep.
//
// All helpers are best-effort: a failed post never rolls back the
// underlying state change. See PUBLIC_RECORD_SIDEBAR_SPEC.md, Part 2.

// planLabel turns "make_demands" into "Make Demands". Used for the
// human-readable body of plan.* system posts.
func planLabel(t model.PlanType) string {
	parts := strings.Split(string(t), "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// playerDisplayName resolves a player ID to its display name. Falls back
// to "Player N" if the lookup fails so the log post still gets emitted.
func playerDisplayName(ctx context.Context, q *dbgen.Queries, playerID int64) string {
	p, err := q.GetPlayerByID(ctx, playerID)
	if err != nil {
		return fmt.Sprintf("Player %d", playerID)
	}
	return p.DisplayName
}

// EmitPlanPrepared writes the boundary post for plan.prepared. The Public
// Record sidebar's plan-tap gesture jumps to this post.
func EmitPlanPrepared(ctx context.Context, q *dbgen.Queries, manager *hub.Manager, plan dbgen.Plan) {
	// plan.RowNumber is nullable while a variable-delay plan is waiting on
	// its delay reveal; the row-anchored post is meaningless until a row
	// exists, so EmitPlanPrepared is called a second time after the reveal
	// resolves (see reveals.go).
	planID := plan.ID
	preparer := playerDisplayName(ctx, q, plan.PreparerID)
	EmitSystemPost(ctx, q, manager, plan.GameID, "plan.prepared",
		model.SeverityBoundary,
		fmt.Sprintf("%s prepared %s.", preparer, planLabel(plan.PlanType)),
		plan.RowNumber, &planID, nil,
		map[string]any{"plan_type": string(plan.PlanType), "preparer_id": plan.PreparerID})
}

// EmitPlanResolved writes the system post for a plan resolution. result
// is "make", "mar", or "cancelled" — matching EventPlanResolved's Result
// field. Cancelled plans get DEFAULT severity; make/mar get IMPORTANT.
func EmitPlanResolved(ctx context.Context, q *dbgen.Queries, manager *hub.Manager, plan dbgen.Plan, result string) {
	planID := plan.ID
	var code, body string
	severity := model.SeverityImportant
	switch result {
	case "make":
		code = "plan.resolved.make"
		body = fmt.Sprintf("%s succeeded.", planLabel(plan.PlanType))
	case "mar":
		code = "plan.resolved.mar"
		body = fmt.Sprintf("%s marred.", planLabel(plan.PlanType))
	case "cancelled":
		code = "plan.cancelled"
		body = fmt.Sprintf("%s cancelled.", planLabel(plan.PlanType))
		severity = model.SeverityDefault
	default:
		code = "plan.resolved"
		body = fmt.Sprintf("%s resolved (%s).", planLabel(plan.PlanType), result)
	}
	EmitSystemPost(ctx, q, manager, plan.GameID, code, severity, body,
		plan.RowNumber, &planID, nil,
		map[string]any{"plan_id": plan.ID, "result": result})
}

// EmitRankingUpdated writes the system post for the algorithmic ranking
// update applied when crossing an engrailed line (after rows 4/8/12).
// Other ranking changes (shake-up, prologue) are not in scope for v1.
func EmitRankingUpdated(ctx context.Context, q *dbgen.Queries, manager *hub.Manager, gameID int64, rowNumber int16) {
	EmitSystemPost(ctx, q, manager, gameID, "ranking.updated",
		model.SeverityImportant,
		"Ranking update applied.",
		&rowNumber, nil, nil,
		map[string]any{"row_number": rowNumber})
}

// EmitAssetDestroyed / Leveraged / Refreshed write the corresponding
// asset.* system post. rowNumber is the row at which the change happens
// (typically game.CurrentRow at the call site).
func EmitAssetDestroyed(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	asset dbgen.Asset,
	rowNumber int16,
) {
	EmitSystemPost(ctx, q, manager, gameID, "asset.destroyed",
		model.SeverityDefault,
		fmt.Sprintf("%s destroyed.", asset.Name),
		&rowNumber, nil, nil,
		map[string]any{"asset_id": asset.ID})
}

func EmitAssetLeveraged(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	asset dbgen.Asset,
	rowNumber int16,
) {
	EmitSystemPost(ctx, q, manager, gameID, "asset.leveraged",
		model.SeverityMinor,
		fmt.Sprintf("%s leveraged.", asset.Name),
		&rowNumber, nil, nil,
		map[string]any{"asset_id": asset.ID})
}

func EmitAssetRefreshed(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	asset dbgen.Asset,
	rowNumber int16,
) {
	EmitSystemPost(ctx, q, manager, gameID, "asset.refreshed",
		model.SeverityMinor,
		fmt.Sprintf("%s refreshed.", asset.Name),
		&rowNumber, nil, nil,
		map[string]any{"asset_id": asset.ID})
}
