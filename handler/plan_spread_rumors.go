package handler

// handler/plan_spread_rumors.go — Spread Rumors plan handler (Phase 3b).
//
// Spread Rumors (esteem, delay 4): The preparer starts a rumor about an asset.
// Difficulty depends on the target:
//   - Target is a main character: 6 - target's esteem rank (target's esteem status)
//   - Any other asset:            preparer's rank on the esteem track
//
// Preparing: target_asset_id required; preparation_notes holds the rumor text.
//
// Make: server creates a rumors row. Then choose N options equal to dice result
// (repeatable):
//   - "break_target"  → tear a marginalia on the target asset
//   - "leverage_target" → leverage the target asset
//   - "take_asset"    → transfer any of the victim's assets (requires the
//                       victim's consent — see request/respond-take-consent)
//   - "hide_source"   → set rumors.source_player_id = NULL; write secret on own asset
//   - "reveal_source" → set rumors.source_player_id = preparer_id
//
// Mar: the target player describes a counter-rumor about the preparer. They
// choose options from the make list, applied against the preparer's assets,
// equal to (difficulty - result). Effects go through extra routes.
//
// Extra routes (all accept either the preparer on a make result, or the
// target-asset owner on a mar result):
//   POST /api/plans/:planId/break-target          {"marginalia_id": M, "asset_id"?: A}
//   POST /api/plans/:planId/hide-source           {"secret_asset_id": N, "secret_text"?: "..."}
//   POST /api/plans/:planId/request-take-consent  {"choices": [...], "result": "...", "take_asset_ids": [A, …]}
//   POST /api/plans/:planId/respond-take-consent  {"agree": true|false}
//
// On mar, asset_id specifies which of the preparer's assets the target player
// is tearing (the counter-rumor applies to preparer assets, not the plan's
// target asset).
//
// "take asset" is consent-gated: unlike break/leverage (which only touch the
// plan's original target asset), a take can claim ANY of the victim's assets —
// the target-asset owner on make, the preparer on mar — and the aggressor may
// take several (one per "take_asset" pick). The aggressor names the specific
// assets up front via request-take-consent; nothing is committed until the
// victim agrees via respond-take-consent. On agree the choices are applied and
// the assets transfer; on disagree nothing happens and the aggressor returns to
// the option picker with "take asset" disabled. See srRequestTakeConsentHandler.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanSpreadRumors, srHandler{})
}

type srHandler struct{}

// Make/mar option keys for Spread Rumors. Defined as constants because several
// are referenced across the handler, its sub-flow routes, and CanComplete.
const (
	srOptBreakTarget = "break_target"
	srOptHideSource  = "hide_source"
)

func (srHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryEsteem, Delay: 4}
}

func (srHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (*int16, string) {
	if v.TargetAssetID == nil {
		return nil, "spread_rumors requires target_asset_id"
	}
	if v.Notes == "" {
		return nil, "spread_rumors requires preparation_notes with the rumor text"
	}
	return nil, ""
}

func (srHandler) ComputeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	if plan.TargetAssetID == nil {
		return 0, errors.New("spread_rumors plan has no target asset")
	}
	asset, err := q.GetAssetByID(ctx, *plan.TargetAssetID)
	if err != nil {
		return 0, fmt.Errorf("could not load target asset: %w", err)
	}

	if asset.IsMainCharacter {
		// Difficulty = target player's esteem STATUS = 6 - rank.
		if asset.OwnerID == 0 {
			return 0, errors.New("main character asset has no owner")
		}
		targetRank, errRank := playerRankInCategory(ctx, q, plan.GameID, asset.OwnerID, model.CategoryEsteem)
		if errRank != nil {
			return 0, fmt.Errorf("could not determine target esteem rank: %w", errRank)
		}
		return gamepkg.SpreadRumorsDifficulty(targetRank, true), nil
	}

	// Difficulty = preparer's esteem rank.
	preparerRank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryEsteem)
	if err != nil {
		return 0, fmt.Errorf("could not determine preparer esteem rank: %w", err)
	}
	return gamepkg.SpreadRumorsDifficulty(preparerRank, false), nil
}

