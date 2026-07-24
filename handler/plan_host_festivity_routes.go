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
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "the event is over; mars can no longer be inflicted")
			return
		}
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureFestivity()
		if hfBlockOnPendingChallenge(w, state) {
			return
		}
		roster, err := hfRoster(ctx, deps.Q, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load roster", err)
			return
		}
		if hfBlockOnActiveRoll(w, state, roster, player.ID) {
			return
		}
		if !state.ConsumeIOU(player.ID) {
			respondErr(w, http.StatusForbidden, "you have no mar to inflict (must have rolled a make)")
			return
		}

		// Some mars can't be applied by the insisting guest: they hinge on a choice
		// about the host's OWN assets (which marginalia to tear for break_self,
		// which peer falls out for disagreement). Those only record an obligation
		// the host settles via resolve-host-mar; every other mar applies at once.
		if gamepkg.MarNeedsHostResolution(body.MarOption) {
			state.PendingHostMars = append(state.PendingHostMars, body.MarOption)
			hfLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
				"%s insisted the host %s — the host must choose how.",
				playerDisplayName(ctx, deps.Q, player.ID), hfInsistedMarPhrase(body.MarOption)))
		} else if err := hfApplyOption(ctx, deps, plan, state, plan.PreparerID,
			body.MarOption, body.RumorText, "", nil, body.AssetID, body.MarginaliaID, false); err != nil {
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
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "mar_option": body.MarOption})
	}
}

// hfInsistedMarPhrase is the chat-log fragment for a mar the host must resolve
// themselves, used when a guest insists it ("… insisted the host <phrase>").
func hfInsistedMarPhrase(marOption string) string {
	switch marOption {
	case gamepkg.FestivityMarBreakSelf:
		return "break themselves"
	case gamepkg.FestivityMarDisagreement:
		return "fall out with one of their peers"
	}
	return "take a mar"
}

// ── resolve-host-mar ──────────────────────────────────────────────────────────

