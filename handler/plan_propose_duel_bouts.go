package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// ── Bout Declare ──────────────────────────────────────────────────────────────

// pduelBoutDeclareHandler: POST /api/plans/:planId/bout-declare
// Body: {"stake_id": N, "declaration": "high"|"low"}
// Only the player with initiative (the current declarer) may call this.
// Starts a new bout; the responder then completes it via bout-respond.
func pduelBoutDeclareHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanProposeDuel)
		if !ok {
			return
		}
		if !pduelIsParticipant(plan, player.ID) {
			respondErr(w, http.StatusForbidden, "only duellists may declare bouts")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureDuel()
		if state.Phase != duelPhaseBouts {
			respondErr(w, http.StatusConflict, "bout-declare is only allowed in 'bouts' phase")
			return
		}
		if state.InitiativePlayerID == nil || *state.InitiativePlayerID != player.ID {
			respondErr(w, http.StatusForbidden, "only the player with initiative may declare")
			return
		}

		// Ensure no unresolved bout exists.
		latest, latestErr := deps.Q.GetLatestDuelBout(r.Context(), plan.ID)
		if latestErr == nil && !latest.ResolvedAt.Valid {
			respondErr(w, http.StatusConflict, "a bout is already in progress — responder must respond first")
			return
		}

		var body struct {
			StakeID     int64  `json:"stake_id"`
			Declaration string `json:"declaration"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Declaration != string(gamepkg.DeclHigh) && body.Declaration != string(gamepkg.DeclLow) {
			respondErr(w, http.StatusBadRequest, "declaration must be 'high' or 'low'")
			return
		}

		ctx := r.Context()
		stake, err := deps.Q.GetDuelStake(ctx, body.StakeID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "stake not found")
			return
		}
		if stake.PlanID != plan.ID || stake.PlayerID != player.ID {
			respondErr(w, http.StatusForbidden, "that stake is not yours")
			return
		}
		if stake.IsResolved {
			respondErr(w, http.StatusConflict, "that stake has already been resolved")
			return
		}

		boutNumber := state.CurrentBout + 1
		_, err = deps.Q.CreateDuelBout(ctx, dbgen.CreateDuelBoutParams{
			PlanID:          plan.ID,
			BoutNumber:      boutNumber,
			DeclarerID:      player.ID,
			DeclarerStakeID: body.StakeID,
			ResponderID:     pduelOpponentID(plan, player.ID),
			Declaration:     &body.Declaration,
			DeclarerDie:     &stake.HiddenDie,
		})
		if err != nil {
			respondInternalErr(w, r, "could not create bout", err)
			return
		}

		state.CurrentBout = boutNumber
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save bout state", err)
			return
		}

		// The responder needs a duel event to refetch the bout list and render
		// the respond UI; broadcastRowState alone won't refresh their panel.
		broadcastEvent(deps.Manager, plan.GameID, model.EventDuelBoutDeclared, model.DuelBoutDeclaredPayload{
			PlanID:      plan.ID,
			BoutNumber:  boutNumber,
			ResponderID: pduelOpponentID(plan, player.ID),
		})
		// pduelLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
		// 	"Bout %d — %s opens with %s, calling %s. %s must answer.",
		// 	boutNumber, playerDisplayName(ctx, deps.Q, player.ID),
		// 	assetDisplayName(ctx, deps.Q, stake.AssetID), body.Declaration,
		// 	playerDisplayName(ctx, deps.Q, pduelOpponentID(plan, player.ID))))
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{
			"plan_id":     plan.ID,
			"bout_number": boutNumber,
			"responder":   pduelOpponentID(plan, player.ID),
		})
	}
}

// ── Bout Respond ──────────────────────────────────────────────────────────────

// pduelBoutRespondHandler: POST /api/plans/:planId/bout-respond
// Body: {"stake_id": N}
// The responder picks their stake. Server compares dice (via game.ResolveBout),
// records the outcome, swaps initiative, and — if this was the final bout —
// creates the standard dice roll with accumulated dice pre-assigned.
//
//nolint:funlen,gocognit // bout state machine (response/auto-advance/result)
func pduelBoutRespondHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanProposeDuel)
		if !ok {
			return
		}
		if !pduelIsParticipant(plan, player.ID) {
			respondErr(w, http.StatusForbidden, "only duellists may respond")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureDuel()
		if state.Phase != duelPhaseBouts {
			respondErr(w, http.StatusConflict, "bout-respond is only allowed in 'bouts' phase")
			return
		}

		ctx := r.Context()
		latest, err := deps.Q.GetLatestDuelBout(ctx, plan.ID)
		if err != nil {
			respondErr(w, http.StatusConflict, "no bout in progress")
			return
		}
		if latest.ResolvedAt.Valid {
			respondErr(w, http.StatusConflict, "most recent bout is already resolved")
			return
		}
		if latest.ResponderID != player.ID {
			respondErr(w, http.StatusForbidden, "you are not the responder for this bout")
			return
		}

		var body struct {
			StakeID int64 `json:"stake_id"`
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		respStake, err := deps.Q.GetDuelStake(ctx, body.StakeID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "stake not found")
			return
		}
		if respStake.PlanID != plan.ID || respStake.PlayerID != player.ID {
			respondErr(w, http.StatusForbidden, "that stake is not yours")
			return
		}
		if respStake.IsResolved {
			respondErr(w, http.StatusConflict, "that stake has already been resolved")
			return
		}

		decSide := pduelSide(plan, latest.DeclarerID)
		declDie := int16(0)
		if latest.DeclarerDie != nil {
			declDie = *latest.DeclarerDie
		}
		outcome, err := gamepkg.ResolveBout(
			decSide, gamepkg.Declaration(*latest.Declaration), declDie, respStake.HiddenDie,
		)
		if err != nil {
			respondInternalErr(w, r, "could not resolve bout", err)
			return
		}

		winnerPtr := gamepkg.BoutWinnerID(outcome, decSide, latest.DeclarerID, latest.ResponderID)

		err = deps.Q.ResolveDuelBout(ctx, dbgen.ResolveDuelBoutParams{
			ID:               latest.ID,
			ResponderStakeID: &respStake.ID,
			ResponderDie:     &respStake.HiddenDie,
			WinnerID:         winnerPtr,
			IsMatch:          outcome.Match,
		})
		if err != nil {
			respondInternalErr(w, r, "could not resolve bout", err)
			return
		}

		// Mark both stakes resolved. Track is_winner per stake only when a
		// stake's die ended up in the winner's accumulated pool.
		declarerIsWinner := winnerPtr != nil && *winnerPtr == latest.DeclarerID
		responderIsWinner := winnerPtr != nil && *winnerPtr == latest.ResponderID
		_ = deps.Q.SetDuelStakeResolved(ctx, dbgen.SetDuelStakeResolvedParams{
			ID: latest.DeclarerStakeID, IsWinner: &declarerIsWinner,
		})
		_ = deps.Q.SetDuelStakeResolved(ctx, dbgen.SetDuelStakeResolvedParams{
			ID: respStake.ID, IsWinner: &responderIsWinner,
		})

		// Swap initiative to the other side.
		nextInit := pduelOpponentID(plan, latest.DeclarerID)
		state.InitiativePlayerID = &nextInit

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			winID := int64(0)
			if winnerPtr != nil {
				winID = *winnerPtr
			}
			h.BroadcastEvent(model.EventDuelBoutResolved, model.DuelBoutResolvedPayload{
				PlanID:       plan.ID,
				BoutNumber:   latest.BoutNumber,
				DeclarerID:   latest.DeclarerID,
				ResponderID:  latest.ResponderID,
				DeclarerDie:  declDie,
				ResponderDie: respStake.HiddenDie,
				WinnerID:     winID,
				IsMatch:      outcome.Match,
			})
		}

		// Both dice are now public — narrate the exchange. A match means the
		// stakes clash and carry forward (see AccumulateDuelDice).
		declName := playerDisplayName(ctx, deps.Q, latest.DeclarerID)
		respName := playerDisplayName(ctx, deps.Q, latest.ResponderID)
		if outcome.Match {
			pduelLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
				"Bout %d: %s rolled %d, %s rolled %d — an even match. The stakes rise.",
				latest.BoutNumber, declName, declDie, respName, respStake.HiddenDie))
		} else {
			winName := declName
			if winnerPtr != nil && *winnerPtr == latest.ResponderID {
				winName = respName
			}
			pduelLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
				"Bout %d: %s rolled %d, %s rolled %d — %s takes the exchange.",
				latest.BoutNumber, declName, declDie, respName, respStake.HiddenDie, winName))
		}

		// Check remaining stakes.
		prepLeft, err := deps.Q.CountUnresolvedDuelStakes(ctx, dbgen.CountUnresolvedDuelStakesParams{
			PlanID: plan.ID, PlayerID: plan.PreparerID,
		})
		if err != nil {
			respondInternalErr(w, r, "could not count stakes", err)
			return
		}
		var targLeft int64
		if plan.TargetPlayerID != nil {
			targLeft, err = deps.Q.CountUnresolvedDuelStakes(ctx, dbgen.CountUnresolvedDuelStakesParams{
				PlanID: plan.ID, PlayerID: *plan.TargetPlayerID,
			})
			if err != nil {
				respondInternalErr(w, r, "could not count stakes", err)
				return
			}
		}

		if prepLeft == 0 || targLeft == 0 {
			// Bouts complete — create the standard roll with accumulated dice.
			if err := pduelCreateFinalRoll(ctx, w, r, deps, plan, &resData, state); err != nil {
				respondInternalErr(w, r, "could not create final roll", err)
				return
			}
		} else {
			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondInternalErr(w, r, "could not save state", err)
				return
			}
		}

		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{
			"plan_id":        plan.ID,
			"bout":           latest.BoutNumber,
			"winner_id":      winnerPtr,
			"is_match":       outcome.Match,
			"bouts_complete": prepLeft == 0 || targLeft == 0,
		})
	}
}

// pduelCreateFinalRoll accumulates winning dice by side and creates the
// plan's standard dice roll with dice pre-assigned. Preparer's accumulated
// dice become actor dice; target's accumulated dice become interference.
//
// Unlike a normal plan roll, the duel's final roll has no leverage window:
// the rules resolve it "using accumulated bout dice" with nothing else added
// ("Both leverage all staked assets" consumes the stakes afterwards in
// ApplyChoice — it does not add leverage dice here). Every die's face is
// already known from the bouts, so we broadcast roll.created (to mount the
// panel) and then finalizeRoll immediately. finalizeRoll preserves pre-set
// faces (see rollAndCancelDice) and lands the roll at stage='resolved', so
// players see the outcome in the standard dice panel with nothing to do.
func pduelCreateFinalRoll(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	state *gamepkg.DuelResolutionData,
) error {
	bouts, err := deps.Q.ListDuelBoutsByPlan(ctx, plan.ID)
	if err != nil {
		return fmt.Errorf("list bouts: %w", err)
	}
	// Build the domain snapshot (chronological bout views) and let the pure
	// rule accumulate winning dice, including tied-bout carry-over. See
	// game.AccumulateDuelDice for the carry-over semantics.
	views := make([]gamepkg.DuelBoutView, 0, len(bouts))
	for _, bt := range bouts {
		var winnerIsPrep *bool
		if bt.WinnerID != nil {
			v := *bt.WinnerID == plan.PreparerID
			winnerIsPrep = &v
		}
		views = append(views, gamepkg.DuelBoutView{
			DeclarerDie:      bt.DeclarerDie,
			ResponderDie:     bt.ResponderDie,
			IsMatch:          bt.IsMatch,
			WinnerIsPreparer: winnerIsPrep,
		})
	}
	prepDice, targDice := gamepkg.AccumulateDuelDice(views)

	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return fmt.Errorf("load game: %w", err)
	}
	difficulty, err := pduelHandler{}.ComputeDifficulty(ctx, deps.Q, plan, resData)
	if err != nil {
		return fmt.Errorf("compute difficulty: %w", err)
	}

	// Create the roll without the default 2 actor dice (createPlanRoll adds 2).
	// Here we create the roll row directly and add one die per accumulated face.
	rollRow, err := deps.Q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
		GameID:     game.ID,
		PlanID:     &plan.ID,
		RowNumber:  &game.CurrentRow,
		ActorID:    plan.PreparerID,
		Difficulty: difficulty,
		Stage:      "leverage",
	})
	if err != nil {
		return fmt.Errorf("create roll: %w", err)
	}

	for _, face := range prepDice {
		die, err := deps.Q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
			RollID: rollRow.ID, PlayerID: plan.PreparerID, IsInterference: false,
		})
		if err != nil {
			return err
		}
		f := face
		if err := deps.Q.SetDieFace(ctx, dbgen.SetDieFaceParams{ID: die.ID, Face: &f}); err != nil {
			return err
		}
	}
	for _, face := range targDice {
		oppID := plan.PreparerID
		if plan.TargetPlayerID != nil {
			oppID = *plan.TargetPlayerID
		}
		die, err := deps.Q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
			RollID: rollRow.ID, PlayerID: oppID, IsInterference: true,
		})
		if err != nil {
			return err
		}
		f := face
		if err := deps.Q.SetDieFace(ctx, dbgen.SetDieFaceParams{ID: die.ID, Face: &f}); err != nil {
			return err
		}
	}

	state.Phase = duelPhaseRoll
	if err := saveResolutionData(ctx, deps.Q, plan.ID, *resData); err != nil {
		return err
	}

	oppID := plan.PreparerID
	if plan.TargetPlayerID != nil {
		oppID = *plan.TargetPlayerID
	}
	pduelLog(ctx, deps, plan, model.SeverityImportant, fmt.Sprintf(
		"The bouts are settled — %s carries %d dice into the final roll, %s carries %d. Now fate decides.",
		playerDisplayName(ctx, deps.Q, plan.PreparerID), len(prepDice),
		playerDisplayName(ctx, deps.Q, oppID), len(targDice)))

	if h, ok := deps.Manager.Get(plan.GameID); ok {
		// Mount the panel first, then resolve below so clients receive
		// roll.created before roll.resolved.
		h.BroadcastEvent(model.EventRollCreated, model.RollCreatedPayload{Roll: rollRow})
		h.BroadcastEvent(model.EventDuelBoutsComplete, model.DuelBoutsCompletePayload{
			PlanID:       plan.ID,
			PreparerDice: prepDice,
			OpponentDice: targDice,
			RollID:       rollRow.ID,
		})
	}

	// No leverage window: resolve immediately. All faces are pre-set, so
	// finalizeRoll keeps them, applies interference cancellation, computes the
	// result/outcome, sets stage='resolved', and broadcasts roll.resolved. The
	// take-stakes make-choice flow then reads the outcome (phase stays 'roll'
	// until ApplyChoice advances it to 'done').
	if err := finalizeRoll(ctx, w, r, deps.Q, deps.Manager, &rollRow); err != nil {
		return fmt.Errorf("resolve duel roll: %w", err)
	}
	return nil
}
