package handler

import (
	"context"
	"fmt"
	"maps"
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

// fallbackAssetName is the placeholder used in log bodies when an asset can't
// be resolved (deleted, or a nil id), so the line still reads cleanly.
const fallbackAssetName = "an asset"

// assetMark renders a player-authored asset name for log bodies. Names are
// emphasised (wrapped in **…**) rather than quoted so they stand out from the
// surrounding prose without quote clutter; the chat feed renders this markup as
// italic (bold is disabled app-wide — see ChatPanel.renderLogBody / app.css).
// Marginalia, rumor, secret, and free-text choices stay quoted — emphasis is
// reserved for names.
func assetMark(name string) string {
	return "**" + name + "**"
}

// assetDisplayName resolves an asset id to a marked (bolded) name for log
// bodies, falling back to fallbackAssetName on any lookup failure.
func assetDisplayName(ctx context.Context, q *dbgen.Queries, assetID int64) string {
	if a, err := q.GetAssetByID(ctx, assetID); err == nil {
		return assetMark(a.Name)
	}
	return fallbackAssetName
}

// notesSuffix renders a plan's free-text preparation notes as a trailing clause
// for a PreparedDescriber descriptor (so naming the structured target doesn't
// drop the preparer's flavour text), or "" when the notes are blank.
func notesSuffix(plan dbgen.Plan) string {
	if plan.PreparationNotes == nil {
		return ""
	}
	if n := strings.TrimSpace(*plan.PreparationNotes); n != "" {
		return fmt.Sprintf(" — %q", n)
	}
	return ""
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
	body := fmt.Sprintf("%s prepared %s: %s", preparer, planLabel(plan.PlanType), notes)
	// A handler may supply a custom descriptor (e.g. Spread Rumors names the
	// target asset and hides a kept-secret rumor's text).
	if h, ok := GetHandler(plan.PlanType); ok {
		if describer, ok := h.(PreparedDescriber); ok {
			resData := loadResolutionData(plan.ResolutionData)
			if descriptor, ok := describer.PreparedDescriptor(ctx, q, plan, &resData); ok {
				body = fmt.Sprintf("%s %s", preparer, descriptor)
			}
		}
	}
	EmitSystemPost(ctx, q, manager, plan.GameID, "plan.prepared",
		model.SeverityImportant, body,
		plan.RowNumber, &planID, nil,
		map[string]any{"plan_type": string(plan.PlanType), "preparer_id": plan.PreparerID})
}

// EmitPlanResolving writes the system post marking the moment a plan flips from
// pending to resolving. It mirrors EmitPlanResolved's make/mar tier (IMPORTANT)
// so the start and end of a resolution read at the same weight in the log.
func EmitPlanResolving(ctx context.Context, q *dbgen.Queries, manager *hub.Manager, plan dbgen.Plan) {
	planID := plan.ID
	EmitSystemPost(ctx, q, manager, plan.GameID, "plan.resolving",
		model.SeverityImportant,
		fmt.Sprintf("%s is resolving.", planLabel(plan.PlanType)),
		plan.RowNumber, &planID, nil,
		map[string]any{"plan_id": plan.ID})
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
	// A handler may supply a custom resolution line (e.g. Host Festivity reads
	// "The festivity drew to a close." rather than the tautological
	// "Host Festivity succeeded."). The code/severity stay outcome-derived.
	if h, ok := GetHandler(plan.PlanType); ok {
		if describer, ok := h.(ResolvedDescriber); ok {
			if custom, ok := describer.ResolvedDescriptor(ctx, q, plan, result); ok {
				body = custom
			}
		}
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
		// Only occupied ranks appear; dummy/filler slots are omitted, so the
		// rank numbers may skip (e.g. "2 Sam · 4 Charlie").
		if len(cat.Final) > 0 {
			var ranks []string
			for _, st := range cat.Final {
				ranks = append(ranks, fmt.Sprintf("%d %s", st.Rank, st.Name))
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
		fmt.Sprintf("%s destroyed.", assetMark(asset.Name)),
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
		fmt.Sprintf("%s leveraged.", assetMark(asset.Name)),
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
		target = assetMark(mc.Name)
	}
	source := "banked die"
	if assetName != nil {
		source = assetMark(*assetName)
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

// EmitDifficultyVoteResolved writes the Default chat entry that records the
// outcome of a difficulty vote. The ballot is hidden during voting but
// revealed simultaneously once the last vote lands (the "3, 2, 1" reveal in
// the rules), so it's public by the time this fires. Votes follow the rules'
// convention: −1 per yea (lower the difficulty), +1 per nay (raise it).
//
// adjusted is the post-vote difficulty (already clamped to 1–6 by the caller).
func EmitDifficultyVoteResolved(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	roll *dbgen.DiceRoll,
	votes []dbgen.DifficultyVote,
	adjusted int16,
) {
	var lowered, raised int
	ballot := make([]map[string]any, 0, len(votes))
	for _, v := range votes {
		if v.Vote < 0 {
			lowered++
		} else {
			raised++
		}
		ballot = append(ballot, map[string]any{"player_id": v.PlayerID, "vote": v.Vote})
	}
	body := fmt.Sprintf("Difficulty vote: %d to lower, %d to raise — difficulty %d → %d",
		lowered, raised, roll.Difficulty, adjusted)
	EmitSystemPost(ctx, q, manager, roll.GameID, "roll.vote_resolved",
		model.SeverityDefault, body,
		roll.RowNumber, roll.PlanID, nil,
		map[string]any{
			"roll_id":             roll.ID,
			"base_difficulty":     roll.Difficulty,
			"adjusted_difficulty": adjusted,
			"lowered":             lowered,
			"raised":              raised,
			"ballot":              ballot,
		})
}

// EmitRollResolved writes the Important chat entry recording a dice roll's
// outcome: the result (distinct uncancelled faces) against the effective
// difficulty, and whether that made or marred. For a plan roll this sits one
// tier below the plan.resolved.* post — it's the dice-level detail behind the
// plan outcome — and is the only log entry for an in-scene roll with no plan.
func EmitRollResolved(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	roll *dbgen.DiceRoll,
	result int16,
	outcome string,
	effectiveDifficulty int16,
) {
	actor := playerDisplayName(ctx, q, roll.ActorID)
	verdict := "Mar"
	if outcome == makeOutcome {
		verdict = "Make"
	}
	body := fmt.Sprintf("%s rolled %d vs difficulty %d → %s",
		actor, result, effectiveDifficulty, verdict)
	EmitSystemPost(ctx, q, manager, roll.GameID, "roll.resolved",
		model.SeverityImportant, body,
		roll.RowNumber, roll.PlanID, nil,
		map[string]any{
			"roll_id":              roll.ID,
			"result":               result,
			"outcome":              outcome,
			"effective_difficulty": effectiveDifficulty,
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
		fmt.Sprintf("%s refreshed.", assetMark(asset.Name)),
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
// rowNumber is nullable: the prologue creates assets before any public-record
// row exists, and scene_posts.row_number is NULL for that case. A non-nil row
// that doesn't exist would fail the (game_id, row_number) FK and silently drop
// the post — so prologue callers pass nil, main-event callers pass the row.
func EmitAssetCreated(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	asset dbgen.Asset,
	marginalia []dbgen.Marginalium,
	rowNumber *int16,
) {
	creator := playerDisplayName(ctx, q, asset.CreatorID)
	body := fmt.Sprintf("%s created the %s %s", creator, asset.AssetType, assetMark(asset.Name))
	if len(marginalia) > 0 {
		quoted := make([]string, 0, len(marginalia))
		for _, m := range marginalia {
			quoted = append(quoted, fmt.Sprintf("%q", m.Text))
		}
		body += fmt.Sprintf(" with marginalia: %s", strings.Join(quoted, ", "))
	}
	EmitSystemPost(ctx, q, manager, gameID, "asset.created",
		model.SeverityMinor, body,
		rowNumber, nil, nil,
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
		fmt.Sprintf("%s renamed %s to %s", actor, assetMark(oldName), assetMark(newName)),
		&rowNumber, nil, nil,
		map[string]any{"asset_id": asset.ID, "editor_id": actorID})
}

// EmitMarginaliaAdded writes the Minor marginalia.added post, carrying the new
// text and the asset it's on.
// rowNumber is nullable for the prologue case — see EmitAssetCreated.
func EmitMarginaliaAdded(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	asset dbgen.Asset,
	m dbgen.Marginalium,
	actorID int64,
	rowNumber *int16,
) {
	actor := playerDisplayName(ctx, q, actorID)
	EmitSystemPost(ctx, q, manager, gameID, "marginalia.added",
		model.SeverityMinor,
		fmt.Sprintf("%s added marginalia %q to %s", actor, m.Text, assetMark(asset.Name)),
		rowNumber, nil, nil,
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
		fmt.Sprintf("%s edited marginalia on %s to %q", actor, assetMark(asset.Name), newText),
		&rowNumber, nil, nil,
		map[string]any{"asset_id": asset.ID, "marginalia_id": m.ID, "position": m.Position, "editor_id": actorID})
}

// EmitMarginaliaTorn writes the Default marginalia.torn post. Tearing is often
// hostile, so the body names whose marginalia was torn as well as who tore it,
// quotes the torn text, and — unless the tear destroyed the asset — invites the
// owner to narrate how the asset has changed. Pass destroyed so the prompt is
// suppressed when nothing remains to re-describe.
func EmitMarginaliaTorn(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	asset dbgen.Asset,
	m dbgen.Marginalium,
	actorID int64,
	destroyed bool,
	rowNumber int16,
) {
	actor := playerDisplayName(ctx, q, actorID)
	var body string
	if actorID == asset.OwnerID {
		body = fmt.Sprintf("%s tore their own marginalia %q on %s", actor, m.Text, assetMark(asset.Name))
	} else {
		owner := playerDisplayName(ctx, q, asset.OwnerID)
		body = fmt.Sprintf("%s tore %s's marginalia %q on %s", actor, owner, m.Text, assetMark(asset.Name))
	}
	body += brokenAssetPrompt(ctx, q, asset.OwnerID, destroyed)
	EmitSystemPost(ctx, q, manager, gameID, "marginalia.torn",
		model.SeverityDefault, body,
		&rowNumber, nil, nil,
		map[string]any{"asset_id": asset.ID, "marginalia_id": m.ID, "position": m.Position, "torn_by_id": actorID})
}

// EmitAssetTaken writes the Default asset.taken post for an ownership transfer.
// rowNumber is nullable for the prologue case — see EmitAssetCreated.
func EmitAssetTaken(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	asset dbgen.Asset,
	oldOwnerID, newOwnerID int64,
	rowNumber *int16,
) {
	taker := playerDisplayName(ctx, q, newOwnerID)
	from := playerDisplayName(ctx, q, oldOwnerID)
	EmitSystemPost(ctx, q, manager, gameID, "asset.taken",
		model.SeverityDefault,
		fmt.Sprintf("%s took %s from %s", taker, assetMark(asset.Name), from),
		rowNumber, nil, nil,
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
	body := fmt.Sprintf("%s stepped %s down as main character", actor, assetMark(asset.Name))
	if isMainCharacter {
		body = fmt.Sprintf("%s named %s as their main character", actor, assetMark(asset.Name))
	}
	EmitSystemPost(ctx, q, manager, gameID, "asset.main_character",
		model.SeverityDefault, body,
		&rowNumber, nil, nil,
		map[string]any{"asset_id": asset.ID, "is_main_character": isMainCharacter, "actor_id": actorID})
}

// EmitMainCharacterConscripted writes the post for the "no peers left" escape
// hatch: a player who lost their main character and had no peer to promote
// conscripted a brand new one, at the cost of all their assets becoming
// leveraged. Default severity — it changes board state the table should see.
func EmitMainCharacterConscripted(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	asset dbgen.Asset,
	actorID int64,
	rowNumber int16,
) {
	actor := playerDisplayName(ctx, q, actorID)
	body := fmt.Sprintf(
		"%s had no peers left and conscripted %s as their new main character — all of their assets are now leveraged",
		actor, assetMark(asset.Name))
	EmitSystemPost(ctx, q, manager, gameID, "asset.main_character",
		model.SeverityDefault, body,
		&rowNumber, nil, nil,
		map[string]any{"asset_id": asset.ID, "is_main_character": true, "actor_id": actorID, "conscripted": true})
}

// ── Prologue (opening setup) ───────────────────────────────────────────────────
//
// The prologue builds the starting board. Asset creation, title marginalia, and
// card takes reuse the canonical asset.* / marginalia.* posts (so the opening
// pieces appear in the log identically to mid-game state — closing the raw
// CreateAsset bypass). Laws/rumors and the per-track opening rankings get the
// dedicated emitters below. Phase boundaries ("Prologue begins" / "Main event
// begins") are already emitted by broadcastPhaseChange.

// EmitLawEnacted writes the Default law.enacted post for a law written into the
// public record (its mid-game edit is law.edited).
// The prologue has no public-record row yet (rows 1–13 are created at
// main-event start), and scene_posts.row_number is nullable for exactly this
// "prologue / lobby" case — so these prologue emitters anchor a nil row. A
// non-nil row that doesn't exist would silently fail the (game_id, row_number)
// foreign key and drop the post.
func EmitLawEnacted(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	law dbgen.Law,
) {
	body := fmt.Sprintf("Law enacted: %q", law.Text)
	if law.SignatoryID != nil {
		body = fmt.Sprintf("%s enacted a law: %q", playerDisplayName(ctx, q, *law.SignatoryID), law.Text)
	}
	EmitSystemPost(ctx, q, manager, gameID, "law.enacted",
		model.SeverityDefault, body,
		nil, nil, nil,
		map[string]any{"law_id": law.ID, "signatory_id": law.SignatoryID})
}

// EmitRumorCreated writes the Default rumor.created post for a rumor written
// into the public record (its mid-game edit is rumor.edited).
func EmitRumorCreated(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	rumor dbgen.Rumor,
) {
	body := fmt.Sprintf("Rumor spread from unknown source: %q", rumor.Text)
	if rumor.SourcePlayerID != nil {
		body = fmt.Sprintf("%s spread a rumor: %q", playerDisplayName(ctx, q, *rumor.SourcePlayerID), rumor.Text)
	}
	EmitSystemPost(ctx, q, manager, gameID, "rumor.created",
		model.SeverityDefault, body,
		nil, nil, nil,
		map[string]any{"rumor_id": rumor.ID, "source_player_id": rumor.SourcePlayerID})
}

// EmitPrologueTrackRanked writes the Important prologue.track_ranked post fixing
// a track's opening standing. It reads back the persisted rankings so the body
// reflects the final order including dummy slots (rendered "—"). Emitted once
// per track, after the track is fully resolved (set-asides placed, if any).
func EmitPrologueTrackRanked(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	track string,
) {
	cat := modelCategoryForTrack(track)
	rankings, err := q.ListRankingsByGame(ctx, gameID)
	if err != nil {
		return
	}
	names := make(map[int16]string)
	for _, rk := range rankings {
		if rk.Category != cat {
			continue
		}
		if rk.PlayerID == nil {
			names[rk.Rank] = "—"
		} else {
			names[rk.Rank] = playerDisplayName(ctx, q, *rk.PlayerID)
		}
	}
	parts := make([]string, 0, 5)
	for r := int16(1); r <= 5; r++ {
		if n, ok := names[r]; ok {
			parts = append(parts, fmt.Sprintf("%d %s", r, n))
		}
	}
	EmitSystemPost(ctx, q, manager, gameID, "prologue.track_ranked",
		model.SeverityImportant,
		fmt.Sprintf("Opening %s ranks: %s", shakeUpCategoryTitle(track), strings.Join(parts, " · ")),
		nil, nil, nil,
		map[string]any{"track": track})
}

// ── Shake-Up (the endgame climax) ──────────────────────────────────────────────
//
// The Shake-Up runs three category passes (esteem → knowledge → power), each a
// rolling step then a spending step. None of it was logged before; these
// emitters give the climax a full reconstructable trail. Structural events are
// Boundary anchors; a player's roll is Minor (routine gather); announcing/
// adjusting a spend is the bidding chatter (Default / Minor); the committed
// spend is the Important outcome and carries the concrete effect in its body
// and structured system_data (rather than re-firing the generic asset.* posts,
// which would drop the shake-up framing and the token cost).

// shakeUpCategoryTitle capitalizes a shake-up category string ("esteem" →
// "Esteem") for display, reusing categoryTitle's logic.
func shakeUpCategoryTitle(category string) string {
	return categoryTitle(model.RankingCategory(category))
}

// EmitShakeUpBegin writes the Boundary post announcing the start of the
// Shake-Up's first (esteem) pass.
func EmitShakeUpBegin(ctx context.Context, q *dbgen.Queries, manager *hub.Manager, gameID int64, category string) {
	EmitSystemPost(ctx, q, manager, gameID, "shake_up.begin",
		model.SeverityBoundary,
		fmt.Sprintf("⚖ The Shake-Up begins — %s", shakeUpCategoryTitle(category)),
		nil, nil, nil,
		map[string]any{"category": category})
}

// EmitShakeUpCategory writes the Boundary post marking the move to a new
// category pass within the Shake-Up.
func EmitShakeUpCategory(ctx context.Context, q *dbgen.Queries, manager *hub.Manager, gameID int64, category string) {
	EmitSystemPost(ctx, q, manager, gameID, "shake_up.category",
		model.SeverityBoundary,
		fmt.Sprintf("⚖ Shake-Up — %s", shakeUpCategoryTitle(category)),
		nil, nil, nil,
		map[string]any{"category": category})
}

// EmitShakeUpEnded writes the Boundary post that closes the Shake-Up (and with
// it the game) after the power pass completes.
func EmitShakeUpEnded(ctx context.Context, q *dbgen.Queries, manager *hub.Manager, gameID int64) {
	EmitSystemPost(ctx, q, manager, gameID, "shake_up.ended",
		model.SeverityBoundary,
		"⚖ The Shake-Up is complete — the game has ended.",
		nil, nil, nil, nil)
}

// EmitShakeUpRolled writes the Minor post for one player's category roll, which
// gathers them that many spending tokens. total is their resulting pool.
func EmitShakeUpRolled(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID, playerID int64,
	result, total int16,
	category string,
) {
	name := playerDisplayName(ctx, q, playerID)
	EmitSystemPost(ctx, q, manager, gameID, "shake_up.rolled",
		model.SeverityMinor,
		fmt.Sprintf("%s rolled %d — gains %d %s token(s) (pool: %d)",
			name, result, result, shakeUpCategoryTitle(category), total),
		nil, nil, nil,
		map[string]any{"player_id": playerID, "result": result, "total": total, "category": category})
}

// shakeUpOptionPhrase turns an option's static Description ("Take a peer
// asset.") into an inline phrase ("Take a peer asset") for log bodies.
func shakeUpOptionPhrase(description string) string {
	return strings.TrimSuffix(strings.TrimSpace(description), ".")
}

// EmitShakeUpAnnounced writes the Default post for an opened (announced) spend.
// optionPhrase is the human label; targetName is the targeted asset's name (or
// "" for option types with no asset target).
func EmitShakeUpAnnounced(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	spend dbgen.ShakeUpSpend,
	optionPhrase, targetName string,
) {
	name := playerDisplayName(ctx, q, spend.PlayerID)
	body := fmt.Sprintf("%s announces a Shake-Up spend: %s", name, optionPhrase)
	if targetName != "" {
		body += fmt.Sprintf(" — targeting %s", assetMark(targetName))
	}
	EmitSystemPost(ctx, q, manager, gameID, "shake_up.spend_announced",
		model.SeverityDefault, body,
		nil, nil, nil,
		map[string]any{"spend_id": spend.ID, "player_id": spend.PlayerID, "option_key": spend.OptionKey})
}

// EmitShakeUpAdjusted writes the Minor post for a cost-adjustment bid against an
// open spend.
func EmitShakeUpAdjusted(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	spend dbgen.ShakeUpSpend,
	adjusterID int64,
	adjustment int16,
	optionPhrase string,
) {
	adjuster := playerDisplayName(ctx, q, adjusterID)
	spender := playerDisplayName(ctx, q, spend.PlayerID)
	EmitSystemPost(ctx, q, manager, gameID, "shake_up.adjusted",
		model.SeverityMinor,
		fmt.Sprintf("%s adjusts %s's cost %+d (%s)", adjuster, spender, adjustment, optionPhrase),
		nil, nil, nil,
		map[string]any{"spend_id": spend.ID, "adjuster_id": adjusterID, "adjustment": adjustment})
}

// EmitShakeUpCommitted writes the Important outcome post for a committed spend.
// The effect appliers build the descriptive body (which names the concrete
// change and the final token cost) and pass effect-specific fields in extra;
// the base spend fields are merged in here.
func EmitShakeUpCommitted(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	spend *dbgen.ShakeUpSpend,
	finalCost int16,
	body string,
	extra map[string]any,
) {
	data := map[string]any{
		"spend_id":   spend.ID,
		"player_id":  spend.PlayerID,
		"option_key": spend.OptionKey,
		"final_cost": finalCost,
	}
	maps.Copy(data, extra)
	EmitSystemPost(ctx, q, manager, gameID, "shake_up.committed",
		model.SeverityImportant, body,
		nil, nil, nil, data)
}
