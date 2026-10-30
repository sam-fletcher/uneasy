package handler

import (
	"context"
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

// hfInsistHostMarHandler spends a make-rolling guest's IOU. The book (p.61) has
// the guest "insist that the host choose one option from the mar list", so the
// insist carries no option: it only queues an unchosen mar for the host, who
// picks which of the four to take via resolve-host-mar. The guest's leverage is
// *when* they insist, not what they get.
func hfInsistHostMarHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanHostFestivity)
		if !ok {
			return
		}

		ctx := r.Context()
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "the event is over; mars can no longer be inflicted")
			return
		}
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureFestivity()
		if hfBlockWhilePaused(ctx, deps.Q, w, state) {
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
		state.UnchosenHostMars = append(state.UnchosenHostMars, player.ID)

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save insist", err)
			return
		}
		hfLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
			"%s insisted that %s take a mar — %s chooses which.",
			playerDisplayName(ctx, deps.Q, player.ID),
			playerDisplayName(ctx, deps.Q, plan.PreparerID), playerDisplayName(ctx, deps.Q, plan.PreparerID)))
		broadcastEvent(
			deps.Manager,
			plan.GameID,
			model.EventFestivityInsistHostMar,
			model.FestivityInsistHostMarPayload{PlanID: plan.ID, InsisterID: player.ID},
		)
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID})
	}
}

// hfChosenMarPhrase is the chat-log fragment for the mar the host chose when
// answering a guest's insistence ("… chose <phrase> in answer to …").
func hfChosenMarPhrase(marOption string) string {
	switch marOption {
	case gamepkg.FestivityMarRumorAboutYou:
		return "to have a rumor spread about them"
	case gamepkg.FestivityMarDisagreement:
		return "to fall out with one of their peers"
	case gamepkg.FestivityMarAcceptDuels:
		return "to accept any duel challenge for the rest of the event"
	case gamepkg.FestivityMarBreakSelf:
		return "to break themselves"
	}
	return "a mar"
}

// hfSettleHostMar clears the obligation a resolve-host-mar call just met. A
// legacy entry (insisterID 0: the insisting guest picked the option, before the
// host chose) is dropped from PendingHostMars; a current one leaves the
// unchosen queue and is recorded as inflicted, logging the host's choice.
func hfSettleHostMar(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	state *gamepkg.FestivityResolutionData,
	insisterID int64,
	marOption string,
) {
	if insisterID == 0 {
		state.RemovePendingHostMar(marOption)
		return
	}
	state.ConsumeUnchosenHostMar(insisterID)
	state.HostMarInsists = append(state.HostMarInsists, marOption)
	hfLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
		"%s chose %s in answer to %s's insistence.",
		playerDisplayName(ctx, deps.Q, plan.PreparerID), hfChosenMarPhrase(marOption),
		playerDisplayName(ctx, deps.Q, insisterID)))
}

// ── resolve-host-mar ──────────────────────────────────────────────────────────

// hfResolveHostMarHandler is where the host answers a guest's insistence. A
// guest's insist leaves an unchosen mar; the host picks which of the four mar
// options to take (insister_id names which insistence they are answering) and
// supplies whatever that option needs — rumor text, which peer falls out, which
// marginalia to tear. Host-only: a mar dealt to the host is the host's call.
//
// A request with no insister_id settles a legacy entry instead: a mar the
// *guest* chose under the old flow, which sits in PendingHostMars awaiting only
// the host's asset pick (break_self / disagreement). It is kept so a game in
// flight across the change can finish.
//

func hfResolveHostMarHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanHostFestivity)
		if !ok {
			return
		}
		// Settling a mar dealt to the host (which of the host's own assets to break
		// or which peer falls out) is the host's resolution step, so a Make Demands
		// perform_steps winner may drive it in the host's stead (locking them out).
		// The mar still lands on the host — hfApplyOption is passed plan.PreparerID.
		if !requireResolutionActor(w, r.Context(), deps.Q, plan, player.ID) {
			return
		}
		var body struct {
			InsisterID   int64  `json:"insister_id"`
			MarOption    string `json:"mar_option"`
			RumorText    string `json:"rumor_text"`
			MarginaliaID int64  `json:"marginalia_id"`
			AssetID      int64  `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !gamepkg.IsValidFestivityMarOption(body.MarOption) {
			respondErr(w, http.StatusBadRequest, "invalid mar option")
			return
		}
		legacy := body.InsisterID == 0
		if legacy && !gamepkg.MarNeedsHostResolution(body.MarOption) {
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
		if hfBlockWhilePaused(ctx, deps.Q, w, state) {
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
		if legacy && !slices.Contains(state.PendingHostMars, body.MarOption) {
			respondErr(w, http.StatusConflict, "you have no such mar to resolve")
			return
		}
		if !legacy && !slices.Contains(state.UnchosenHostMars, body.InsisterID) {
			respondErr(w, http.StatusConflict, "no insistence from that guest is waiting on your choice")
			return
		}

		// Soft-lock guard for break_self: if the host's main character can absorb
		// no break at all, there is nothing to land. Settle the debt as a no-op so
		// the event can still be wound down rather than blocking on the impossible.
		// A *blank* main character is not that case — the break destroys it
		// outright (D3) — so it falls through to hfApplyOption below.
		passed := false
		if body.MarOption == gamepkg.FestivityMarBreakSelf {
			canBreak, herr := hfHostCanBreakSelf(ctx, deps, plan)
			if herr != nil {
				respondInternalErr(w, r, "could not inspect host main character", herr)
				return
			}
			if !canBreak {
				hfLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
					"%s had nothing left to tear — the insisted break passes.",
					playerDisplayName(ctx, deps.Q, plan.PreparerID)))
				passed = true
			}
		}

		// The host is the actor and supplies the specifics themselves.
		if !passed {
			if err := hfApplyOption(ctx, deps, plan, &resData, festivityOptionInput{
				ActingPlayerID: plan.PreparerID,
				Choice:         body.MarOption,
				RumorText:      body.RumorText,
				AssetID:        body.AssetID,
				MarginaliaID:   body.MarginaliaID,
			}); err != nil {
				respondErr(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		hfSettleHostMar(ctx, deps, plan, state, body.InsisterID, body.MarOption)

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save resolved mar", err)
			return
		}
		hfBroadcastUpdated(deps, plan)
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{
			"plan_id":            plan.ID,
			"pending_host_mars":  state.PendingHostMars,
			"unchosen_host_mars": state.UnchosenHostMars,
		})
	}
}

// ── host-choice ──────────────────────────────────────────────────────────────

// The host spends one of their earned extra makes. These are the host's spoils
// (one for hosting, one per guest who marred or opted out), not debts owed to
// any particular guest — so there is no target to name for most options; the
// route just checks the host still has makes remaining and applies the option to
// the host themself. The one exception is challenge_duel, which names the guest
// being called out (target_player_id) and opens the same challenge a rolling
// guest's make would.
//

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
			DraftID        string   `json:"draft_id"`
			TargetPlayerID int64    `json:"target_player_id"`
			Notes          string   `json:"preparation_notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !gamepkg.IsValidFestivityMakeOption(body.Choice) {
			respondErr(w, http.StatusBadRequest, "invalid make option for festivity")
			return
		}
		notes, ok := textField(w, "preparation_notes", body.Notes, maxNarrativeLen)
		if !ok {
			return
		}

		ctx := r.Context()
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "the event is over")
			return
		}
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureFestivity()
		if hfBlockWhilePaused(ctx, deps.Q, w, state) {
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

		if body.Choice == gamepkg.FestivityMakeChallengeDuel {
			hfHostChallengeDuel(ctx, w, r, deps, plan, &resData, roster, body.TargetPlayerID, notes)
			return
		}

		// The make benefits the HOST: host-choice is make-only, so no marginalia
		// asset-tear (break is a mar option) — MarginaliaID stays 0. peer_marginalia
		// is only consumed by the introduce_peer option.
		if err := hfApplyOption(ctx, deps, plan, &resData, festivityOptionInput{
			ActingPlayerID: plan.PreparerID,
			Choice:         body.Choice,
			RumorText:      body.RumorText,
			PeerName:       body.PeerName,
			PeerMarginalia: body.PeerMarginalia,
			AssetID:        body.AssetID,
			DraftID:        body.DraftID,
			IsMake:         true,
		}); err != nil {
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

// hfHostChallengeDuel is the host spending an earned make on challenge_duel. The
// host is the challenger (the nested Propose Duel is theirs, even when a
// perform_steps winner chose the target for them) and, like a guest's, the make
// is spent whether or not the target accepts. The caller has already checked the
// host has a make left and that no roll-and-choice or challenge is in flight.
func hfHostChallengeDuel(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	roster []int64,
	targetID int64,
	notes string,
) {
	state := resData.EnsureFestivity()
	if targetID == 0 {
		respondErr(w, http.StatusBadRequest, "target_player_id required")
		return
	}
	if targetID == plan.PreparerID {
		respondErr(w, http.StatusBadRequest, "cannot duel yourself")
		return
	}
	if !slices.Contains(roster, targetID) {
		respondErr(w, http.StatusBadRequest, "challenge target must be a guest")
		return
	}
	state.HostMakesTaken = append(state.HostMakesTaken, gamepkg.FestivityMakeChallengeDuel)
	if err := hfOpenChallenge(ctx, deps, plan, resData, plan.PreparerID, targetID, notes); err != nil {
		respondInternalErr(w, r, "could not save challenge state", err)
		return
	}
	respond(w, http.StatusCreated, map[string]any{
		"plan_id":         plan.ID,
		"choice":          gamepkg.FestivityMakeChallengeDuel,
		"challenger_id":   plan.PreparerID,
		"target_id":       targetID,
		"must_accept":     state.HasAcceptDuels(targetID),
		"makes_remaining": state.RemainingHostMakes(roster),
	})
}

// hfNestedDuelInProgress reports whether a duel spawned from this festivity is
// still unresolved. A finished duel's id is cleared from state as a side effect
// (the caller persists it with its own write), so a spent duel never blocks the
// next challenge or roll.
func hfNestedDuelInProgress(ctx context.Context, q *dbgen.Queries, state *gamepkg.FestivityResolutionData) bool {
	if state.PendingDuelPlanID == nil {
		return false
	}
	if dp, err := q.GetPlanByID(ctx, *state.PendingDuelPlanID); err == nil && dp.Status != model.PlanResolved {
		return true
	}
	state.PendingDuelPlanID = nil
	return false
}

// hfOpenChallenge records a duel challenge awaiting the target's response —
// pausing every other festivity action until they accept or decline — then
// persists it, announces it and logs it. The caller has already recorded the
// challenger's spent make (a guest's choice or a host make), which is spent
// whatever the target answers.
func hfOpenChallenge(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	challengerID, targetID int64,
	notes string,
) error {
	state := resData.EnsureFestivity()
	state.PendingChallenge = &gamepkg.PendingChallenge{
		ChallengerID: challengerID,
		TargetID:     targetID,
		Notes:        notes,
	}
	if err := saveResolutionData(ctx, deps.Q, plan.ID, *resData); err != nil {
		return err
	}
	broadcastEvent(
		deps.Manager,
		plan.GameID,
		model.EventFestivityChallengeIssued,
		model.FestivityChallengeIssuedPayload{
			PlanID:       plan.ID,
			ChallengerID: challengerID,
			TargetID:     targetID,
			MustAccept:   state.HasAcceptDuels(targetID),
		},
	)
	hfLog(ctx, deps, plan, model.SeverityImportant, fmt.Sprintf(
		"%s threw down a challenge across the hall, calling out %s to a duel.",
		playerDisplayName(ctx, deps.Q, challengerID),
		playerDisplayName(ctx, deps.Q, targetID)))
	broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
	return nil
}

// ── challenge-duel ───────────────────────────────────────────────────────────

// A guest who rolled make uses this route (instead of guest-choice) to invoke
// the challenge_duel option. (The host spends an earned make on a duel through
// host-choice instead — see hfHostChallengeDuel.) The target is anyone in the guest list — they may
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
		if hfBlockWhilePaused(ctx, deps.Q, w, state) {
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
		if err := hfOpenChallenge(ctx, deps, plan, &resData, player.ID, body.TargetPlayerID, body.Notes); err != nil {
			respondInternalErr(w, r, "could not save challenge state", err)
			return
		}
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
