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
	notes := ""
	if plan.PreparationNotes != nil {
		notes = strings.TrimSpace(*plan.PreparationNotes)
	}
	EmitSystemPost(ctx, q, manager, plan.GameID, "plan.prepared",
		model.SeverityImportant,
		fmt.Sprintf("%s prepared %s: %s", preparer, planLabel(plan.PlanType), notes),
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

// rankingGlyphSymbol maps a rankingMove glyph keyword to its chat symbol.
// "up" performed an up-swap (an adjacent-preparer cancel is two of these that
// undo each other); "top" sits atop the track with no real rival to overtake.
func rankingGlyphSymbol(glyph string) string {
	switch glyph {
	case "up":
		return "↑"
	case "top":
		return "👑"
	default:
		return "?"
	}
}

// categoryTitle capitalizes a ranking category for display ("power" → "Power").
func categoryTitle(cat model.RankingCategory) string {
	s := string(cat)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// EmitRankingUpdated narrates the algorithmic ranking update applied when
// crossing an engrailed line (after rows 4/8/12) into the chat feed. Other
// ranking changes (shake-up, prologue) are not in scope for v1.
//
// It emits, in order:
//   - a Boundary headline that anchors the Public Record rail,
//   - per category (in display order) an Important header,
//   - for each prepared plan a Default numbered line listing its preparers and
//     their movement glyphs (↑ performed an up-swap / 👑 top of the track),
//   - an Important "Standing:" line with the category's resulting rank order,
//   - a Minor sentence explaining the clear when every plan was prepared,
//   - a Default "no preparations" line for an untouched category.
//
// rowNumber is the new row just entered; the engrailed line sits after the
// preceding row, which is what the headline names.
func EmitRankingUpdated(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	rowNumber int16,
	diff *rankingUpdateDiff,
) {
	EmitSystemPost(ctx, q, manager, gameID, "ranking.updated",
		model.SeverityBoundary,
		fmt.Sprintf("⚖ Rankings update — after Row %d", rowNumber-1),
		&rowNumber, nil, nil,
		map[string]any{"row_number": rowNumber, "diff": diff})

	if diff == nil {
		return
	}

	for _, cat := range diff.Categories {
		EmitSystemPost(ctx, q, manager, gameID, "ranking.category",
			model.SeverityImportant, categoryTitle(cat.Category),
			&rowNumber, nil, nil,
			map[string]any{"category": string(cat.Category)})

		if len(cat.Plans) == 0 {
			EmitSystemPost(ctx, q, manager, gameID, "ranking.empty",
				model.SeverityDefault, "No preparations, ranks unchanged.",
				&rowNumber, nil, nil,
				map[string]any{"category": string(cat.Category)})
			continue
		}

		// Each movement, in order
		for i, plan := range cat.Plans {
			var movers []string
			for _, m := range plan.Movers {
				movers = append(movers, fmt.Sprintf("%s %s", m.Name, rankingGlyphSymbol(m.Glyph)))
			}
			body := fmt.Sprintf("%d. %s: %s", i+1, planLabel(plan.PlanType), strings.Join(movers, ", "))
			EmitSystemPost(ctx, q, manager, gameID, "ranking.plan",
				model.SeverityDefault, body,
				&rowNumber, nil, nil,
				map[string]any{"category": string(cat.Category), "plan_type": string(plan.PlanType)})
		}

		// The resulting ranks for the category — the payoff of the update.
		if len(cat.Final) > 0 {
			var ranks []string
			for i, name := range cat.Final {
				ranks = append(ranks, fmt.Sprintf("%d %s", i+1, name))
			}
			EmitSystemPost(ctx, q, manager, gameID, "ranking.standing",
				model.SeverityImportant,
				fmt.Sprintf("New Ranks: %s", strings.Join(ranks, " · ")),
				&rowNumber, nil, nil,
				map[string]any{"category": string(cat.Category), "final": cat.Final})
		}

		if cat.Cleared {
			EmitSystemPost(ctx, q, manager, gameID, "ranking.cleared",
				model.SeverityMinor,
				"All plans were prepared → they're now freely available",
				&rowNumber, nil, nil,
				map[string]any{"category": string(cat.Category)})
		}
	}
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

// EmitRollCommit writes the Minor chat-system entry for a single dice
// commit (asset leverage or banked-die spend). assetName == nil means a
// banked-die commit; the chat substitutes "banked die" in that case.
func EmitRollCommit(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	roll *dbgen.DiceRoll,
	player *dbgen.Player,
	isInterference bool,
	assetName *string,
) {
	playerName := playerDisplayName(ctx, q, player.ID)
	target := "the roll"
	if mc, err := q.GetMainCharacterByOwner(ctx, dbgen.GetMainCharacterByOwnerParams{
		GameID: roll.GameID, OwnerID: roll.ActorID,
	}); err == nil {
		target = mc.Name
	}
	source := "banked die"
	if assetName != nil {
		source = fmt.Sprintf("%q", *assetName)
	}
	verb := "aided"
	if isInterference {
		verb = "interfered with"
	}
	body := fmt.Sprintf("%s's %s %s %s.", playerName, source, verb, target)
	EmitSystemPost(ctx, q, manager, roll.GameID, "roll.commit",
		model.SeverityMinor, body,
		roll.RowNumber, roll.PlanID, nil,
		map[string]any{
			"roll_id":         roll.ID,
			"player_id":       player.ID,
			"is_interference": isInterference,
		})
}

// EmitRollSkipLeverage writes the Minor chat entry that fires when the
// leverage stage is short-circuited because no participant can add a die.
func EmitRollSkipLeverage(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	roll *dbgen.DiceRoll,
) {
	EmitSystemPost(ctx, q, manager, roll.GameID, "roll.skip_leverage",
		model.SeverityMinor,
		"No dice can be added — rolling immediately.",
		roll.RowNumber, roll.PlanID, nil,
		map[string]any{"roll_id": roll.ID})
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

// ── Asset & marginalia lifecycle (add / edit / tear / take / main-character) ────
//
// These emitters back the goal of a chat log from which the full game state can
// be reconstructed. Severity follows the scale in model/severity.go:
//   - adds (asset, marginalia) → Minor: a new piece of state appeared.
//   - edits (rename, marginalia text) → Trace: a tweak to existing state.
//   - tear / take / main-character → Default: notable, often adversarial moves.
//
// Marginalia are a first-class game concept, not throwaway notes, so their text
// is always carried in the body (adds and edits alike) for reconstruction.

// EmitAssetCreated writes the Minor asset.created post. When the asset is born
// with marginalia (CreateAsset accepts an initial set), they're folded into the
// same line so the creation reads as a single event rather than a burst of
// per-marginalia adds.
func EmitAssetCreated(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	asset dbgen.Asset,
	marginalia []dbgen.Marginalium,
	rowNumber int16,
) {
	creator := playerDisplayName(ctx, q, asset.CreatorID)
	body := fmt.Sprintf("%s created the %s %q", creator, asset.AssetType, asset.Name)
	if len(marginalia) > 0 {
		quoted := make([]string, 0, len(marginalia))
		for _, m := range marginalia {
			quoted = append(quoted, fmt.Sprintf("%q", m.Text))
		}
		body += fmt.Sprintf(" with marginalia: %s", strings.Join(quoted, ", "))
	}
	EmitSystemPost(ctx, q, manager, gameID, "asset.created",
		model.SeverityMinor, body,
		&rowNumber, nil, nil,
		map[string]any{"asset_id": asset.ID, "creator_id": asset.CreatorID})
}

// EmitAssetRenamed writes the Trace asset.renamed post naming both the old and
// new asset name.
func EmitAssetRenamed(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	asset dbgen.Asset,
	oldName, newName string,
	actorID int64,
	rowNumber int16,
) {
	actor := playerDisplayName(ctx, q, actorID)
	EmitSystemPost(ctx, q, manager, gameID, "asset.renamed",
		model.SeverityTrace,
		fmt.Sprintf("%s renamed %q to %q", actor, oldName, newName),
		&rowNumber, nil, nil,
		map[string]any{"asset_id": asset.ID, "editor_id": actorID})
}

// EmitMarginaliaAdded writes the Minor marginalia.added post, carrying the new
// text and the asset it's on.
func EmitMarginaliaAdded(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	asset dbgen.Asset,
	m dbgen.Marginalium,
	actorID int64,
	rowNumber int16,
) {
	actor := playerDisplayName(ctx, q, actorID)
	EmitSystemPost(ctx, q, manager, gameID, "marginalia.added",
		model.SeverityMinor,
		fmt.Sprintf("%s added marginalia %q to %q", actor, m.Text, asset.Name),
		&rowNumber, nil, nil,
		map[string]any{"asset_id": asset.ID, "marginalia_id": m.ID, "position": m.Position, "author_id": actorID})
}

// EmitMarginaliaEdited writes the Trace marginalia.edited post, carrying the new
// text and the asset it's on.
func EmitMarginaliaEdited(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	asset dbgen.Asset,
	m dbgen.Marginalium,
	newText string,
	actorID int64,
	rowNumber int16,
) {
	actor := playerDisplayName(ctx, q, actorID)
	EmitSystemPost(ctx, q, manager, gameID, "marginalia.edited",
		model.SeverityTrace,
		fmt.Sprintf("%s edited marginalia on %q to %q", actor, asset.Name, newText),
		&rowNumber, nil, nil,
		map[string]any{"asset_id": asset.ID, "marginalia_id": m.ID, "position": m.Position, "editor_id": actorID})
}

// EmitMarginaliaTorn writes the Default marginalia.torn post. Tearing is often
// hostile, so the body names whose marginalia was torn as well as who tore it.
func EmitMarginaliaTorn(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	asset dbgen.Asset,
	m dbgen.Marginalium,
	actorID int64,
	rowNumber int16,
) {
	actor := playerDisplayName(ctx, q, actorID)
	var body string
	if actorID == asset.OwnerID {
		body = fmt.Sprintf("%s tore their own marginalia %q on %q", actor, m.Text, asset.Name)
	} else {
		owner := playerDisplayName(ctx, q, asset.OwnerID)
		body = fmt.Sprintf("%s tore %s's marginalia %q on %q", actor, owner, m.Text, asset.Name)
	}
	EmitSystemPost(ctx, q, manager, gameID, "marginalia.torn",
		model.SeverityDefault, body,
		&rowNumber, nil, nil,
		map[string]any{"asset_id": asset.ID, "marginalia_id": m.ID, "position": m.Position, "torn_by_id": actorID})
}

// EmitAssetTaken writes the Default asset.taken post for an ownership transfer.
func EmitAssetTaken(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	asset dbgen.Asset,
	oldOwnerID, newOwnerID int64,
	rowNumber int16,
) {
	taker := playerDisplayName(ctx, q, newOwnerID)
	from := playerDisplayName(ctx, q, oldOwnerID)
	EmitSystemPost(ctx, q, manager, gameID, "asset.taken",
		model.SeverityDefault,
		fmt.Sprintf("%s took %q from %s", taker, asset.Name, from),
		&rowNumber, nil, nil,
		map[string]any{"asset_id": asset.ID, "old_owner_id": oldOwnerID, "new_owner_id": newOwnerID})
}

// EmitMainCharacterChanged writes the Default asset.main_character post for a
// promotion (isMainCharacter true) or demotion (false).
func EmitMainCharacterChanged(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	asset dbgen.Asset,
	isMainCharacter bool,
	actorID int64,
	rowNumber int16,
) {
	actor := playerDisplayName(ctx, q, actorID)
	body := fmt.Sprintf("%s stepped %q down as main character", actor, asset.Name)
	if isMainCharacter {
		body = fmt.Sprintf("%s named %q as their main character", actor, asset.Name)
	}
	EmitSystemPost(ctx, q, manager, gameID, "asset.main_character",
		model.SeverityDefault, body,
		&rowNumber, nil, nil,
		map[string]any{"asset_id": asset.ID, "is_main_character": isMainCharacter, "actor_id": actorID})
}
