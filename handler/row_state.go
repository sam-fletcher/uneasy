package handler

import (
	"context"
	"strings"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
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
		if override, ok := resolvingPlanSubPhase(ctx, q, plan); ok {
			override.PlanID = &id
			return override, nil
		}
		return model.RowState{Kind: model.RowStatePlanResolving, PlanID: &id}, nil
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

// findFollowScene returns the follow-scene set for a resolved plan (the scene
// whose resolved_plan_id points at it), or nil if none exists yet. There is at
// most one per plan: CreateScene attaches resolved_plan_id to the scene set
// after a resolution, and only one such scene is allowed per resolved plan.
func findFollowScene(scenes []dbgen.Scene, planID int64) *dbgen.Scene {
	for i := range scenes {
		if scenes[i].ResolvedPlanID != nil && *scenes[i].ResolvedPlanID == planID {
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

// resolvingPlanSubPhase checks whether the resolving plan is inside a
// sub-phase that warrants its own RowState kind (i.e. the table is blocked
// on a player other than the focus player, or the WaitingOnBar copy would
// otherwise mis-attribute the wait). Returns the narrower RowState with
// Kind and ActingPlayerID set; the caller fills in PlanID.
//
// The fan-out is by plan_type because each type's sub-phases are encoded
// in its own resolution_data shape. Today only Make Demands' counter-
// demand window is covered; future kinds (festivity guest turns, duel
// bouts, etc.) plug in here.
func resolvingPlanSubPhase(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	//nolint:exhaustive // only plan types with sub-phase overrides need cases here
	switch plan.PlanType {
	case model.PlanMakeDemands:
		return demandSubPhase(ctx, q, plan)
	case model.PlanHostFestivity:
		return festivitySubPhase(ctx, q, plan)
	case model.PlanProposeDuel:
		return duelSubPhase(ctx, q, plan)
	case model.PlanSpreadRumors:
		return srSubPhase(plan)
	case model.PlanSeekAnswers:
		return saSubPhase(plan)
	case model.PlanClandestinelyLiaise:
		return liaiseSubPhase(ctx, q, plan)
	case model.PlanExchangeCourtiers:
		return ecSubPhase(ctx, q, plan)
	case model.PlanChronicleHistories:
		return chSubPhase(ctx, q, plan)
	}
	return model.RowState{}, false
}

// liaiseSubPhase narrows a resolving Clandestinely Liaise during its
// collaborative submit phases so the WaitingOnBar names exactly who still owes
// a submission — never the focus player, who (unlike every other resolving
// plan) is often not even a participant, since the liaison was prepared on an
// earlier turn and resolves on its delayed row. The preparer-only phases
// (together_at_last, done) return false and ride the generic plan_resolving
// case, which already names the resolving plan's preparer.
func liaiseSubPhase(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	resData := loadResolutionData(plan.ResolutionData)
	ld := resData.Liaise
	if ld == nil || ld.PartnerID == nil {
		return model.RowState{}, false
	}
	participants := []int64{plan.PreparerID, *ld.PartnerID}

	// pendingThen names participants who still owe a submission; once all are
	// in, it falls back to the preparer, who owes the "Advance" click.
	pendingThen := func(submitted map[int64]bool, whenDone []int64) []int64 {
		var pending []int64
		for _, p := range participants {
			if !submitted[p] {
				pending = append(pending, p)
			}
		}
		if len(pending) == 0 {
			return whenDone
		}
		return pending
	}

	//nolint:exhaustive // together_at_last/done handled by default (ride generic)
	switch ld.Phase {
	case gamepkg.LiaisePhaseSecretsWeKeep:
		submitted := map[int64]bool{}
		for _, ks := range ld.KeptSecrets {
			submitted[ks.PlayerID] = true
		}
		ids := pendingThen(submitted, []int64{plan.PreparerID})
		return model.RowState{Kind: model.RowStateLiaiseResolving, ActingPlayerIDs: ids}, true
	case gamepkg.LiaisePhaseThingsWeShare:
		submitted := map[int64]bool{}
		for _, id := range ld.ShareSubmitterIDs {
			submitted[id] = true
		}
		ids := pendingThen(submitted, []int64{plan.PreparerID})
		return model.RowState{Kind: model.RowStateLiaiseResolving, ActingPlayerIDs: ids}, true
	case gamepkg.LiaisePhaseWhenWillISeeYouAgain:
		// Both participants reveal a redelay face simultaneously; the reveal
		// completing advances the plan on its own, so name only those who still
		// owe a face (no "advance" click). All in → ride the generic case.
		submitted := liaiseRedelaySubmitters(ctx, q, ld)
		ids := pendingThen(submitted, nil)
		if len(ids) == 0 {
			return model.RowState{}, false
		}
		return model.RowState{Kind: model.RowStateLiaiseResolving, ActingPlayerIDs: ids}, true
	default:
		return model.RowState{}, false
	}
}

// liaiseRedelaySubmitters returns the set of participants who have submitted a
// face in the when-will-I-see-you-again redelay reveal.
func liaiseRedelaySubmitters(ctx context.Context, q *dbgen.Queries, ld *gamepkg.LiaiseResolutionData) map[int64]bool {
	submitted := map[int64]bool{}
	if ld.RedelayRevealID == nil {
		return submitted
	}
	entries, err := q.ListRevealEntries(ctx, *ld.RedelayRevealID)
	if err != nil {
		return submitted
	}
	for _, e := range entries {
		if e.Face != nil {
			submitted[e.PlayerID] = true
		}
	}
	return submitted
}

// ecSubPhase narrows a resolving Exchange Courtiers to AwaitCourtierResponse
// during its target-driven sub-steps, so the bar blocks on the target player
// (never the preparer or focus player). The preparer's sub-steps (accept/decline,
// make choices, riposte break, completion) return false and ride the generic
// plan_resolving case (which names the preparer).
func ecSubPhase(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	if plan.TargetPlayerID == nil {
		return model.RowState{}, false
	}
	target := *plan.TargetPlayerID
	blockTarget := model.RowState{Kind: model.RowStateAwaitCourtierResponse, ActingPlayerID: &target}

	ec := loadResolutionData(plan.ResolutionData).ExchangeCourtiers
	// Before any fair-trade action the target owes the opening offer.
	if ec == nil {
		return blockTarget, true
	}
	// Fair-trade step still open: target owes the offer until one is made;
	// then the preparer owes accept/decline (generic).
	if ec.FairTradeAccepted == nil {
		if ec.FairTradeAssetID == nil {
			return blockTarget, true
		}
		return model.RowState{}, false
	}
	// Post-roll target-driven sub-steps.
	if ec.MessyBreakRequired && !ec.MessyBreakDone {
		return blockTarget, true
	}
	if ec.PeerClaimsDone < ec.PeerClaimsRequired {
		return blockTarget, true
	}
	// A marred roll hands the option choices to the target; block on them until
	// they submit (a fair_trade-only mar leaves no other trace, hence the flag).
	if !ec.MarChoicesSubmitted && planRollIsMar(ctx, q, plan) {
		return blockTarget, true
	}
	return model.RowState{}, false
}

// chSubPhase narrows a marred Chronicle Histories to AwaitChronicleChoices: every
// player present when the mar scene began must each submit one option. The bar
// names those who still owe a choice (game players minus the distinct submitters
// in MakeMarChoices). The make path and a fully-chosen mar return false and ride
// the generic plan_resolving case (the preparer completes).
func chSubPhase(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	if !planRollIsMar(ctx, q, plan) {
		return model.RowState{}, false
	}
	resData := loadResolutionData(plan.ResolutionData)
	submitted := map[int64]bool{}
	for _, c := range resData.MakeMarChoices {
		if c.PlayerID != nil {
			submitted[*c.PlayerID] = true
		}
	}
	players, err := q.GetPlayersByGame(ctx, plan.GameID)
	if err != nil {
		return model.RowState{}, false
	}
	var pending []int64
	for i := range players {
		if !submitted[players[i].ID] {
			pending = append(pending, players[i].ID)
		}
	}
	if len(pending) == 0 {
		return model.RowState{}, false
	}
	return model.RowState{Kind: model.RowStateAwaitChronicleChoices, ActingPlayerIDs: pending}, true
}

// saSubPhase returns AwaitQuestionAnswer while a Seek Answers "ask a player a
// question" pick is waiting on the target's answer or veto. ActingPlayerID names
// the target, so the table blocks on them rather than the resolving plan's focus
// player. No pending question → keep the default PlanResolving copy.
func saSubPhase(plan *dbgen.Plan) (model.RowState, bool) {
	resData := loadResolutionData(plan.ResolutionData)
	if sa := resData.SeekAnswers; sa != nil && sa.PendingQuestion != nil {
		target := sa.PendingQuestion.TargetID
		return model.RowState{Kind: model.RowStateAwaitQuestionAnswer, ActingPlayerID: &target}, true
	}
	return model.RowState{}, false
}

// srSubPhase returns AwaitTakeConsent while a Spread Rumors "take asset" choice
// is waiting on the victim's agree/disagree. ActingPlayerID names the victim
// (the asset owner), so the table blocks on them rather than the resolving
// plan's focus player. No pending consent → keep the default PlanResolving copy.
func srSubPhase(plan *dbgen.Plan) (model.RowState, bool) {
	resData := loadResolutionData(plan.ResolutionData)
	if sr := resData.SpreadRumors; sr != nil && sr.PendingTakeConsent != nil {
		victim := sr.PendingTakeConsent.VictimID
		return model.RowState{Kind: model.RowStateAwaitTakeConsent, ActingPlayerID: &victim}, true
	}
	return model.RowState{}, false
}

// demandSubPhase routes a resolving Make Demands plan to the right
// override: AwaitDemandDraftPick on a made roll while the four-pick draft
// is in progress, AwaitDemandCounter on a marred roll until the target
// counters or waives. Detection uses the dice roll outcome — plan.Result
// isn't written until the demand is completed, so the roll is the source
// of truth during resolution.
func demandSubPhase(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	outcome := mdRollOutcome(ctx, q, plan.ID)
	if outcome == "" || plan.TargetedPlanID == nil {
		return model.RowState{}, false
	}
	target, err := q.GetPlanByID(ctx, *plan.TargetedPlanID)
	if err != nil {
		return model.RowState{}, false
	}
	resData := loadResolutionData(plan.ResolutionData)
	switch outcome {
	case makeOutcome:
		return demandDraftSubPhase(ctx, q, plan, &target, &resData)
	case marOutcome:
		if md := resData.MakeDemands; md != nil && md.CounterDemandPlaced {
			return model.RowState{}, false
		}
		actor := target.PreparerID
		return model.RowState{Kind: model.RowStateAwaitDemandCounter, ActingPlayerID: &actor}, true
	}
	return model.RowState{}, false
}

// demandDraftSubPhase returns AwaitDemandDraftPick when the four-pick
// draft is still in progress, with ActingPlayerID set to whoever owes the
// next pick (alternating starting with the higher-ranked player).
func demandDraftSubPhase(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	target *dbgen.Plan,
	resData *ResolutionData,
) (model.RowState, bool) {
	picks := 0
	if md := resData.MakeDemands; md != nil {
		picks = len(md.DraftChoices)
	}
	if picks >= 4 {
		return model.RowState{}, false
	}
	first, second, err := mdDraftPickers(ctx, q, plan.GameID, plan.PreparerID, target.PreparerID)
	if err != nil {
		return model.RowState{}, false
	}
	actor := first
	if picks%2 == 1 {
		actor = second
	}
	return model.RowState{Kind: model.RowStateAwaitDemandDraftPick, ActingPlayerID: &actor}, true
}

// festivitySubPhase returns the narrower RowState for a resolving Host
// Festivity plan. During 'socializing': an open challenge takes precedence
// (block on the target), otherwise block on the next guest in esteem order.
// 'host_choosing' and 'done' keep the default PlanResolving copy — the host
// is the preparer (= focus player at resolve time) so the existing label is
// already accurate.
func festivitySubPhase(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	state := gamepkg.LoadFestivityData(plan.ResolutionData)
	if state.Phase != gamepkg.FestivityPhaseSocializing {
		return model.RowState{}, false
	}
	if state.PendingChallenge != nil {
		actor := state.PendingChallenge.TargetID
		return model.RowState{Kind: model.RowStateAwaitFestivityChallengeResponse, ActingPlayerID: &actor}, true
	}
	rankings, err := q.ListRankingsByGame(ctx, plan.GameID)
	if err != nil {
		return model.RowState{}, false
	}
	rankFor := func(playerID int64) int16 {
		for i := range rankings {
			r := &rankings[i]
			if r.Category != model.CategoryEsteem || r.PlayerID == nil {
				continue
			}
			if *r.PlayerID == playerID {
				return r.Rank
			}
		}
		// Low sentinel: an unranked guest sorts LAST in NextSocializingTurn's
		// descending-by-rank order (a high value would wrongly put them first).
		return 0
	}
	next := state.NextSocializingTurn(plan.PreparerID, rankFor)
	if next == 0 {
		return model.RowState{}, false
	}
	return model.RowState{Kind: model.RowStateAwaitFestivityGuestTurn, ActingPlayerID: &next}, true
}

// duelSubPhase returns the narrower RowState for a resolving Propose Duel
// plan. setup/staking are simultaneous-submit (multiple waitees, derived
// client-side from plan.resolution_data). 'bouts' blocks on a single
// player — the responder if a declared bout is unresolved, otherwise the
// declarer (InitiativePlayerID). 'roll' and 'done' keep the default
// PlanResolving copy (roll is the standard dice flow; done is terminal).
func duelSubPhase(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	state := gamepkg.LoadDuelData(plan.ResolutionData)
	switch state.Phase {
	case gamepkg.DuelPhaseSetup, gamepkg.DuelPhaseStaking:
		ids := duelStakingActors(ctx, q, plan, &state)
		return model.RowState{Kind: model.RowStateAwaitDuelStaking, ActingPlayerIDs: ids}, true
	case gamepkg.DuelPhaseBouts:
		actor := duelBoutActor(ctx, q, plan, &state)
		if actor == 0 {
			return model.RowState{}, false
		}
		return model.RowState{Kind: model.RowStateAwaitDuelBout, ActingPlayerID: &actor}, true
	case gamepkg.DuelPhaseRoll, gamepkg.DuelPhaseDone:
		return model.RowState{}, false
	}
	return model.RowState{}, false
}

// duelStakingActors returns the duellists who still owe a staking action,
// filtering the "both duellists" default down to who actually hasn't submitted.
// In setup that's whoever hasn't revealed a stake count; in staking, whoever
// hasn't staked their full count of assets. An empty result (a transient
// between-sub-step moment) falls back to both, so the bar never blanks out.
func duelStakingActors(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	state *gamepkg.DuelResolutionData,
) []int64 {
	participants := []int64{plan.PreparerID}
	if plan.TargetPlayerID != nil {
		participants = append(participants, *plan.TargetPlayerID)
	}

	var pending []int64
	//nolint:exhaustive // only setup/staking filter; other phases hit default
	switch state.Phase {
	case gamepkg.DuelPhaseSetup:
		for _, p := range participants {
			if _, ok := state.StakeCounts[p]; !ok {
				pending = append(pending, p)
			}
		}
	case gamepkg.DuelPhaseStaking:
		stakes, err := q.ListDuelStakesByPlan(ctx, plan.ID)
		if err != nil {
			return participants
		}
		staked := map[int64]int16{}
		for i := range stakes {
			staked[stakes[i].PlayerID]++
		}
		required := func(pid int64) int16 {
			if pid == plan.PreparerID {
				return state.PreparerStakeCount
			}
			return state.TargetStakeCount
		}
		for _, p := range participants {
			if staked[p] < required(p) {
				pending = append(pending, p)
			}
		}
	default:
		return participants
	}

	if len(pending) == 0 {
		return participants
	}
	return pending
}

// duelBoutActor returns the duellist who owes the next bout action: the
// responder if a declared bout is unresolved, else the declarer (=
// InitiativePlayerID). Returns 0 if neither can be determined.
func duelBoutActor(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan, state *gamepkg.DuelResolutionData) int64 {
	latest, err := q.GetLatestDuelBout(ctx, plan.ID)
	if err == nil && !latest.ResolvedAt.Valid {
		return latest.ResponderID
	}
	if state.InitiativePlayerID != nil {
		return *state.InitiativePlayerID
	}
	return 0
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
