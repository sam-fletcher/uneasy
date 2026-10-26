package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// ── Advance Liaise ────────────────────────────────────────────────────────────

// clAdvanceLiaiseHandler handles POST /api/plans/:planId/advance-liaise.
//
// The preparer advances the liaise to the next phase. Valid transitions:
//
//	together_at_last → secrets_we_keep
//	secrets_we_keep  → things_we_share   (only after both keep-secret submitted)
//	things_we_share  → when_will_i_see_you_again (only after both share-choice submitted)
func clAdvanceLiaiseHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, plan, ok := requirePlanPreparer(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanClandestinelyLiaise {
			respondErr(w, http.StatusBadRequest, "advance-liaise is only for Clandestinely Liaise")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		ld := resData.EnsureLiaise()

		var nextPhase LiaisePhase
		// The terminal phases (WhenWillISeeYouAgain, Done) fall through to
		// default — advancing from them is intentionally an error.
		//nolint:exhaustive // terminal phases handled by default
		switch ld.Phase {
		case LiaiseTogetherAtLast:
			nextPhase = LiaiseSecretsWeKeep
		case LiaiseSecretsWeKeep:
			// Both players must have submitted keep-secret before advancing.
			if !clBothKeepSecretsSubmitted(*ld, plan.PreparerID) {
				respondErr(w, http.StatusConflict, "both players must submit keep-secret before advancing")
				return
			}
			nextPhase = LiaiseThingsWeShare
		case LiaiseThingsWeShare:
			// Both players must have submitted share-choice before advancing.
			count, err := deps.Q.CountLiaiseChoicesByPlan(ctx, plan.ID)
			if err != nil || count < 2 {
				respondErr(w, http.StatusConflict, "both players must submit share-choice before advancing")
				return
			}
			nextPhase = LiaiseWhenWillISeeYouAgain

			// Create the redelay reveal row if not already created.
			if err := clEnsureRedelayReveal(ctx, deps, plan, ld); err != nil {
				respondInternalErr(w, r, "could not create redelay reveal", err)
				return
			}
		default:
			respondErr(w, http.StatusConflict, fmt.Sprintf("cannot advance from phase %q", ld.Phase))
			return
		}

		ld.Phase = nextPhase

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save liaise phase", err)
			return
		}

		broadcastEvent(deps.Manager, plan.GameID, model.EventLiaisePhaseChanged, model.LiaisePhaseChangedPayload{
			PlanID: plan.ID,
			Phase:  string(nextPhase),
		})

		// The new phase changes who the table is waiting on (e.g.
		// together_at_last names only the preparer, but secrets_we_keep and
		// things_we_share name both participants). LiaisePhaseChanged alone only
		// refetches the plan on clients — it does not recompute row state — so
		// the WaitingOnBar would stay stale until a manual refresh. Recompute
		// and broadcast row state, as every other multi-actor sub-phase does.
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID,
			"phase":   nextPhase,
		})
	}
}

// clBothKeepSecretsSubmitted checks whether both participants have submitted
// their keep-secret choice.
func clBothKeepSecretsSubmitted(ld LiaiseResolutionData, preparerID int64) bool {
	prepSubmitted := false
	partnerSubmitted := false
	for _, ks := range ld.KeptSecrets {
		if ks.PlayerID == preparerID {
			prepSubmitted = true
		} else if ld.PartnerID != nil && ks.PlayerID == *ld.PartnerID {
			partnerSubmitted = true
		}
	}
	return prepSubmitted && partnerSubmitted
}

// ── Keep Secret ───────────────────────────────────────────────────────────────

