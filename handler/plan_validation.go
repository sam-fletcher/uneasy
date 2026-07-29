package handler

// handler/plan_validation.go — Plan-preparation validation. validatePlanPreparation
// runs the common pre-flight checks (phase, notes, esteem lockout, eligibility,
// target-row bounds, endgame overflow, peers) and delegates plan-specific
// validation to the registered handler. validateExchangeCourtiersPlan lives here
// rather than in plan_exchange_courtiers.go to avoid a package-internal cycle.

import (
	"context"
	"net/http"
	"strings"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// validateExchangeCourtiersPlan is kept here because it's called from
// ecHandler.ValidatePreparation (in plan_exchange_courtiers.go) and both live
// in package handler. Moving it to the EC file avoids a circular dependency.
func validateExchangeCourtiersPlan(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
	targetPlayerID *int64,
	targetAssetID *int64,
) string {
	if targetPlayerID == nil || targetAssetID == nil {
		return "exchange_courtiers requires target_player_id and target_asset_id"
	}

	asset, err := q.GetAssetByID(ctx, *targetAssetID)
	if err != nil {
		return "target asset not found"
	}
	if asset.OwnerID != *targetPlayerID {
		return "target asset does not belong to target player"
	}
	if asset.AssetType != model.AssetPeer {
		return "exchange_courtiers target must be a peer asset"
	}

	targetHasPeers, err := playerHasPeers(ctx, q, gameID, *targetPlayerID)
	if err != nil {
		return "could not check target peer assets"
	}
	if !targetHasPeers {
		return "target player has no peers"
	}

	return ""
}

// ── preparePlanValidation ─────────────────────────────────────────────────────

type preparePlanValidation struct {
	Status int
	ErrMsg string
	// TargetRow is nil when the plan defers its row to a post-prep
	// simultaneous reveal (Make War, Clandestinely Liaise). For every other
	// plan it holds the row the plan will sit on at creation time.
	TargetRow *int16
	Meta      PlanMetadata
	// FinaleBonus marks a preparation that overflowed row 13 and was clamped
	// onto it under Explosive Finale — the player's one bonus plan. Persisted
	// as plans.is_finale_bonus, which is the slot accounting.
	FinaleBonus           bool
	EndgameChoiceRequired bool // overflow detected with no ending_mode set
}

// validatePlanPreparation performs all common checks for plan preparation
// and delegates plan-specific validation to the registered handler.
//
//nolint:funlen // sequential validation steps; splitting obscures the order
func validatePlanPreparation(
	ctx context.Context,
	q *dbgen.Queries,
	game *dbgen.Game,
	player *dbgen.Player,
	planType model.PlanType,
	targetPlayerID *int64,
	targetAssetID *int64,
	targetPlanID *int64,
	peerCount int16,
	enemyPlayerIDs []int64,
	preparerPeerID *int64,
	partnerPeerID *int64,
	notes string,
) preparePlanValidation {
	// Check game phase.
	if game.Phase != model.PhaseMainEvent {
		return preparePlanValidation{
			Status: http.StatusConflict,
			ErrMsg: "game is not in the main event phase",
		}
	}

	// The endgame vote pauses the row 7 → 8 advance, and no plan may be prepared
	// while it is up — not just an overflowing one. Belt and braces behind the
	// row-state gate: that gate means nobody holds a turn, and preparation only
	// happens inside a turn, but focus HAS already passed by the time the vote
	// opens, so a direct API call could otherwise slip a plan in behind it (and
	// it would land relative to row 7, not row 8).
	//
	// The guard keys off the flag, not a row number: current_row is still 7
	// throughout the vote, so it cannot distinguish "row 7 is being played" from
	// "row 7 is over and the vote is up".
	if game.EndingVoteOpen {
		return preparePlanValidation{
			Status: http.StatusConflict,
			ErrMsg: "the table is voting on how the game ends — no plans can be prepared until it settles",
		}
	}

	// Preparation notes are required for every plan — they're the only
	// fiction-side trace some plans leave on the public record, and the
	// system-post log includes them verbatim. Enforced centrally here so
	// handlers don't each repeat the check.
	if strings.TrimSpace(notes) == "" {
		return preparePlanValidation{
			Status: http.StatusBadRequest,
			ErrMsg: "preparation notes are required",
		}
	}

	// Resolve handler from registry.
	h, supported := GetHandler(planType)
	if !supported {
		return preparePlanValidation{
			Status: http.StatusBadRequest,
			ErrMsg: "unsupported plan type",
		}
	}
	meta := h.Metadata()

	// Check esteem lockout (SP mar option b "censured") before eligibility.
	// Any esteem-category plan is blocked while a lockout is active.
	if meta.Category == model.CategoryEsteem {
		locked, lockErr := hasEsteemLockout(ctx, q, game.ID, player.ID)
		if lockErr == nil && locked {
			return preparePlanValidation{
				Status: http.StatusForbidden,
				ErrMsg: "esteem lockout: your next plan must be a non-esteem plan (Spread Propaganda mar censured)",
			}
		}
	}

	// Check eligibility.
	eligible, reason, err := checkPlanEligible(
		ctx, q, game.ID, player.ID, game.CurrentRow, planType, meta.Category)
	if err != nil {
		return preparePlanValidation{
			Status: http.StatusInternalServerError,
			ErrMsg: "could not check eligibility",
		}
	}
	if !eligible {
		return preparePlanValidation{
			Status: http.StatusForbidden,
			ErrMsg: reason,
		}
	}

	// Compute target row.
	// For variable-delay plans (Delay == -1), ValidatePreparation returns the row.
	// For fixed-delay plans, we compute it from the metadata.
	vc := &ValidationContext{
		Q:              q,
		Game:           game,
		Player:         player,
		TargetPlayerID: targetPlayerID,
		TargetAssetID:  targetAssetID,
		TargetPlanID:   targetPlanID,
		PeerCount:      peerCount,
		EnemyPlayerIDs: enemyPlayerIDs,
		PreparerPeerID: preparerPeerID,
		PartnerPeerID:  partnerPeerID,
		Notes:          notes,
	}
	handlerTargetRow, errMsg := h.ValidatePreparation(ctx, vc)
	if errMsg != "" {
		return preparePlanValidation{
			Status: http.StatusBadRequest,
			ErrMsg: errMsg,
		}
	}

	// targetRow is nil when the plan defers its row to a post-prep reveal
	// (Make War, Clandestinely Liaise); the row bound is re-checked when the
	// reveal closes (see reveals.go, applyMakeWarDelayResult), which is also
	// where an Explosive Finale collapse onto row 13 happens for those two —
	// never here, since their overflow isn't known until the faces are in.
	var targetRow *int16
	if meta.Delay == -1 {
		targetRow = handlerTargetRow
	} else {
		row := game.CurrentRow + meta.Delay
		targetRow = &row
	}

	// boundedRow is what the row-13 overflow check below runs against. A
	// still-deferred plan (targetRow == nil) can't be checked directly, but
	// if it has a known MinDelay (Make War, Clandestinely Liaise), even the
	// best-case dice result is checked now rather than letting a
	// guaranteed-futile declaration through only to be silently cancelled
	// once the reveal completes (see applyMakeWarDelayResult).
	boundedRow := targetRow
	if targetRow == nil && meta.MinDelay > 0 {
		row := game.CurrentRow + meta.MinDelay
		boundedRow = &row
	}

	// Target row bounds. Past row 13 means we're hitting the end of the public
	// record, and what happens next is the ending mode's business. The decision
	// lives in planOverflowOutcome, which planIneligibilityReason reads too — the
	// prep grid must grey out exactly what this rejects, and one shared answer is
	// the only way to guarantee that.
	var finaleBonus bool
	if boundedRow != nil && *boundedRow > publicRecordRowCount {
		outcome, oErr := planOverflowOutcome(ctx, q, game, player.ID, targetRow == nil)
		if oErr != nil {
			return preparePlanValidation{
				Status: http.StatusInternalServerError,
				ErrMsg: "could not check the ending mode",
			}
		}
		switch {
		case outcome.ModeUnsettled:
			return preparePlanValidation{
				Status:                http.StatusConflict,
				ErrMsg:                "plan would land past row 13, and the table has not settled how the game ends",
				EndgameChoiceRequired: true,
			}
		case outcome.Reason != "":
			return preparePlanValidation{
				Status: http.StatusConflict,
				ErrMsg: outcome.Reason,
			}
		case outcome.ClampToFinalRow:
			// Explosive Finale, slot free: the plan piles onto the final row
			// instead of its own, and this is the player's one bonus plan.
			row := int16(publicRecordRowCount)
			targetRow = &row
			finaleBonus = outcome.FinaleBonus
		}
	}

	// Check preparer has peers.
	hasPeers, err := playerHasPeers(ctx, q, game.ID, player.ID)
	if err != nil {
		return preparePlanValidation{
			Status: http.StatusInternalServerError,
			ErrMsg: "could not check peer assets",
		}
	}
	if !hasPeers {
		return preparePlanValidation{
			Status: http.StatusForbidden,
			ErrMsg: "you have no peers — a player without peers cannot prepare plans",
		}
	}

	return preparePlanValidation{
		Status:      http.StatusOK,
		TargetRow:   targetRow,
		Meta:        meta,
		FinaleBonus: finaleBonus,
	}
}