// OnResolve creates the dice roll.
func (srHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return nil, err
	}
	resData := loadResolutionData(plan.ResolutionData)
	difficulty, err := srHandler{}.ComputeDifficulty(ctx, deps.Q, plan, &resData)
	if err != nil {
		return nil, err
	}
	return createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
}

// ApplyChoice creates the rumors row on make and handles "leverage_target" and
// "reveal_source" which are pure DB ops. "break_target", "take_asset", and
// "hide_source" go through extra routes.
func (srHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	choices []string,
	result string,
) error {
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return fmt.Errorf("could not load game: %w", err)
	}

	// Create the rumor row (both make and mar create one; on mar the target
	// player describes a counter-rumor which is narrative — we still create
	// a placeholder rumor from the preparation_notes).
	rumorText := ""
	if plan.PreparationNotes != nil {
		rumorText = *plan.PreparationNotes
	}
	// If the preparer kept the rumor secret, its text lives in the Secret on
	// their asset rather than the (blanked) plan note. A Make now spreads it
	// publicly, so pull the text back out. On a Mar the secret stays hidden —
	// the rumor row is the target's counter-rumor placeholder instead.
	if sr := resData.SpreadRumors; result == makeOutcome && sr != nil && sr.IsSecret && sr.SecretID != nil {
		if secret, sErr := deps.Q.GetSecretByID(ctx, *sr.SecretID); sErr == nil {
			rumorText = secret.Text
		}
	}
	if rumorText == "" {
		rumorText = "(no rumor text)"
	}

	// Count existing rumors for display_order.
	existingRumors, _ := deps.Q.ListRumors(ctx, plan.GameID)
	displayOrder := int16(len(existingRumors))

	// Source attribution drives both the Rumors panel ("Spread by: …") and the
	// chat-log message. On make the preparer is the source — unless they chose
	// "hide_source", in which case the rumor is anonymous from the start (the
	// hide-source sub-flow later records the secret on one of their assets). On
	// mar the counter-rumor is left unattributed.
	hideSource := slices.Contains(choices, srOptHideSource)
	var sourcePlayerID *int64
	if result == makeOutcome && !hideSource {
		sourcePlayerID = &plan.PreparerID
	}

	rumor, err := deps.Q.CreateRumor(ctx, dbgen.CreateRumorParams{
		GameID:         game.ID,
		Text:           rumorText,
		TargetAssetID:  plan.TargetAssetID,
		OriginPlanID:   &plan.ID,
		SourcePlayerID: sourcePlayerID,
		DisplayOrder:   displayOrder,
	})
	if err != nil {
		return fmt.Errorf("could not create rumor: %w", err)
	}
	resData.EnsureSpreadRumors().RumorID = &rumor.ID
	if hideSource {
		resData.EnsureSpreadRumors().SourceHidden = true
	}

	broadcastEvent(deps.Manager, plan.GameID, model.EventRumorCreated, model.RumorCreatedPayload{Rumor: rumor})
	// Chat-log entry. EmitRumorCreated names the source when one is set and
	// stays anonymous otherwise, so a hidden spreader is never named here.
	EmitRumorCreated(ctx, deps.Q, deps.Manager, plan.GameID, rumor)

	// Apply inline choices.
	for _, choice := range choices {
		switch choice {
		case "leverage_target":
			// On mar the counter-rumor would leverage one of the preparer's
			// assets, but the flat choices list carries no asset picker. Treat
			// as narrative-only on mar; leverage the target asset on make.
			if result == marOutcome {
				continue
			}
			if plan.TargetAssetID != nil {
				if err := deps.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
					ID:          *plan.TargetAssetID,
					IsLeveraged: true,
				}); err != nil {
					return fmt.Errorf("could not leverage target asset: %w", err)
				}
				broadcastEvent(
					deps.Manager,
					plan.GameID,
					model.EventAssetLeveraged,
					model.AssetIDPayload{AssetID: *plan.TargetAssetID},
				)
				if asset, aErr := deps.Q.GetAssetByID(ctx, *plan.TargetAssetID); aErr == nil {
					EmitAssetLeveraged(ctx, deps.Q, deps.Manager, plan.GameID, asset, logRow(game))
				}
			}
		case "reveal_source":
			resData.EnsureSpreadRumors().SourceHidden = false
			// Source is already set to preparer above; nothing more needed.
		case srOptHideSource:
			// Handled via the extra route (requires secret text).
			resData.EnsureSpreadRumors().SourceHidden = true
		}
	}

	return nil
}

