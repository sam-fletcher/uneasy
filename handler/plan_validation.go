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
	TargetRow             *int16
	Meta                  PlanMetadata
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
	eligible, reason, err := checkPlanEligible(ctx, q, game.ID, player.ID, planType, meta.Category)
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
	// reveal closes (see reveals.go, applyMakeWarDelayResult) — unless the
	// bounds check below already collapses it onto row 13 (Explosive Finale).
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

	// Target row bounds. Past row 13 means we're hitting the end of the
	// public record and the table needs to choose an endgame mode.
	if boundedRow != nil && *boundedRow > publicRecordRowCount {
		switch {
		case game.EndingMode == nil:
			return preparePlanValidation{
				Status:                http.StatusConflict,
				ErrMsg:                "plan would land past row 13 — facilitator must choose an endgame mode",
				EndgameChoiceRequired: true,
			}
		case *game.EndingMode == EndingModeSmoothLanding:
			return preparePlanValidation{
				Status: http.StatusConflict,
				ErrMsg: "you simply cannot prepare a plan that would go beyond the last row of the public record. " +
					"Choose a different plan, or don't prepare anything",
			}
		case *game.EndingMode == EndingModeExplosiveFinale:
			// Collapse to row 13 — every plan piles onto the final row.
			row := int16(publicRecordRowCount)
			targetRow = &row
		default:
			return preparePlanValidation{
				Status: http.StatusConflict,
				ErrMsg: "endgame mode " + *game.EndingMode + " does not allow new plans past row 13",
			}
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
		Status:    http.StatusOK,
		TargetRow: targetRow,
		Meta:      meta,
	}
}
