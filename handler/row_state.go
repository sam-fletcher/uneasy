package handler

import (
	"context"
	"strings"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// ComputeRowState returns the single authoritative RowState for a game by
// reading the persisted state of plans, scenes, wars, and reveals.
//
// Precedence chain — rulebook step 1 first, then the in-row sequence:
//
//  1. Not main_event              → PhaseNotMainEvent
//  2. Outstanding surrender claim → AwaitSurrenderClaim
//  3. Outstanding battle cost     → AwaitBattleCost            (rulebook step 1)
//  4. Plan resolving              → PlanResolving              (step 2, active)
//  5. Plan pending on current row → PlanPending                (step 2, queued)
//  6. Open delay-reveal plan      → AwaitDelayReveal           (Make War / CL)
//  7. Focus player has a started, not-yet-ended turn-scene → SceneActive (step 4)
//  8. Focus player's turn-scene has ended_at set → PostSceneAction      (step 5)
//  9. Default                     → SceneSetting                         (step 3)
//
// Note on delay reveal vs. battle costs: a Make War plan that just finished
// its reveal puts an active war on a future row (or the current one). Battle
// costs only become due at the START of a row in which a war is active. So
// the two gates don't fight — costs precede a fresh reveal chronologically,
// and the reveal precedes the cost it eventually enables.
func ComputeRowState(ctx context.Context, q *dbgen.Queries, gameID int64) (model.RowState, error) {
	game, err := q.GetGameByID(ctx, gameID)
	if err != nil {
		return model.RowState{}, err
	}
	if game.Phase != model.PhaseMainEvent {
		return model.RowState{Kind: model.RowStatePhaseNotMainEvent}, nil
	}

	// 2. Surrender claim still open. Step 1 of the rulebook — but in
	// practice claims only arise from a prior row's surrender, so they
	// surface here as the highest-priority unresolved gate.
	claims, err := q.ListOpenSurrenderClaimsByGame(ctx, gameID)
	if err != nil {
		return model.RowState{}, err
	}
	if len(claims) > 0 {
		id := claims[0].ID
		return model.RowState{Kind: model.RowStateAwaitSurrenderClaim, ClaimID: &id}, nil
	}

	// 3. Outstanding battle costs on the current row. Rulebook step 1:
	// nothing else may happen until each war participant pays (or peace
	// has been agreed). Sits above plan resolution/preparation.
	outstanding, err := mwOutstandingCostsForGame(ctx, q, gameID, game.CurrentRow)
	if err != nil {
		return model.RowState{}, err
	}
	if warID, ok := firstKey(outstanding); ok {
		return model.RowState{Kind: model.RowStateAwaitBattleCost, WarID: &warID}, nil
	}

	plans, err := q.ListPlansByGame(ctx, gameID)
	if err != nil {
		return model.RowState{}, err
	}

	// 4. Plan currently resolving.
	for i := range plans {
		if plans[i].Status == model.PlanResolving {
			id := plans[i].ID
			return model.RowState{Kind: model.RowStatePlanResolving, PlanID: &id}, nil
		}
	}

	// 5. Plan pending on the current row.
	if top := topPendingPlanOnRow(plans, game.CurrentRow); top != nil {
		id := top.ID
		return model.RowState{Kind: model.RowStatePlanPending, PlanID: &id}, nil
	}

	// 6. Open delay-reveal plan (Make War or Clandestinely Liaise).
	// The kind is the same for both — the client picks the right panel
	// from the plan's type via RowState.PlanID.
	if dr := openDelayRevealPlan(plans); dr != nil {
		id := dr.ID
		return model.RowState{Kind: model.RowStateAwaitDelayReveal, PlanID: &id}, nil
	}

	// 7/8/9. Turn-scene state for the focus player.
	if game.FocusPlayerID == nil {
		// No focus player set yet in main_event — treat as scene_setting
		// so clients render the most permissive empty state.
		return model.RowState{Kind: model.RowStateSceneSetting}, nil
	}
	turnScene, err := q.GetTurnScene(ctx, dbgen.GetTurnSceneParams{
		GameID:        gameID,
		RowNumber:     game.CurrentRow,
		FocusPlayerID: *game.FocusPlayerID,
	})
	if err != nil {
		if isNoRows(err) {
			// 9. No turn-scene yet for this row & focus player.
			return model.RowState{Kind: model.RowStateSceneSetting}, nil
		}
		return model.RowState{}, err
	}
	if !turnScene.EndedAt.Valid {
		// 7. Turn-scene started and still running.
		id := turnScene.ID
		return model.RowState{Kind: model.RowStateSceneActive, SceneID: &id}, nil
	}
	// 8. Turn-scene ended → focus player is in post-scene action step.
	return model.RowState{Kind: model.RowStatePostSceneAction}, nil
}

// topPendingPlanOnRow returns the lowest-row_order pending plan on rowNumber,
// matching plansPendingOnRow in shared.ts. plans is assumed already ordered
// by (row_number, row_order) ascending — ListPlansByGame guarantees this.
func topPendingPlanOnRow(plans []dbgen.Plan, rowNumber int16) *dbgen.Plan {
	for i := range plans {
		p := &plans[i]
		if p.Status != model.PlanPending {
			continue
		}
		if p.RowNumber == nil || *p.RowNumber != rowNumber {
			continue
		}
		return p
	}
	return nil
}

// hasOpenDelayReveal is the predicate shared by every plan type whose
// landing row is set by a simultaneous reveal rather than a fixed delay:
// the plan is 'pending' and its row_number is still nil. Today that's
// Make War and Clandestinely Liaise; if a future plan type adopts the
// same pattern, this predicate will pick it up automatically.
func hasOpenDelayReveal(p *dbgen.Plan) bool {
	return p.Status == model.PlanPending && p.RowNumber == nil
}

// isDelayRevealPlanType reports whether plan_type uses a simultaneous
// reveal to set its landing row. Centralised so adding a new plan type
// with this pattern is a one-line change.
func isDelayRevealPlanType(t model.PlanType) bool {
	return t == model.PlanMakeWar || t == model.PlanClandestinelyLiaise
}

// openDelayRevealPlan returns the first plan whose landing row is still
// being decided by an open simultaneous reveal (Make War or Clandestinely
// Liaise), if any. Both kinds block the row identically — every player
// watches the participants submit, and play resumes once the reveal
// completes and the plan's row_number is set.
func openDelayRevealPlan(plans []dbgen.Plan) *dbgen.Plan {
	for i := range plans {
		p := &plans[i]
		if !isDelayRevealPlanType(p.PlanType) {
			continue
		}
		if !hasOpenDelayReveal(p) {
			continue
		}
		return p
	}
	return nil
}

// firstKey returns an arbitrary key from m. We don't care which war we
// surface when several owe costs on the same row — the client fetches
// full war state to render specifics.
func firstKey[K comparable, V any](m map[K]V) (K, bool) {
	for k := range m {
		return k, true
	}
	var zero K
	return zero, false
}

func isNoRows(err error) bool {
	// pgx returns a sentinel error for empty result sets; match it the
	// same way loadActiveScene does (avoid importing pgx here so this
	// stays usable in tests without a live driver).
	return err != nil && strings.Contains(err.Error(), "no rows")
}

// broadcastRowState recomputes the RowState for a game and sends a
// row_state.changed event. Called by every handler whose action could
// change the result of ComputeRowState.
//
// Auto-kickoff: if the computed state is kind=plan_pending, the helper
// immediately calls kickoffPlanResolution and recomputes. In normal play,
// plan_pending is a transient state the table never lingers in — there is
// no decision to make at step 2 of the row sequence (the rulebook mandates
// the plan must resolve before the scene), so the manual "Begin resolution"
// click was just gating a foregone conclusion. The plan is now driven
// straight from pending → resolving without operator action.
//
// If the kickoff itself errors out, the plan stays pending; the row state
// remains plan_pending and surfaces as a recovery state (clients can retry
// via the /resolve endpoint). The error is logged but not returned —
// broadcast helpers must not interrupt the mutation that triggered them.
//
// Errors from ComputeRowState or the broadcast are swallowed silently — a
// missed broadcast self-heals on the next legitimate transition or on the
// next snapshot fetch (loadGameState includes row_state).
func broadcastRowState(ctx context.Context, q *dbgen.Queries, manager *hub.Manager, gameID int64) {
	if manager == nil {
		return
	}
	h, ok := manager.Get(gameID)
	if !ok {
		return
	}
	state, err := ComputeRowState(ctx, q, gameID)
	if err != nil {
		return
	}
	// If the row has reached a pending plan, auto-kick off resolution and
	// recompute. The kickoff itself broadcasts plan.resolving; we'll then
	// broadcast the recomputed row_state (kind=plan_resolving, or later if
	// OnResolve fully resolved the plan synchronously, e.g. Make War).
	if state.Kind == model.RowStatePlanPending && state.PlanID != nil {
		plan, perr := q.GetPlanByID(ctx, *state.PlanID)
		if perr == nil {
			if _, kerr := kickoffPlanResolution(ctx, q, manager, &plan); kerr != nil {
				loggerFromContext(ctx).ErrorContext(ctx, "auto-kickoff failed",
					"plan_id", plan.ID, "game_id", gameID, "err", kerr)
			} else if recomputed, rerr := ComputeRowState(ctx, q, gameID); rerr == nil {
				state = recomputed
			}
		}
	}
	h.BroadcastEvent(model.EventRowStateChanged, model.RowStateChangedPayload{RowState: state})
}