// clKeepSecretHandler handles POST /api/plans/:planId/keep-secret.
//
// Phase 2 — Secrets We Keep: each seat (the preparer's and the partner's)
// nominates one of its own assets to hold the secret of this meeting. The server
// writes a new secret on the asset's underside. Choices are revealed to both
// players once both have submitted.
//
// Which seat a submission fills is read off the asset: the seat is its owner. That
// is what lets a perform_steps winner stand in for the preparer (they nominate
// one of the PREPARER's assets, recorded under the preparer's seat) and what lets
// a winner who is also the partner fill both seats, one asset each. See clSeatsFor.
// TODO: Should this reject destroyed assets? The UI is hiding them but what's the right call here?
//
// Request body: {"asset_id": N}
func clKeepSecretHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanClandestinelyLiaise {
			respondErr(w, http.StatusBadRequest, "keep-secret is only for Clandestinely Liaise")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		ld := resData.EnsureLiaise()

		if ld.Phase != LiaiseSecretsWeKeep {
			respondErr(w, http.StatusConflict, fmt.Sprintf("keep-secret requires phase %q, currently %q",
				LiaiseSecretsWeKeep, ld.Phase))
			return
		}

		// Only a seat-holder may submit: the partner, or whoever holds the
		// preparer's slot (the preparer, unless a perform_steps winner stands in).
		seats, ok := clRequireSeats(w, ctx, deps.Q, plan, ld, player.ID, "keep-secret")
		if !ok {
			return
		}

		var body struct {
			AssetID int64 `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AssetID == 0 {
			respondErr(w, http.StatusBadRequest, "asset_id is required")
			return
		}

		asset, err := deps.Q.GetAssetByID(ctx, body.AssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "asset not found")
			return
		}
		if asset.GameID != plan.GameID {
			respondErr(w, http.StatusBadRequest, "asset does not belong to this game")
			return
		}
		// The asset's owner is the seat this fills; the caller must hold it.
		seat := asset.OwnerID
		if !slices.Contains(seats, seat) {
			respondErr(w, http.StatusForbidden, "that asset belongs to a seat you are not filling")
			return
		}

		// Reject a duplicate submission for this seat. The UI hides the form after
		// submitting (it derives "already kept" from the server-side kept_secrets),
		// so a stale client, a retry, or a direct API call is the only way to reach
		// here a second time — and without this guard it would write a second
		// secret on a second asset and append a duplicate KeptSecrets entry. Mirror
		// the share-choice idempotence guard below and Spread Rumors' principle
		// that a stale client can't write a duplicate.
		if slices.ContainsFunc(ld.KeptSecrets, func(ks KeptSecret) bool { return ks.PlayerID == seat }) {
			respondErr(w, http.StatusConflict, "this seat has already submitted a keep-secret")
			return
		}

		// Write a secret on the asset's underside.
		var preparationNotes string
		if plan.PreparationNotes != nil {
			preparationNotes = *plan.PreparationNotes
		}
		// Plain: secret text is read on the asset's underside, not in the chat
		// log, so it must not carry log markup.
		secretText := fmt.Sprintf("Clandestine meeting with %s — %s",
			playerPlainName(ctx, deps.Q, seat), preparationNotes)
		// Authored by the SEAT, not the caller: a stand-in's keystrokes are the
		// preparer's meeting, and authorship is what lets the asset's own player read
		// the secret on its underside.
		if _, err := deps.Q.CreateSecret(ctx, dbgen.CreateSecretParams{
			AssetID:  body.AssetID,
			AuthorID: seat,
			Text:     secretText,
		}); err != nil {
			respondInternalErr(w, r, "could not write secret on asset", err)
			return
		}

		// Record keep-secret choice.
		ld.KeptSecrets = append(ld.KeptSecrets, KeptSecret{
			PlayerID: seat,
			AssetID:  body.AssetID,
		})

		// Once both participants have nominated an asset, advance straight to
		// Things We Share. Secrets We Keep has no post-submission decision — each
		// pick is already logged as it lands — so a manual "Advance" click would be
		// pure friction (and left the second submitter staring at a "Waiting for …"
		// with nobody named). The phase change is server-authoritative, so a
		// refresh can't re-prompt the keep-secret form.
		bothKept := clBothKeepSecretsSubmitted(*ld, plan.PreparerID)
		if bothKept {
			ld.Phase = LiaiseThingsWeShare
		}

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save keep-secret choice", err)
			return
		}

		clLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf("%s entrusted the meeting's secret to %s.",
			playerDisplayName(ctx, deps.Q, seat), assetMark(asset.Name)))

		// The other participant must refetch the plan to see this submission.
		// Without this they'd be soft-locked on the secrets-we-keep panel until a
		// manual refresh.
		broadcastEvent(deps.Manager, plan.GameID, model.EventLiaiseKeepSecretSubmitted,
			model.LiaiseKeepSecretSubmittedPayload{PlanID: plan.ID})

		// When both are in we auto-advanced above; tell clients so their panels
		// move to Things We Share live (LiaisePhaseChanged drives a plan refetch).
		if bothKept {
			broadcastEvent(deps.Manager, plan.GameID, model.EventLiaisePhaseChanged,
				model.LiaisePhaseChangedPayload{PlanID: plan.ID, Phase: string(LiaiseThingsWeShare)})
		}

		// This submission narrows the acting set (the submitter no longer owes a
		// keep-secret; once both are in the phase moves on and names both share
		// pickers). KeepSecretSubmitted/PhaseChanged only trigger a plan refetch on
		// clients, not a row state recompute, so without this the WaitingOnBar
		// keeps naming the submitter until a manual refresh — the reported bug.
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":  plan.ID,
			"asset_id": body.AssetID,
			"seat":     seat,
		})
	}
}

// ── Share Choice ──────────────────────────────────────────────────────────────

// clShareChoiceHandler handles POST /api/plans/:planId/share-choice.
//
// Phase 3 — Things We Share: both players simultaneously choose one option.
// Choices are stored and revealed when both have submitted.
//
// Request body:
//
//	{"choice": "look_at_secret|update_peer|break_peer|take_gift|leverage_partner",
//	 "target_asset_id": N,  // required for look_at_secret, take_gift, leverage_partner
//	 "die_face": N}         // required for leverage_partner (1–6)
//

func clShareChoiceHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanClandestinelyLiaise {
			respondErr(w, http.StatusBadRequest, "share-choice is only for Clandestinely Liaise")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		ld := resData.EnsureLiaise()

		if ld.Phase != LiaiseThingsWeShare {
			respondErr(w, http.StatusConflict, fmt.Sprintf("share-choice requires phase %q", LiaiseThingsWeShare))
			return
		}
		// The Things We Share seats the caller may fill (writes a 403 and returns
		// ok=false if none). The partner fills their own; the preparer's belongs to
		// the resolution actor, so a perform_steps winner stands in (and the
		// preparer is locked out). A caller holding both seats — the demander is
		// also the partner — is told apart by whose assets they target, below.
		seats, ok := clRequireSeats(w, ctx, deps.Q, plan, ld, player.ID, "share-choice")
		if !ok {
			return
		}

		var body struct {
			Choice        string `json:"choice"`
			TargetAssetID *int64 `json:"target_asset_id"`
			TargetMargID  *int64 `json:"target_marginalia_id"`
			UpdateText    string `json:"update_text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Choice == "" {
			respondErr(w, http.StatusBadRequest, "choice is required")
			return
		}
		updateText, ok := textField(w, "update_text", body.UpdateText, maxMarginaliaLen)
		if !ok {
			return
		}
		body.UpdateText = updateText

		validChoices := []string{
			liaiseChoiceLookAtSecret, liaiseChoiceUpdatePeer, liaiseChoiceBreakPeer,
			liaiseChoiceTakeGift, liaiseChoiceLeveragePartner,
		}
		if !slices.Contains(validChoices, body.Choice) {
			respondErr(w, http.StatusBadRequest, fmt.Sprintf("choice must be one of: %v", validChoices))
			return
		}

		// The seat this fills. One seat leaves no choice; a caller holding both picks
		// by target, since every option targets the OTHER seat's assets.
		slotID, ok := clShareSeat(ctx, deps.Q, seats, plan, ld, body.TargetAssetID)
		if !ok {
			respondErr(w, http.StatusBadRequest,
				"target_asset_id must belong to the other player in the meeting")
			return
		}

		// Validate the target. Every Things We Share option targets the
		// PARTNER's assets (the rules are second-person — "your partner's …").
		// Each option carries a target asset (and break_peer a marginalia too).
		// slotID (not player.ID) keys the seat: a winner driving the preparer's
		// slot still targets the preparer's partner.
		partnerID := clOtherParticipantID(plan, slotID, *ld)
		// For update_peer/break_peer the valid target is the partner's MEETING
		// PEER specifically (the peer they brought to this liaison).
		meetingPeerID := clPartnerMeetingPeerID(plan, slotID, *ld)
		if status, msg := clValidateShareTarget(ctx, deps, plan, partnerID, meetingPeerID,
			body.Choice, body.TargetAssetID, body.TargetMargID, body.UpdateText); status != 0 {
			respondErr(w, status, msg)
			return
		}

		// Record the choice in liaise_choices. Effects are applied once both
		// players have submitted. update_text is only meaningful for update_peer
		// (the authored replacement marginalia text).
		var updateTextPtr *string
		if body.Choice == liaiseChoiceUpdatePeer && body.UpdateText != "" {
			updateTextPtr = &body.UpdateText
		}
		if _, err := deps.Q.CreateLiaiseChoice(ctx, dbgen.CreateLiaiseChoiceParams{
			PlanID:             plan.ID,
			PlayerID:           slotID,
			Choice:             body.Choice,
			TargetAssetID:      body.TargetAssetID,
			TargetMarginaliaID: body.TargetMargID,
			UpdateText:         updateTextPtr,
		}); err != nil {
			respondInternalErr(w, r, "could not record share-choice", err)
			return
		}

		// Mirror the submission into resolution_data so it rides the game-state
		// snapshot: the panel derives "I've submitted" from this (refresh-safe,
		// no client-local flag) and the WaitingOnBar names who still owes a
		// pick. The liaise_choices upsert is idempotent (UNIQUE plan_id,
		// player_id), so record the submitter at most once here too.
		if !slices.Contains(ld.ShareSubmitterIDs, slotID) {
			ld.ShareSubmitterIDs = append(ld.ShareSubmitterIDs, slotID)
			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondInternalErr(w, r, "could not record share submission", err)
				return
			}
		}

		// Once both have submitted, reveal and apply both choices.
		bothShared, status, msg := clRevealSharesIfBothIn(ctx, deps, plan, resData)
		if status != 0 {
			respondErr(w, status, msg)
			return
		}

		// With both picks revealed and applied, advance straight to "When will I
		// see you again?" — there is no preparer decision left in this phase, so a
		// manual "Advance" click would be pure friction (and left the second
		// submitter staring at "Waiting for your partner…"). Mirrors the
		// auto-advance out of Secrets We Keep; server-authoritative so a refresh
		// can't re-prompt the share form.
		if bothShared {
			if msg, err := clAdvanceToRedelay(ctx, deps, plan, resData, ld); err != nil {
				respondInternalErr(w, r, msg, err)
				return
			}
		}

		// The submitter no longer owes a share-choice; once both are in the phase
		// moves on to the redelay reveal (which names both who still owe a face).
		// Recompute row state so the WaitingOnBar reflects the new acting set live,
		// not just on refresh.
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"player_id": slotID,
			"choice":    body.Choice,
		})
	}
}