// CanComplete blocks completion while a take-asset consent request is still
// awaiting the owner's answer (the analogue of Seek Answers' pending-question
// gate — completing here would strand an in-flight asset transfer), and until
// every committed break_target / hide_source pick has been performed or forfeited
// (sr-forfeit-step). The make/mar effects are server-authoritative: the client's
// "all steps done" gate is UX, not the only enforcement.
func (srHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	sr := resData.SpreadRumors
	if sr == nil {
		return nil
	}
	if sr.PendingTakeConsent != nil {
		return errors.New("a take-asset request is awaiting the owner's consent before you can complete")
	}
	return subflowPicksRemaining(resData,
		subflowProgress{srOptBreakTarget, "break-target", sr.BreakTargetDone},
		subflowProgress{srOptHideSource, "hide-source", sr.HideSourceDone},
	)
}

// MaxChoices: make picks equal to the result (repeatable); the mar counter-rumor
// picks equal to (difficulty − result).
func (srHandler) MaxChoices(result string, rollResult, difficulty int16) int {
	if result == makeOutcome {
		return int(rollResult)
	}
	return int(difficulty - rollResult)
}

// PreparedDescriptor customizes the plan.prepared log post. It always names the
// target asset the rumor is about; when the preparer kept the rumor secret it
// names the asset holding the hidden text rather than the text itself (which
// would otherwise leak the secret), otherwise it states the rumor openly.
func (srHandler) PreparedDescriptor(
	ctx context.Context,
	q *dbgen.Queries,
	plan dbgen.Plan,
	resData *ResolutionData,
) (string, bool) {
	about := fallbackAssetName
	if plan.TargetAssetID != nil {
		about = assetDisplayName(ctx, q, *plan.TargetAssetID)
	}
	if sr := resData.SpreadRumors; sr != nil && sr.IsSecret && sr.SecretAssetID != nil {
		keeper := assetDisplayName(ctx, q, *sr.SecretAssetID)
		return fmt.Sprintf(
			"prepared Spread Rumors: there's a rumor brewing about %s, but it's a secret kept by %s.",
			about, keeper), true
	}
	notes := ""
	if plan.PreparationNotes != nil {
		notes = strings.TrimSpace(*plan.PreparationNotes)
	}
	if notes == "" {
		return fmt.Sprintf("prepared Spread Rumors: there's a rumor brewing about %s.", about), true
	}
	return fmt.Sprintf("prepared Spread Rumors: there's a rumor brewing about %s — %q", about, notes), true
}

func (srHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"break-target":         srBreakTargetHandler(deps),
		"hide-source":          srHideSourceHandler(deps),
		"request-take-consent": srRequestTakeConsentHandler(deps),
		"respond-take-consent": srRespondTakeConsentHandler(deps),
		"sr-forfeit-step":      srForfeitStepHandler(deps),
	}
}

// srLog emits a Spread Rumors action-log entry anchored to the plan's row.
func srLog(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, severity int32, body string) {
	planID := plan.ID
	EmitSystemPost(ctx, deps.Q, deps.Manager, plan.GameID, "plan.spread_rumors",
		severity, body, plan.RowNumber, &planID, nil,
		map[string]any{"plan_id": plan.ID})
}

// ResolvingWaitees returns AwaitTakeConsent while a Spread Rumors "take asset"
// choice is waiting on the victim's agree/disagree. ActingPlayerIDs names the
// victim (the asset owner), so the table blocks on them rather than the resolving
// plan's focus player. No pending consent → ride the generic PlanResolving case.
func (srHandler) ResolvingWaitees(_ context.Context, _ *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	resData := loadResolutionData(plan.ResolutionData)
	if sr := resData.SpreadRumors; sr != nil && sr.PendingTakeConsent != nil {
		victim := sr.PendingTakeConsent.VictimID
		return model.RowState{Kind: model.RowStateAwaitTakeConsent, ActingPlayerIDs: []int64{victim}}, true
	}
	return model.RowState{}, false
}
