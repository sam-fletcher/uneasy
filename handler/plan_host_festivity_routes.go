package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// ── insist-host-mar ──────────────────────────────────────────────────────────

func hfInsistHostMarHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanHostFestivity)
		if !ok {
			return
		}
		var body struct {
			MarOption    string `json:"mar_option"`
			AssetID      int64  `json:"asset_id"`
			MarginaliaID int64  `json:"marginalia_id"`
			RumorText    string `json:"rumor_text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !gamepkg.IsValidFestivityMarOption(body.MarOption) {
			respondErr(w, http.StatusBadRequest, "invalid mar option")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureFestivity()
		if state.Phase == gamepkg.FestivityPhaseDone {
			respondErr(w, http.StatusConflict, "event is over; IOUs can no longer be cashed")
			return
		}
		if hfBlockOnPendingChallenge(w, state) {
			return
		}
		if !state.ConsumeIOU(player.ID) {
			respondErr(w, http.StatusForbidden, "you have no IOU to cash (must have rolled a make)")
			return
		}

		// Apply the mar effect to the host. Note: a guest insisting break_self on
		// the host cannot pick the host's marginalia for them — marginaliaID 0
		// falls back to the host's first intact marginalia.
		if err := hfApplyOption(ctx, deps, plan, state, plan.PreparerID,
			body.MarOption, body.RumorText, "", body.AssetID, body.MarginaliaID, false); err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}
		state.HostMarInsists = append(state.HostMarInsists, body.MarOption)

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save insist", err)
			return
		}
		broadcastEvent(
			deps.Manager,
			plan.GameID,
			model.EventFestivityInsistHostMar,
			model.FestivityInsistHostMarPayload{
				PlanID: plan.ID, InsisterID: player.ID, MarOption: body.MarOption,
			},
		)
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "mar_option": body.MarOption})
	}
}

// ── host-choice ──────────────────────────────────────────────────────────────

func hfHostChoiceHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanHostFestivity)
		if !ok {
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the host may submit host-choice")
			return
		}
		var body struct {
			TargetPlayerID int64  `json:"target_player_id"`
			Choice         string `json:"choice"`
			RumorText      string `json:"rumor_text"`
			PeerName       string `json:"peer_name"`
			AssetID        int64  `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TargetPlayerID == 0 {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !gamepkg.IsValidFestivityMakeOption(body.Choice) || body.Choice == gamepkg.FestivityMakeChallengeDuel {
			respondErr(w, http.StatusBadRequest, "host-choice must be a non-duel make option")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureFestivity()
		if state.Phase != gamepkg.FestivityPhaseHostChoosing {
			respondErr(w, http.StatusConflict, "host-choice is only allowed during the host_choosing phase")
			return
		}
		if hfBlockOnPendingChallenge(w, state) {
			return
		}
		tk := strconv.FormatInt(body.TargetPlayerID, 10)
		oc, ok := state.Outcomes[tk]
		if !ok || (oc != gamepkg.FestivityOutcomeMar && oc != gamepkg.FestivityOutcomeOptOut) {
			respondErr(w, http.StatusBadRequest, "target guest did not roll mar or opt out")
			return
		}
		if _, done := state.HostChoices[tk]; done {
			respondErr(w, http.StatusConflict, "host has already chosen for this guest")
			return
		}

		// The make benefits the HOST: per the rules, for each guest who rolled a
		// mar or opted out, the host takes a make for themself. body.TargetPlayerID
		// only identifies which owed slot is being filled (recorded in
		// HostChoices below) — the effect's actor is the host. Host-choice is
		// make-only, so no marginalia (break is a mar option) — pass 0.
		if err := hfApplyOption(ctx, deps, plan, state, plan.PreparerID,
			body.Choice, body.RumorText, body.PeerName, body.AssetID, 0, true); err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if state.HostChoices == nil {
			state.HostChoices = map[string]string{}
		}
		state.HostChoices[tk] = body.Choice

		roster, err := hfRoster(ctx, deps.Q, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load roster", err)
			return
		}
		if err := hfFinalizeIfDone(ctx, deps, plan, state, roster); err != nil {
			respondInternalErr(w, r, "could not finalize", err)
			return
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save host choice", err)
			return
		}
		broadcastEvent(deps.Manager, plan.GameID, model.EventFestivityHostChose, model.FestivityHostChosePayload{
			PlanID: plan.ID, GuestPlayerID: body.TargetPlayerID, Choice: body.Choice,
		})
		if state.Phase == gamepkg.FestivityPhaseDone {
			hfBroadcastPhase(deps, plan, string(state.Phase))
		}
		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID, "target_player_id": body.TargetPlayerID, "choice": body.Choice, "phase": state.Phase,
		})
	}
}