// clAdvanceToRedelay moves a liaison whose Things We Share picks are both in on
// to "When will I see you again?": it opens the redelay reveal (registering both
// seats), persists the new phase and tells clients to refetch. On failure it
// returns the message for the 500 alongside the error. ld points into resData.
func clAdvanceToRedelay(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData ResolutionData,
	ld *LiaiseResolutionData,
) (msg string, err error) {
	ld.Phase = LiaiseWhenWillISeeYouAgain
	if err = clEnsureRedelayReveal(ctx, deps, plan, ld); err != nil {
		return "could not create redelay reveal", err
	}
	if err = saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return "could not save liaise phase", err
	}
	broadcastEvent(deps.Manager, plan.GameID, model.EventLiaisePhaseChanged,
		model.LiaisePhaseChangedPayload{PlanID: plan.ID, Phase: string(LiaiseWhenWillISeeYouAgain)})
	return "", nil
}

// clRevealSharesIfBothIn checks whether both participants have submitted their
// Things We Share choice; if so it applies both choices' effects, persists the
// resolution_data, and broadcasts the reveal. The bothIn return reports whether
// both submissions were in (so the caller can auto-advance the phase). Returns a
// non-zero HTTP status + message on failure, or (false/true, 0, "") otherwise.
func clRevealSharesIfBothIn(
	ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, resData ResolutionData,
) (bothIn bool, status int, msg string) {
	count, err := deps.Q.CountLiaiseChoicesByPlan(ctx, plan.ID)
	if err != nil {
		return false, http.StatusInternalServerError, "could not count choices"
	}
	if count < 2 {
		return false, 0, ""
	}
	choices, err := deps.Q.ListLiaiseChoicesByPlan(ctx, plan.ID)
	if err != nil {
		return false, http.StatusInternalServerError, "could not load choices"
	}
	if err := clApplyShareChoices(ctx, deps, plan, resData, choices); err != nil {
		return false, http.StatusInternalServerError, "could not apply share choices"
	}
	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return false, http.StatusInternalServerError, "could not save liaise data"
	}
	broadcastEvent(deps.Manager, plan.GameID, model.EventLiaiseChoicesRevealed,
		model.LiaiseChoicesRevealedPayload{PlanID: plan.ID, Choices: choices})
	return true, 0, ""
}

// liaiseChoiceApplier applies one player's Things We Share choice. Each handler
// is responsible for its own DB calls and event broadcasts. Every option
// targets the PARTNER's asset (validated at submission in clValidateShareTarget).
