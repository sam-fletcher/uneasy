package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// ── demand-leverage (Stage 4) ────────────────────────────────────────────────
//
// Mounted on the *target* plan: POST /api/plans/:planId/demand-leverage with
// body {"asset_ids": [int64]}. Callable only by the control_leverage winner of
// a resolved, made demand against this plan, while the target plan is still
// in its leverage window (status = resolving, roll open). Leverages the chosen
// subset of the target preparer's own assets onto the target plan's roll.
// The target preparer's own leverage of their own assets is separately blocked
// while a control_leverage winner exists (see handler/rolls.go LeverageRoll).
//
// This call is the winner's FINALIZE signal: each successful call flips the
// plan's demand_leverage_finalized flag, after which the winner stops blocking
// the roll and readies normally (so the roll can auto-resolve). An empty
// asset_ids list is valid and meaningful — it leverages none of the preparer's
// assets, deliberately guaranteeing the roll's failure per the rulebook. Until
// this fires, the winner is seeded unready and excluded from the auto-ready
// sweeps (see rolls_stage.go), so the roll cannot resolve without them.
//
//nolint:gocognit,funlen // leverage rights handoff to demand winner
func mdDemandLeverageHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType == model.PlanMakeDemands {
			respondErr(w, http.StatusBadRequest, "demand-leverage is mounted on the target plan, not the demand plan")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "target plan is not in resolving status")
			return
		}

		ctx := r.Context()
		_, winners, err := DemandWinnersForTargetPlan(ctx, deps.Q, plan)
		if err != nil {
			respondInternalErr(w, r, "could not load demand winners", err)
			return
		}
		winnerID, ok := winners[gamepkg.DemandOptionControlLeverage]
		if !ok || winnerID == 0 {
			respondErr(w, http.StatusConflict, "no control_leverage winner on this plan")
			return
		}
		if player.ID != winnerID {
			respondErr(w, http.StatusForbidden, "only the control_leverage winner may set leverage here")
			return
		}
		// The option drew a dud: this target has no roll of the preparer's own to
		// leverage into (audit D7). Say so rather than reaching for whatever roll
		// happens to carry this plan_id — for a Host Festivity that is a *guest's*
		// roll, and spending the host's assets into it would be interference by a
		// player who never chose to interfere.
		if !mdTargetHasPreparerRoll(plan.PlanType) {
			respondErr(w, http.StatusConflict, fmt.Sprintf(
				"leverage control is inert against %s: the plan has no roll of its preparer's own to leverage into",
				planLabel(plan.PlanType)))
			return
		}

		roll, err := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID)
		if err != nil {
			respondErr(w, http.StatusConflict, "target plan has no open roll")
			return
		}
		if !rollIsOpen(&roll) {
			respondErr(w, http.StatusConflict, "roll is already resolved")
			return
		}

		var body struct {
			AssetIDs []int64 `json:"asset_ids"`
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		existingDice, err := deps.Q.ListDiceByRoll(ctx, roll.ID)
		if err != nil {
			respondInternalErr(w, r, "could not list dice", err)
			return
		}
		committed := map[int64]struct{}{}
		for _, d := range existingDice {
			if d.LeveragedAssetID != nil {
				committed[*d.LeveragedAssetID] = struct{}{}
			}
		}

		var leveragedNames []string
		for _, assetID := range body.AssetIDs {
			asset, err := deps.Q.GetAssetByID(ctx, assetID)
			if err != nil {
				respondErr(w, http.StatusNotFound, fmt.Sprintf("asset %d not found", assetID))
				return
			}
			if asset.OwnerID != plan.PreparerID {
				respondErr(w, http.StatusForbidden,
					fmt.Sprintf("asset %d does not belong to the target preparer", assetID))
				return
			}
			if asset.IsDestroyed {
				respondErr(w, http.StatusConflict, fmt.Sprintf("asset %d is destroyed", assetID))
				return
			}
			if _, dup := committed[assetID]; dup {
				continue
			}
			if err := deps.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
				ID: assetID, IsLeveraged: true,
			}); err != nil {
				respondInternalErr(w, r, "could not leverage asset", err)
				return
			}
			// Target preparer's own dice would not be interference; these are
			// added on their behalf by the demand winner, so keep non-interference.
			if _, err := deps.Q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
				RollID:           roll.ID,
				PlayerID:         plan.PreparerID,
				IsInterference:   plan.PreparerID != roll.ActorID,
				LeveragedAssetID: &assetID,
			}); err != nil {
				respondInternalErr(w, r, "could not add leverage die", err)
				return
			}
			leveragedNames = append(leveragedNames, assetMark(asset.Name))
			committed[assetID] = struct{}{}
		}

		broadcastEvent(deps.Manager, plan.GameID, demandEventLeverageSet, map[string]any{
			"plan_id":   plan.ID,
			"roll_id":   roll.ID,
			"asset_ids": body.AssetIDs,
			"player_id": player.ID,
		})

		// The demand winner is spending the target's own assets against them — a
		// pointed, dramatic move worth narrating in the log.
		if len(leveragedNames) > 0 {
			mdLog(ctx, deps, plan, model.SeverityImportant, fmt.Sprintf(
				"Holding the reins of %s's plan, %s forced %s into the roll against them.",
				playerDisplayName(ctx, deps.Q, plan.PreparerID),
				playerDisplayName(ctx, deps.Q, player.ID),
				strings.Join(leveragedNames, ", ")))
		} else {
			// Leveraging none is a deliberate move (guaranteeing failure), so it
			// gets its own log line rather than passing silently.
			mdLog(ctx, deps, plan, model.SeverityImportant, fmt.Sprintf(
				"Holding the reins of %s's plan, %s leveraged none of their assets — leaving the roll to fail.",
				playerDisplayName(ctx, deps.Q, plan.PreparerID),
				playerDisplayName(ctx, deps.Q, player.ID)))
		}

		// Finalize: flip the plan's flag so the winner stops blocking the roll,
		// then let them ready normally and resolve if everyone else is in. Done
		// after the leverage writes above so the gate clears only once their
		// decision is locked in.
		resData := loadResolutionData(plan.ResolutionData)
		resData.DemandLeverageFinalized = true
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not finalize demand leverage", err)
			return
		}
		if err := runAutoReadySweep(ctx, deps.Q, deps.Manager, &roll, player.ID); err != nil {
			respondInternalErr(w, r, "could not update ready state", err)
			return
		}
		if err := maybeAutoResolve(ctx, w, r, deps.Q, deps.Manager, &roll); err != nil {
			respondInternalErr(w, r, "could not auto-resolve roll", err)
			return
		}
		// Clears the AwaitDemandLeverage row-state gate even when the roll didn't
		// resolve yet (the winner kept their own dice in hand).
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"roll_id":   roll.ID,
			"asset_ids": body.AssetIDs,
		})
	}
}

