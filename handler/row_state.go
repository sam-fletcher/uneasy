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
//     4x. Player owes a replacement main character → AwaitMainCharacterChoice
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

	// 2/3. War-conflict gates (open surrender claim, then outstanding battle
	// costs) — the highest-priority unresolved blocks, above plan resolution.
	if rs, ok, gErr := warConflictGate(ctx, q, gameID, game.CurrentRow); gErr != nil {
		return model.RowState{}, gErr
	} else if ok {
		return rs, nil
	}

	plans, err := q.ListPlansByGame(ctx, gameID)
	if err != nil {
		return model.RowState{}, err
	}

	// 4. Plan currently resolving. Some plan types have sub-phases that
	// block on a *different* player than the focus player (e.g. Make
	// Demands' counter-demand window blocks on the target). When that's
	// the case, return the narrower kind so the WaitingOnBar can name the
	// actual decision-maker.
	for i := range plans {
		if plans[i].Status != model.PlanResolving {
			continue
		}
		plan := &plans[i]
		id := plan.ID
		// Pre-roll cross-player gate: a Make Demands control_leverage winner owes
		// the leverage decision on this (target) plan's still-open roll. They block
		// the roll from resolving, so name them rather than letting a sub-phase
		// override or the generic preparer case mis-attribute the wait. This is the
		// pre-roll mirror of the post-roll perform_steps handoff handled below.
		if chooser := pendingControlLeverageChooser(ctx, q, plan); chooser != 0 {
			return model.RowState{
				Kind:            model.RowStateAwaitDemandLeverage,
				PlanID:          &id,
				ActingPlayerIDs: []int64{chooser},
			}, nil
		}
		if override, ok := planResolvingWaitees(ctx, q, plan); ok {
			override.PlanID = &id
			return override, nil
		}
		// Generic resolution: the plan is resolved by its preparer (never the
		// focus player — a delayed plan routinely resolves on a row whose focus
		// is someone else). Name the preparer authoritatively so the client
		// needs no focus/preparer proxy of its own.
		//
		// Exception: a Make Demands "perform_steps" winner drives this (target)
		// plan's post-roll make-choice in the preparer's stead. While that choice
		// is outstanding the bar names the winner, not the preparer. This handoff
		// is cross-plan (the chooser isn't a participant of *this* plan's type) so
		// it can't live in a per-plan ResolvingWaitees; it belongs here.
		actor := plan.PreparerID
		if chooser, ok := pendingPerformStepsChooser(ctx, q, plan); ok {
			actor = chooser
		}
		return model.RowState{
			Kind:            model.RowStatePlanResolving,
			PlanID:          &id,
			ActingPlayerIDs: []int64{actor},
		}, nil
	}

	// 4.x. Replacement main character owed. A take/trade/payment (or a fatal
	// break) may have left a player with no main character. Every player must
	// always have exactly one, so the table pauses here until each such player
	// picks a replacement. Placed below the active plan-resolution gates above
	// (a resolution that *caused* the loss finishes first, including its
	// post-commit sub-flows) but above the follow-scene/pending/turn states, so
	// the obligation surfaces before any new turn or plan kickoff.
	if rs, ok, gErr := mainCharacterChoiceGate(ctx, q, gameID); gErr != nil {
		return model.RowState{}, gErr
	} else if ok {
		return rs, nil
	}

	// 4.5. Follow-scene turn for a plan that just resolved on this row.
	// When two plans share a row, the rulebook inserts a full focus-player
	// turn between them — set the scene (using the resolved plan's follow-on
	// prompt), roleplay, prepare/refresh, pass focus — before the next plan
	// resolves. Without this gate the pending-plan step below would win and
	// the next plan would be auto-kicked off the instant the first resolved,
	// skipping the scene/prepare/pass steps entirely. The gate only fires
	// while the resolved plan's follow-scene turn is still in progress (its
	// setter still holds focus); once they pass, it falls through to the
	// pending-plan step so the next plan resolves for the new focus player.
	if rs, ok, err := followSceneGate(ctx, q, &game); err != nil {
		return model.RowState{}, err
	} else if ok {
		return rs, nil
	}

	// 5. Plan pending on the current row. A brief pre-kickoff/recovery state;
	// same actor as resolution — the preparer owns it.
	if top := topPendingPlanOnRow(plans, game.CurrentRow); top != nil {
		id := top.ID
		return model.RowState{
			Kind:            model.RowStatePlanPending,
			PlanID:          &id,
			ActingPlayerIDs: []int64{top.PreparerID},
		}, nil
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

// findFollowScene returns the follow-scene set for a resolved plan (the scene
// whose resolved_plan_id points at it), or nil if none exists yet. There is at
// most one per plan: CreateScene attaches resolved_plan_id to the scene set
// after a resolution, and only one such scene is allowed per resolved plan.
// The kind check is defense-in-depth: a plan-scene never sets resolved_plan_id
// (it's tied to the plan it belongs to via plan_id instead), so this could
// only matter if that invariant were ever violated.
func findFollowScene(scenes []dbgen.Scene, planID int64) *dbgen.Scene {
	for i := range scenes {
		if scenes[i].Kind == model.SceneKindTurn &&
			scenes[i].ResolvedPlanID != nil && *scenes[i].ResolvedPlanID == planID {
			return &scenes[i]
		}
	}
	return nil
}

// followSceneGate reports the focus player's row-state when the most-recently
// resolved plan on the current row still owes (or is mid-) its follow-scene
// turn. It returns ok=false — deferring to the normal pending-plan precedence —
// when no plan has resolved on this row yet (we're at the row's first
// resolution, which the rulebook runs before any scene), or when the resolved
// plan's follow-scene turn is already complete (its setter has passed focus).
//
//   - no follow-scene yet          → SceneSetting   (focus owes the scene)
//   - follow-scene not ended       → SceneActive    (roleplaying it)
//   - follow-scene ended, setter still holds focus → PostSceneAction
//   - follow-scene ended, focus moved on → ok=false (turn done; next plan resolves)
func followSceneGate(ctx context.Context, q *dbgen.Queries, game *dbgen.Game) (model.RowState, bool, error) {
	recent, err := q.GetMostRecentResolvedPlanOnRow(ctx, dbgen.GetMostRecentResolvedPlanOnRowParams{
		GameID:    game.ID,
		RowNumber: new(game.CurrentRow),
	})
	if err != nil {
		if isNoRows(err) {
			// No plan has resolved on this row → row start; resolve first.
			return model.RowState{}, false, nil
		}
		return model.RowState{}, false, err
	}

	scenes, err := q.ListScenesForRow(ctx, dbgen.ListScenesForRowParams{
		GameID:    game.ID,
		RowNumber: game.CurrentRow,
	})
	if err != nil {
		return model.RowState{}, false, err
	}

	follow := findFollowScene(scenes, recent.ID)
	if follow == nil {
		// The focus player owes the just-resolved plan's follow-scene.
		return model.RowState{Kind: model.RowStateSceneSetting}, true, nil
	}
	if !follow.EndedAt.Valid {
		id := follow.ID
		return model.RowState{Kind: model.RowStateSceneActive, SceneID: &id}, true, nil
	}
	// Follow-scene ended. If its setter still holds focus, they owe the
	// post-scene action (prepare a plan or refresh) before passing. Once
	// they've passed — focus has moved to another player — the turn is
	// complete and the next pending plan should resolve.
	if game.FocusPlayerID != nil && *game.FocusPlayerID == follow.FocusPlayerID {
		return model.RowState{Kind: model.RowStatePostSceneAction}, true, nil
	}
	return model.RowState{}, false, nil
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

// planResolvingWaitees asks the resolving plan's handler whether it wants to
// override the generic preparer case with a narrower RowState (table blocked on
// a player other than the focus player, or a wait the WaitingOnBar would
// otherwise mis-attribute). Returns the narrower RowState with Kind and
// ActingPlayerIDs set; the caller fills in PlanID.
//
// Each plan owns this logic via the optional ResolvingWaitees capability
// (defined next to its OnResolve / CanComplete). Plans without the capability
// (the linear single-step plans) ride the generic preparer case. There is no
// central per-type switch any more — adding a plan's waiting-on no longer means
// editing this file; this is just the type-assertion seam.
func planResolvingWaitees(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	h, ok := GetHandler(plan.PlanType)
	if !ok {
		return model.RowState{}, false
	}
	rw, ok := h.(ResolvingWaitees)
	if !ok {
		return model.RowState{}, false
	}
	return rw.ResolvingWaitees(ctx, q, plan)
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
// warConflictGate reports the two war-conflict gates that preempt plan
// resolution: an open surrender claim (rulebook step 1), then any outstanding
// battle cost on the current row. ok is false when neither applies. Split out of
// ComputeRowState to keep it short.
func warConflictGate(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
	currentRow int16,
) (model.RowState, bool, error) {
	claims, err := q.ListOpenSurrenderClaimsByGame(ctx, gameID)
	if err != nil {
		return model.RowState{}, false, err
	}
	if len(claims) > 0 {
		id := claims[0].ID
		return model.RowState{Kind: model.RowStateAwaitSurrenderClaim, ClaimID: &id}, true, nil
	}

	outstanding, err := mwOutstandingCostsForGame(ctx, q, gameID, currentRow)
	if err != nil {
		return model.RowState{}, false, err
	}
	if warID, ok := firstKey(outstanding); ok {
		return model.RowState{Kind: model.RowStateAwaitBattleCost, WarID: &warID}, true, nil
	}
	return model.RowState{}, false, nil
}

// mainCharacterChoiceGate reports the replacement-main-character gate
// (RowStateAwaitMainCharacterChoice) when any player has lost their main
// character — taken, traded, or destroyed — and has none. ok is false when
// every player still has one. Split out of ComputeRowState to keep it short.
func mainCharacterChoiceGate(ctx context.Context, q *dbgen.Queries, gameID int64) (model.RowState, bool, error) {
	missing, err := q.ListPlayersMissingMainCharacter(ctx, gameID)
	if err != nil {
		return model.RowState{}, false, err
	}
	if len(missing) == 0 {
		return model.RowState{}, false, nil
	}
	return model.RowState{
		Kind:            model.RowStateAwaitMainCharacterChoice,
		ActingPlayerIDs: missing,
	}, true, nil
}

func broadcastRowState(ctx context.Context, q *dbgen.Queries, manager *hub.Manager, gameID int64) {
	if manager == nil {
		return
	}
	state, err := ComputeRowState(ctx, q, gameID)
	if err != nil {
		return
	}
	// If the row has reached a pending plan, auto-kick off resolution and
	// recompute — regardless of whether a hub exists for this game yet. The
	// state transition (pending -> resolving) must happen even if nobody is
	// currently connected (async play-by-post), not only when there happens
	// to be a live broadcast to piggyback on; otherwise the plan is stuck
	// pending until some other action incidentally re-triggers this function
	// while a client is online. The kickoff itself broadcasts plan.resolving
	// (a no-op if no hub exists); we'll then broadcast the recomputed
	// row_state below (kind=plan_resolving, or later if OnResolve fully
	// resolved the plan synchronously, e.g. Make War) — but only if there's
	// actually a hub to broadcast to.
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

	h, ok := manager.Get(gameID)
	if !ok {
		return
	}
	h.BroadcastEvent(model.EventRowStateChanged, model.RowStateChangedPayload{RowState: state})
}
