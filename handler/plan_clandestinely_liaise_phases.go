package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

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
// Phase 2 — Secrets We Keep: each player (preparer and partner) nominates one
// of their own assets to hold the secret of this meeting. The server writes a
// new secret on the asset's underside. Choices are revealed to both players
// once both have submitted.
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

		// Only preparer or partner may submit.
		if !clIsParticipant(plan, player.ID, *ld) {
			respondErr(w, http.StatusForbidden, "only the preparer and partner may submit keep-secret")
			return
		}

		// Reject a duplicate submission. The UI hides the form after submitting
		// (it derives iKeptSecret from the server-side kept_secrets), so a stale
		// client, a retry, or a direct API call is the only way to reach here a
		// second time — and without this guard it would write a second secret on
		// a second asset and append a duplicate KeptSecrets entry. Mirror the
		// share-choice idempotence guard above and Spread Rumors' principle that
		// a stale client can't write a duplicate.
		if slices.ContainsFunc(ld.KeptSecrets, func(ks KeptSecret) bool { return ks.PlayerID == player.ID }) {
			respondErr(w, http.StatusConflict, "you have already submitted a keep-secret")
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
		if asset.OwnerID != player.ID {
			respondErr(w, http.StatusForbidden, "you do not own this asset")
			return
		}
		if asset.GameID != plan.GameID {
			respondErr(w, http.StatusBadRequest, "asset does not belong to this game")
			return
		}

		// Write a secret on the asset's underside.
		var preparationNotes string
		if plan.PreparationNotes != nil {
			preparationNotes = *plan.PreparationNotes
		}
		secretText := fmt.Sprintf("Clandestine meeting with %s — %s",
			playerDisplayName(ctx, deps.Q, player.ID), preparationNotes)
		if _, err := deps.Q.CreateSecret(ctx, dbgen.CreateSecretParams{
			AssetID:  body.AssetID,
			AuthorID: player.ID,
			Text:     secretText,
		}); err != nil {
			respondInternalErr(w, r, "could not write secret on asset", err)
			return
		}

		// Record keep-secret choice.
		ld.KeptSecrets = append(ld.KeptSecrets, KeptSecret{
			PlayerID: player.ID,
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

		clLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf("%s entrusted the meeting's secret to %q.",
			playerDisplayName(ctx, deps.Q, player.ID), asset.Name))

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
		if !clIsParticipant(plan, player.ID, *ld) {
			respondErr(w, http.StatusForbidden, "only the preparer and partner may submit share-choice")
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
		body.UpdateText = strings.TrimSpace(body.UpdateText)

		validChoices := []string{
			liaiseChoiceLookAtSecret, liaiseChoiceUpdatePeer, liaiseChoiceBreakPeer,
			liaiseChoiceTakeGift, liaiseChoiceLeveragePartner,
		}
		if !slices.Contains(validChoices, body.Choice) {
			respondErr(w, http.StatusBadRequest, fmt.Sprintf("choice must be one of: %v", validChoices))
			return
		}

		// Validate the target. Every Things We Share option targets the
		// PARTNER's assets (the rules are second-person — "your partner's …").
		// Each option carries a target asset (and break_peer a marginalia too).
		partnerID := clOtherParticipantID(plan, player.ID, *ld)
		// For update_peer/break_peer the valid target is the partner's MEETING
		// PEER specifically (the peer they brought to this liaison).
		meetingPeerID := clPartnerMeetingPeerID(plan, player.ID, *ld)
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
			PlayerID:           player.ID,
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
		if !slices.Contains(ld.ShareSubmitterIDs, player.ID) {
			ld.ShareSubmitterIDs = append(ld.ShareSubmitterIDs, player.ID)
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
			ld.Phase = LiaiseWhenWillISeeYouAgain
			if err := clEnsureRedelayReveal(ctx, deps, plan, ld); err != nil {
				respondInternalErr(w, r, "could not create redelay reveal", err)
				return
			}
			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondInternalErr(w, r, "could not save liaise phase", err)
				return
			}
			broadcastEvent(deps.Manager, plan.GameID, model.EventLiaisePhaseChanged,
				model.LiaisePhaseChangedPayload{PlanID: plan.ID, Phase: string(LiaiseWhenWillISeeYouAgain)})
		}

		// The submitter no longer owes a share-choice; once both are in the phase
		// moves on to the redelay reveal (which names both who still owe a face).
		// Recompute row state so the WaitingOnBar reflects the new acting set live,
		// not just on refresh.
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"player_id": player.ID,
			"choice":    body.Choice,
		})
	}
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