// ── demand-retarget (Stage 4) ────────────────────────────────────────────────
//
// Mounted on the *target* plan: POST /api/plans/:planId/demand-retarget with
// body {"keep"?, "target_player_id"?, "target_asset_id"?}. Callable only by the
// keep_or_change_target winner. With "keep": true the plan's target columns are
// left exactly as they are; otherwise the route re-validates the target plan's
// preparation rules with the proposed new values (treating the target plan's
// own preparer as the nominal player) before persisting. Only valid while the
// target plan has not yet resolved its roll.
//
// Either way the call is the winner's FINALIZE signal, exactly like
// /demand-leverage: it flips the plan's demand_retarget_finalized flag, after
// which the winner stops blocking the roll and readies normally. "Keep the
// current target" needs that explicit signal because the rules make keeping a
// real choice ("keep OR change"), and a kept target is indistinguishable from an
// untouched one — without a finalize the roll could auto-resolve the window shut
// before the winner ever acted (audit D5).
func mdDemandRetargetHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType == model.PlanMakeDemands {
			respondErr(w, http.StatusBadRequest, "demand-retarget is mounted on the target plan, not the demand plan")
			return
		}
		if plan.Status != model.PlanPending && plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "target plan has already resolved")
			return
		}

		ctx := r.Context()
		_, winners, err := DemandWinnersForTargetPlan(ctx, deps.Q, plan)
		if err != nil {
			respondInternalErr(w, r, "could not load demand winners", err)
			return
		}
		winnerID, ok := winners[gamepkg.DemandOptionKeepOrChangeTarget]
		if !ok || winnerID == 0 {
			respondErr(w, http.StatusConflict, "no keep_or_change_target winner on this plan")
			return
		}
		if player.ID != winnerID {
			respondErr(w, http.StatusForbidden, "only the keep_or_change_target winner may retarget")
			return
		}

		// Block retarget once the roll has been RESOLVED — stakes are locked then.
		// "No roll yet" is not an error and not a closed window: the status guard
		// above admits PlanPending, which by definition has no roll, and 7 of the
		// 10 legal target types create no roll in OnResolve at all — two never
		// (Clandestinely Liaise, Host Festivity) and five from a later extra route
		// (Propose Duel, Chronicle Histories, Make Introductions, Seek Answers,
		// Exchange Courtiers). Treating the empty result as an internal error made
		// this route 500 for exactly the window the winner would act in (audit D2).
		roll, errRoll := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID)
		haveRoll := errRoll == nil
		if haveRoll && !rollIsOpen(&roll) {
			respondErr(w, http.StatusConflict, "target plan's roll has already resolved")
			return
		}

		var body struct {
			// Keep is the explicit "leave the target exactly where it is" call —
			// a legitimate outcome the rules name, and the reason this route needs
			// a finalize signal at all.
			Keep           bool   `json:"keep"`
			TargetPlayerID *int64 `json:"target_player_id"`
			TargetAssetID  *int64 `json:"target_asset_id"`
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		if !body.Keep {
			if !mdApplyRetarget(w, r, deps, plan, body.TargetPlayerID, body.TargetAssetID) {
				return
			}
		}

		// Finalize: flip the plan's flag so the winner stops blocking the roll.
		// Done after the target write above so the gate clears only once their
		// decision is locked in — the same ordering /demand-leverage uses.
		resData := loadResolutionData(plan.ResolutionData)
		resData.DemandRetargetFinalized = true
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not finalize demand retarget", err)
			return
		}

		broadcastEvent(deps.Manager, plan.GameID, demandEventRetargeted, map[string]any{
			"plan_id":          plan.ID,
			"kept":             body.Keep,
			"target_player_id": body.TargetPlayerID,
			"target_asset_id":  body.TargetAssetID,
			"player_id":        player.ID,
		})

		// Both branches are narrated: "the demand changed nothing" is itself a
		// beat the table should see, not silence.
		if body.Keep {
			mdLog(ctx, deps, plan, model.SeverityImportant, fmt.Sprintf(
				"%s let %s's %s stand exactly as aimed.",
				playerDisplayName(ctx, deps.Q, player.ID),
				playerDisplayName(ctx, deps.Q, plan.PreparerID), planLabel(plan.PlanType)))
		} else {
			mdLog(ctx, deps, plan, model.SeverityImportant, fmt.Sprintf(
				"%s re-aimed %s's %s under their demand.",
				playerDisplayName(ctx, deps.Q, player.ID),
				playerDisplayName(ctx, deps.Q, plan.PreparerID), planLabel(plan.PlanType)))
		}

		// Release the roll the winner was holding open, then let it resolve if
		// everyone else is in. Skipped for a target with no preparer-owned roll:
		// such a winner was never seeded as a blocker, and on a Propose Duel the
		// roll carries no participant rows at all (audit D7).
		if haveRoll && mdTargetHasPreparerRoll(plan.PlanType) {
			if err := runAutoReadySweep(ctx, deps.Q, deps.Manager, &roll, player.ID); err != nil {
				respondInternalErr(w, r, "could not update ready state", err)
				return
			}
			if err := maybeAutoResolve(ctx, w, r, deps.Q, deps.Manager, &roll); err != nil {
				respondInternalErr(w, r, "could not auto-resolve roll", err)
				return
			}
		}
		// Clears the AwaitDemandRetarget row-state gate even when the roll didn't
		// resolve yet (or never existed).
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":          plan.ID,
			"kept":             body.Keep,
			"target_player_id": body.TargetPlayerID,
			"target_asset_id":  body.TargetAssetID,
		})
	}
}