// hfResolveHostMarHandler lets the host settle a mar a guest insisted on them
// that hinges on a choice about the host's own assets — break_self (which
// marginalia on their main character to tear) and disagreement (which of their
// peers falls out). The insisting guest can't make these calls; only the owner
// can. Host-only, and guarded by the pending-mar queue so it can't be called
// more times than were insisted.
func hfResolveHostMarHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanHostFestivity)
		if !ok {
			return
		}
		// Settling a mar dealt to the host (which of the host's own assets to break
		// or which peer falls out) is the host's resolution step, so a Make Demands
		// perform_steps winner may drive it in the host's stead (locking them out).
		// The break still lands on the host — hfApplyOption is passed plan.PreparerID.
		if !requireResolutionActor(w, r.Context(), deps.Q, plan, player.ID) {
			return
		}
		var body struct {
			MarOption    string `json:"mar_option"`
			MarginaliaID int64  `json:"marginalia_id"`
			AssetID      int64  `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !gamepkg.MarNeedsHostResolution(body.MarOption) {
			respondErr(w, http.StatusBadRequest, "that mar is not one the host resolves")
			return
		}

		ctx := r.Context()
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "the event is over")
			return
		}
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureFestivity()
		if hfBlockOnPendingChallenge(w, state) {
			return
		}
		roster, err := hfRoster(ctx, deps.Q, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load roster", err)
			return
		}
		if hfBlockOnActiveRoll(w, state, roster, player.ID) {
			return
		}
		if !slices.Contains(state.PendingHostMars, body.MarOption) {
			respondErr(w, http.StatusConflict, "you have no such mar to resolve")
			return
		}

		// Soft-lock guard for break_self: if the host's main character can absorb
		// no break at all, there is nothing to land. Settle the debt as a no-op so
		// the event can still be wound down rather than blocking on the impossible.
		// A *blank* main character is not that case — the break destroys it
		// outright (D3) — so it falls through to hfApplyOption below.
		if body.MarOption == gamepkg.FestivityMarBreakSelf {
			canBreak, herr := hfHostCanBreakSelf(ctx, deps, plan)
			if herr != nil {
				respondInternalErr(w, r, "could not inspect host main character", herr)
				return
			}
			if !canBreak {
				hfLog(ctx, deps, plan, model.SeverityDefault,
					"The host had nothing left to tear — the insisted break passes.")
				state.RemovePendingHostMar(body.MarOption)
				if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
					respondInternalErr(w, r, "could not save resolved mar", err)
					return
				}
				hfBroadcastUpdated(deps, plan)
				broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
				respond(w, http.StatusOK, map[string]any{
					"plan_id": plan.ID, "pending_host_mars": state.PendingHostMars,
				})
				return
			}
		}

		// The host is the actor and chooses the asset themselves (marginalia for a
		// break, peer for a disagreement).
		if err := hfApplyOption(ctx, deps, plan, state, plan.PreparerID,
			body.MarOption, "", "", nil, body.AssetID, body.MarginaliaID, false); err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}
		state.RemovePendingHostMar(body.MarOption)

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save resolved mar", err)
			return
		}
		hfBroadcastUpdated(deps, plan)
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID, "pending_host_mars": state.PendingHostMars,
		})
	}
}

// ── host-choice ──────────────────────────────────────────────────────────────

// The host spends one of their earned extra makes. These are the host's spoils
// (one for hosting, one per guest who marred or opted out), not debts owed to
// any particular guest — so there is no target; the route just checks the host
// still has makes remaining and applies the option to the host themself.
func hfHostChoiceHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanHostFestivity)
		if !ok {
			return
		}
		// Taking a host make is the host's (preparer's) resolution step, so a Make
		// Demands perform_steps winner may drive it in their stead (locking the host
		// out). The make's effects still land on the host — hfApplyOption is passed
		// plan.PreparerID below, not the caller.
		if !requireResolutionActor(w, r.Context(), deps.Q, plan, player.ID) {
			return
		}
		var body struct {
			Choice         string   `json:"choice"`
			RumorText      string   `json:"rumor_text"`
			PeerName       string   `json:"peer_name"`
			PeerMarginalia []string `json:"peer_marginalia"`
			AssetID        int64    `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !gamepkg.IsValidFestivityMakeOption(body.Choice) || body.Choice == gamepkg.FestivityMakeChallengeDuel {
			respondErr(w, http.StatusBadRequest, "host-choice must be a non-duel make option")
			return
		}

		ctx := r.Context()
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "the event is over")
			return
		}
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureFestivity()
		if hfBlockOnPendingChallenge(w, state) {
			return
		}
		roster, err := hfRoster(ctx, deps.Q, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load roster", err)
			return
		}
		if hfBlockOnActiveRoll(w, state, roster, player.ID) {
			return
		}
		if state.RemainingHostMakes(roster) <= 0 {
			respondErr(w, http.StatusConflict, "you have no extra makes left to take")
			return
		}

		// The make benefits the HOST: host-choice is make-only, so no marginalia
		// asset-tear (break is a mar option) — pass 0. peer_marginalia is only
		// consumed by the introduce_peer option.
		if err := hfApplyOption(ctx, deps, plan, state, plan.PreparerID,
			body.Choice, body.RumorText, body.PeerName, body.PeerMarginalia, body.AssetID, 0, true); err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}
		state.HostMakesTaken = append(state.HostMakesTaken, body.Choice)

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save host make", err)
			return
		}
		broadcastEvent(deps.Manager, plan.GameID, model.EventFestivityHostChose, model.FestivityHostChosePayload{
			PlanID: plan.ID, Choice: body.Choice,
		})
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID, "choice": body.Choice,
			"makes_remaining": state.RemainingHostMakes(roster),
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
		notes, ok := textField(w, "preparation_notes", body.Notes, maxNarrativeLen)
		if !ok {
			return
		}
		body.Notes = notes
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
		// The challenger concluding their own roll-and-choice is exempt; a
		// different guest mid-roll-and-choice blocks this.
		if hfBlockOnActiveRoll(w, state, roster, player.ID) {
			return
		}
		if !slices.Contains(roster, body.TargetPlayerID) {
			respondErr(w, http.StatusBadRequest, "challenge target must be a guest")
			return
		}
		// Caller must have rolled a make and not yet spent it. A guest reaches
		// challenge_duel mid roll-and-choice: their roll has resolved to make, but
		// state.Outcomes[ck] is only written once the make is spent (here, or via
		// guest-choice). So an existing Outcomes entry means they have already
		// acted; otherwise the authority for "rolled make" is the resolved roll.
		ck := strconv.FormatInt(player.ID, 10)
		if _, acted := state.Outcomes[ck]; acted {
			respondErr(w, http.StatusConflict, "you have already submitted your choice")
			return
		}
		rollID, ok := state.GuestRollIDs[ck]
		if !ok {
			respondErr(w, http.StatusForbidden, "challenge_duel requires rolling a make")
			return
		}
		roll, err := deps.Q.GetDiceRollByID(ctx, rollID)
		if err != nil {
			respondInternalErr(w, r, "could not load roll", err)
			return
		}
		if roll.Outcome == nil {
			respondErr(w, http.StatusConflict, "your roll has not resolved yet")
			return
		}
		if *roll.Outcome != makeOutcome {
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