// ── challenge-duel ───────────────────────────────────────────────────────────

// A guest who rolled make uses this route (instead of guest-choice) to invoke
// the challenge_duel option. The target is anyone in the guest list — they may
// accept or decline via /respond-challenge. Targets who have taken the
// accept_duels mar cannot decline (the UI should disable that option). The
// challenger's make is spent on challenge_duel regardless of the target's
// response. All festivity game actions pause while PendingChallenge is set.
func hfChallengeDuelHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanHostFestivity)
		if !ok {
			return
		}
		var body struct {
			TargetPlayerID int64  `json:"target_player_id"`
			Notes          string `json:"preparation_notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TargetPlayerID == 0 {
			respondErr(w, http.StatusBadRequest, "target_player_id required")
			return
		}
		if body.TargetPlayerID == player.ID {
			respondErr(w, http.StatusBadRequest, "cannot duel yourself")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureFestivity()
		// The challenger is a guest by construction (every player attends); the
		// target just has to be a real player at the table.
		roster, err := hfRoster(ctx, deps.Q, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load roster", err)
			return
		}
		if !slices.Contains(roster, body.TargetPlayerID) {
			respondErr(w, http.StatusBadRequest, "challenge target must be a guest")
			return
		}
		// Caller must have rolled make.
		ck := strconv.FormatInt(player.ID, 10)
		if state.Outcomes[ck] != gamepkg.FestivityOutcomeMake {
			respondErr(w, http.StatusForbidden, "challenge_duel requires rolling a make")
			return
		}
		if state.PendingDuelPlanID != nil {
			respondErr(w, http.StatusConflict, "a nested duel is already in progress")
			return
		}
		if state.PendingChallenge != nil {
			respondErr(w, http.StatusConflict, "a challenge is already awaiting a response")
			return
		}

		// Record the challenger's make pick up front: challenge_duel is spent
		// whether or not the target accepts (per house rule).
		if state.Outcomes == nil {
			state.Outcomes = map[string]string{}
		}
		if state.GuestMakes == nil {
			state.GuestMakes = map[string]string{}
		}
		if _, already := state.GuestMakes[ck]; !already {
			state.Outcomes[ck] = gamepkg.FestivityOutcomeMake
			state.GuestMakes[ck] = gamepkg.FestivityMakeChallengeDuel
			state.GuestIOUs = append(state.GuestIOUs, player.ID)
		}
		state.PendingChallenge = &gamepkg.PendingChallenge{
			ChallengerID: player.ID,
			TargetID:     body.TargetPlayerID,
			Notes:        body.Notes,
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save challenge state", err)
			return
		}
		broadcastEvent(
			deps.Manager,
			plan.GameID,
			model.EventFestivityChallengeIssued,
			model.FestivityChallengeIssuedPayload{
				PlanID:       plan.ID,
				ChallengerID: player.ID,
				TargetID:     body.TargetPlayerID,
				MustAccept:   state.HasAcceptDuels(body.TargetPlayerID),
			},
		)
		hfLog(ctx, deps, plan, model.SeverityImportant, fmt.Sprintf(
			"%s threw down a challenge across the hall, calling out %s to a duel.",
			playerDisplayName(ctx, deps.Q, player.ID),
			playerDisplayName(ctx, deps.Q, body.TargetPlayerID)))
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusCreated, map[string]any{
			"plan_id":       plan.ID,
			"challenger_id": player.ID,
			"target_id":     body.TargetPlayerID,
			"must_accept":   state.HasAcceptDuels(body.TargetPlayerID),
		})
	}
}

// ── respond-challenge ────────────────────────────────────────────────────────

// Body: {"accept": true|false}. Only the challenge's target may call.
// Decline is refused if the target has the accept_duels mar.
//
//nolint:funlen // decline + accept (nested-duel spawn) paths in one handler
func hfRespondChallengeHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanHostFestivity)
		if !ok {
			return
		}
		var body struct {
			Accept bool `json:"accept"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureFestivity()
		pc := state.PendingChallenge
		if pc == nil {
			respondErr(w, http.StatusConflict, "no challenge is awaiting a response")
			return
		}
		if player.ID != pc.TargetID {
			respondErr(w, http.StatusForbidden, "only the challenged player may respond")
			return
		}
		if !body.Accept && state.HasAcceptDuels(player.ID) {
			respondErr(w, http.StatusConflict,
				"you have taken the accept_duels mar and cannot decline")
			return
		}

		if !body.Accept {
			// Challenger's make is already spent; just clear the challenge.
			challengerID := pc.ChallengerID
			state.PendingChallenge = nil
			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondInternalErr(w, r, "could not save decline", err)
				return
			}
			broadcastEvent(
				deps.Manager,
				plan.GameID,
				model.EventFestivityChallengeDeclined,
				model.FestivityChallengeDeclinedPayload{
					PlanID: plan.ID, ChallengerID: challengerID, TargetID: player.ID,
				},
			)
			hfLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
				"%s waved off %s's challenge, and the moment passed.",
				playerDisplayName(ctx, deps.Q, player.ID),
				playerDisplayName(ctx, deps.Q, challengerID)))
			broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
			respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "accepted": false})
			return
		}

		// Accept: spawn the nested duel plan and advance it to resolving.
		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load game", err)
			return
		}
		notes := pc.Notes
		if notes == "" {
			notes = "Duel triggered by festivity"
		}
		challengerID := pc.ChallengerID
		duelPlan, err := deps.Q.CreatePlan(ctx, dbgen.CreatePlanParams{
			GameID:           plan.GameID,
			PlanType:         model.PlanProposeDuel,
			Category:         model.CategoryEsteem,
			PreparerID:       challengerID,
			TargetPlayerID:   &player.ID,
			RowNumber:        new(game.CurrentRow),
			RowOrder:         0,
			PreparedAtRow:    game.CurrentRow,
			PreparationNotes: &notes,
		})
		if err != nil {
			respondInternalErr(w, r, "could not create nested duel", err)
			return
		}
		hfLog(ctx, deps, plan, model.SeverityImportant, fmt.Sprintf(
			"%s took up %s's challenge — steel will settle it.",
			playerDisplayName(ctx, deps.Q, player.ID),
			playerDisplayName(ctx, deps.Q, challengerID)))
		// The nested duel is created directly (not via PreparePlan), so emit its
		// "prepared" post by hand — otherwise the spawned duel would surface in
		// the log only at resolution, with no opening beat.
		EmitPlanPrepared(ctx, deps.Q, deps.Manager, duelPlan)
		if err := deps.Q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
			ID: duelPlan.ID, Status: model.PlanResolving,
		}); err != nil {
			respondInternalErr(w, r, "could not resolve nested duel", err)
			return
		}
		if h, ok := GetHandler(model.PlanProposeDuel); ok {
			refreshed, _ := deps.Q.GetPlanByID(ctx, duelPlan.ID)
			if _, err := h.OnResolve(ctx, deps, &refreshed); err != nil {
				respondInternalErr(w, r, "duel setup failed", err)
				return
			}
		}

		state.PendingChallenge = nil
		state.PendingDuelPlanID = &duelPlan.ID
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save accept", err)
			return
		}
		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventFestivityDuelTriggered, model.FestivityDuelTriggeredPayload{
				PlanID: plan.ID, DuelPlanID: duelPlan.ID,
			})
			h.BroadcastEvent(model.EventPlanResolving, model.PlanPayload{Plan: duelPlan})
		}
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusCreated, map[string]any{
			"plan_id": plan.ID, "duel_plan_id": duelPlan.ID, "accepted": true,
		})
	}
}