// mdApplyRetarget re-validates and persists a re-aimed target plan's
// target_player_id / target_asset_id. It runs the target plan type's own
// ValidatePreparation with the proposed values, standing the plan's preparer in
// as the nominal player, so a demand winner cannot aim a plan somewhere its
// preparer never could have. Reports false (and has already written the
// response) when validation or the write failed.
//
// Everything the plan already declared is replayed into the context alongside
// the new target — the notes column, Make Introductions' peer count,
// Clandestinely Liaise's two meeting peers. The validator is re-judging THIS
// plan with only its aim changed, so withholding those fields just makes it
// reject the plan's own settled state: an empty Notes alone 400s a re-aim of
// Spread Rumors, Propose Decree, Seek Answers, Chronicle Histories, Propose Duel
// and Host Festivity — including the rulebook's own worked example for this
// option ("if it's Spread Rumors… make it a rumor about a different character").
//
// This does not let the winner CHANGE any of that declared content; that is a
// separate, deferred question (audit D12).
func mdApplyRetarget(
	w http.ResponseWriter,
	r *http.Request,
	deps *PlanDeps,
	plan *dbgen.Plan,
	targetPlayerID, targetAssetID *int64,
) bool {
	ctx := r.Context()
	targetHandler, ok := GetHandler(plan.PlanType)
	if !ok {
		respondErr(w, http.StatusInternalServerError, "no handler for target plan type")
		return false
	}
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		respondInternalErr(w, r, "could not load game", err)
		return false
	}
	preparer, err := deps.Q.GetPlayerByID(ctx, plan.PreparerID)
	if err != nil {
		respondInternalErr(w, r, "could not load target preparer", err)
		return false
	}
	vc := &ValidationContext{
		Q:              deps.Q,
		Game:           &game,
		Player:         &preparer,
		TargetPlayerID: targetPlayerID,
		TargetAssetID:  targetAssetID,
	}
	if plan.PreparationNotes != nil {
		vc.Notes = *plan.PreparationNotes
	}
	resData := loadResolutionData(plan.ResolutionData)
	if mi := resData.MakeIntroductions; mi != nil {
		vc.PeerCount = mi.PeerCount
	}
	if ld := resData.Liaise; ld != nil {
		vc.PreparerPeerID = ld.PreparerPeerID
		vc.PartnerPeerID = ld.PartnerPeerID
	}
	if _, errMsg := targetHandler.ValidatePreparation(ctx, vc); errMsg != "" {
		respondErr(w, http.StatusBadRequest, "retarget invalid: "+errMsg)
		return false
	}
	if err := deps.Q.SetPlanTargets(ctx, dbgen.SetPlanTargetsParams{
		ID:             plan.ID,
		TargetPlayerID: targetPlayerID,
		TargetAssetID:  targetAssetID,
	}); err != nil {
		respondInternalErr(w, r, "could not update plan targets", err)
		return false
	}
	return true
}
