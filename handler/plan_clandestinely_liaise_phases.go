package handler

import (
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
			if ld.RedelayRevealID == nil && ld.PartnerID != nil {
				redelayReveal, err := deps.Q.CreateSimultaneousReveal(ctx, dbgen.CreateSimultaneousRevealParams{
					GameID:     plan.GameID,
					PlanID:     &plan.ID,
					RevealType: "liaise_redelay",
				})
				if err != nil {
					respondInternalErr(w, r, "could not create redelay reveal", err)
					return
				}
				if err := deps.Q.CreateRevealEntry(ctx, dbgen.CreateRevealEntryParams{
					RevealID: redelayReveal.ID,
					PlayerID: plan.PreparerID,
				}); err != nil {
					respondInternalErr(w, r, "could not register preparer in redelay reveal", err)
					return
				}
				if err := deps.Q.CreateRevealEntry(ctx, dbgen.CreateRevealEntryParams{
					RevealID: redelayReveal.ID,
					PlayerID: *ld.PartnerID,
				}); err != nil {
					respondInternalErr(w, r, "could not register partner in redelay reveal", err)
					return
				}
				ld.RedelayRevealID = &redelayReveal.ID
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
		secretText := fmt.Sprintf("[Clandestine meeting — %s]", preparationNotes)
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

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save keep-secret choice", err)
			return
		}

		clLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf("%s entrusted the meeting's secret to %q.",
			playerDisplayName(ctx, deps.Q, player.ID), asset.Name))

		// The other participant must refetch the plan to see this submission —
		// in particular the preparer needs it to learn both have submitted and
		// unlock the advance. Without this they'd be soft-locked on the
		// secrets-we-keep panel until a manual refresh.
		broadcastEvent(deps.Manager, plan.GameID, model.EventLiaiseKeepSecretSubmitted,
			model.LiaiseKeepSecretSubmittedPayload{PlanID: plan.ID})

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

		// Check if both players have now submitted.
		count, err := deps.Q.CountLiaiseChoicesByPlan(ctx, plan.ID)
		if err != nil {
			respondInternalErr(w, r, "could not count choices", err)
			return
		}

		if count >= 2 {
			// Both have submitted — reveal choices and apply effects.
			choices, err := deps.Q.ListLiaiseChoicesByPlan(ctx, plan.ID)
			if err != nil {
				respondInternalErr(w, r, "could not load choices", err)
				return
			}

			if err := clApplyShareChoices(ctx, deps, plan, resData, choices); err != nil {
				respondInternalErr(w, r, "could not apply share choices", err)
				return
			}

			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondInternalErr(w, r, "could not save liaise data", err)
				return
			}

			broadcastEvent(
				deps.Manager,
				plan.GameID,
				model.EventLiaiseChoicesRevealed,
				model.LiaiseChoicesRevealedPayload{
					PlanID:  plan.ID,
					Choices: choices,
				},
			)
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"player_id": player.ID,
			"choice":    body.Choice,
		})
	}
}

// liaiseChoiceApplier applies one player's Things We Share choice. Each handler
// is responsible for its own DB calls and event broadcasts. Every option
// targets the PARTNER's asset (validated at submission in clValidateShareTarget).
