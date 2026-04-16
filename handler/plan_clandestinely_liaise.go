package handler

// handler/plan_clandestinely_liaise.go — Clandestinely Liaise plan handler (Phase 3c).
//
// Clandestinely Liaise (knowledge, delay: variable) is a multi-phase plan
// with no standard dice roll. Two players meet in secret, share secrets, and
// may schedule a future meeting.
//
// Variable Delay — Simultaneous Reveal:
//   When prepared, both the preparer and their chosen partner simultaneously
//   reveal a die face (1–6). delay = ceil(average). The plan's row_number is
//   set after both submit.
//
// Resolution phases (tracked in ResData.LiaisePhase):
//   "together_at_last"    — scene-setting posts; focus player advances when ready.
//   "secrets_we_keep"     — each player picks one of their own assets to hold
//                           the meeting's secret. Submitted simultaneously via
//                           keep-secret; revealed when both have submitted.
//   "things_we_share"     — both players simultaneously choose one option each
//                           (look_at_secret, update_peer, break_peer, take_gift,
//                           leverage_partner). Submitted via share-choice.
//   "when_will_i_see_you_again" — optional re-delay reveal. Face 0 = cancel.
//
// OnResolve sets phase to "together_at_last" and returns nil (no dice roll).
//
// CanComplete: plan must have reached the "when_will_i_see_you_again" phase
//   and both redelay reveals must be complete (or both cancelled with face 0).
//
// Extra routes:
//   POST /api/plans/:planId/advance-liaise   Focus player advances to next phase.
//   POST /api/plans/:planId/keep-secret      Player commits their secret-bearing asset.
//   POST /api/plans/:planId/share-choice     Player submits Things We Share choice.
//   POST /api/plans/:planId/redelay-reveal   Player submits re-delay face (0 = cancel).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// Liaise phases (stored in ResData.LiaisePhase).
const (
	LiaiseTogetherAtLast       = "together_at_last"
	LiaiseSecretsWeKeep        = "secrets_we_keep"
	LiaiseThingsWeShare        = "things_we_share"
	LiaiseWhenWillISeeYouAgain = "when_will_i_see_you_again"
	LiaiseDone                 = "done"
)

func init() {
	RegisterPlan(model.PlanClandestinelyLiaise, clHandler{})
}

type clHandler struct{}

func (clHandler) Metadata() PlanMetadata {
	// Delay -1 = variable; the actual delay is determined by simultaneous reveal.
	return PlanMetadata{Category: model.CategoryKnowledge, Delay: -1}
}

func (clHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (int16, string) {
	if v.TargetPlayerID == nil {
		return 0, "clandestinely_liaise requires target_player_id (the partner)"
	}
	if v.Player != nil && *v.TargetPlayerID == v.Player.ID {
		return 0, "you cannot liaise with yourself"
	}
	// Row 0 is the placeholder value; the actual row is set after the delay reveal.
	// The row bounds check against row 13 happens in the reveal completion flow.
	return 0, ""
}

// ComputeDifficulty: CL has no dice roll so difficulty is not used in play.
// Returning 0 (N/A) for display purposes.
func (clHandler) ComputeDifficulty(
	_ context.Context,
	_ *dbgen.Queries,
	_ *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	return 0, nil
}

// OnResolve sets the plan phase to "together_at_last" and returns nil — CL
// does not use the standard dice roll mechanism.
func (clHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	resData := loadResolutionData(plan.ResolutionData)
	resData.LiaisePhase = LiaiseTogetherAtLast

	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return nil, fmt.Errorf("could not set liaise phase: %w", err)
	}

	if h, ok := deps.Manager.Get(plan.GameID); ok {
		h.BroadcastEvent(model.EventLiaisePhaseChanged, model.LiaisePhaseChangedPayload{
			PlanID: plan.ID,
			Phase:  LiaiseTogetherAtLast,
		})
	}

	return nil, nil
}

// ApplyChoice: no standard make/mar in CL; all choices are through extra routes.
func (clHandler) ApplyChoice(
	_ context.Context,
	_ *PlanDeps,
	_ *dbgen.Plan,
	_ *ResolutionData,
	_ []string,
	_ string,
) error {
	return nil
}

// CanComplete verifies the plan has completed all phases, including that the
// redelay reveal (When Will I See You Again) has been resolved.
func (clHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	if resData.LiaisePhase != LiaiseDone {
		return fmt.Errorf("liaise is still in phase %q — all four phases must complete first", resData.LiaisePhase)
	}
	return nil
}

func (clHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"advance-liaise": clAdvanceLiaiseHandler(deps),
		"keep-secret":    clKeepSecretHandler(deps),
		"share-choice":   clShareChoiceHandler(deps),
		"redelay-reveal": clRedelayRevealHandler(deps),
	}
}

// ── OnPrepare ─────────────────────────────────────────────────────────────────

// OnPrepare implements OnPreparer. Called immediately after the plan is created
// by PreparePlan. Sets up the simultaneous delay reveal between preparer and partner.
func (clHandler) OnPrepare(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) error {
	if plan.TargetPlayerID == nil {
		return errors.New("clandestinely_liaise requires a target player (partner)")
	}
	partnerID := *plan.TargetPlayerID

	// Create the simultaneous reveal row for the delay.
	reveal, err := deps.Q.CreateSimultaneousReveal(ctx, dbgen.CreateSimultaneousRevealParams{
		GameID:     plan.GameID,
		PlanID:     &plan.ID,
		RevealType: "liaise_delay",
	})
	if err != nil {
		return fmt.Errorf("could not create liaise delay reveal: %w", err)
	}

	// Register both participants.
	if err := deps.Q.CreateRevealEntry(ctx, dbgen.CreateRevealEntryParams{
		RevealID: reveal.ID,
		PlayerID: plan.PreparerID,
	}); err != nil {
		return fmt.Errorf("could not add preparer to reveal: %w", err)
	}
	if err := deps.Q.CreateRevealEntry(ctx, dbgen.CreateRevealEntryParams{
		RevealID: reveal.ID,
		PlayerID: partnerID,
	}); err != nil {
		return fmt.Errorf("could not add partner to reveal: %w", err)
	}

	// Store partner_id and reveal_id in resolution_data.
	resData := loadResolutionData(plan.ResolutionData)
	resData.PartnerID = &partnerID
	resData.LiaiseDelayRevealID = &reveal.ID

	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return fmt.Errorf("could not save liaise resolution data: %w", err)
	}

	return nil
}

// ── Advance Liaise ────────────────────────────────────────────────────────────

// clAdvanceLiaiseHandler handles POST /api/plans/:planId/advance-liaise.
//
// The focus player (preparer) advances the liaise to the next phase.
// Valid transitions:
//
//	together_at_last → secrets_we_keep
//	secrets_we_keep  → things_we_share   (only after both keep-secret submitted)
//	things_we_share  → when_will_i_see_you_again (only after both share-choice submitted)
func clAdvanceLiaiseHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, plan, _, ok := requirePlanFocus(w, r, deps.Q)
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

		var nextPhase string
		switch resData.LiaisePhase {
		case LiaiseTogetherAtLast:
			nextPhase = LiaiseSecretsWeKeep
		case LiaiseSecretsWeKeep:
			// Both players must have submitted keep-secret before advancing.
			if !clBothKeepSecretsSubmitted(resData, plan.PreparerID) {
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
			if resData.RedelayRevealID == nil && resData.PartnerID != nil {
				redelayReveal, err := deps.Q.CreateSimultaneousReveal(ctx, dbgen.CreateSimultaneousRevealParams{
					GameID:     plan.GameID,
					PlanID:     &plan.ID,
					RevealType: "liaise_redelay",
				})
				if err != nil {
					respondErr(w, http.StatusInternalServerError, "could not create redelay reveal")
					return
				}
				if err := deps.Q.CreateRevealEntry(ctx, dbgen.CreateRevealEntryParams{
					RevealID: redelayReveal.ID,
					PlayerID: plan.PreparerID,
				}); err != nil {
					respondErr(w, http.StatusInternalServerError, "could not register preparer in redelay reveal")
					return
				}
				if err := deps.Q.CreateRevealEntry(ctx, dbgen.CreateRevealEntryParams{
					RevealID: redelayReveal.ID,
					PlayerID: *resData.PartnerID,
				}); err != nil {
					respondErr(w, http.StatusInternalServerError, "could not register partner in redelay reveal")
					return
				}
				resData.RedelayRevealID = &redelayReveal.ID
			}
		default:
			respondErr(w, http.StatusConflict, fmt.Sprintf("cannot advance from phase %q", resData.LiaisePhase))
			return
		}

		resData.LiaisePhase = nextPhase

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save liaise phase")
			return
		}

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventLiaisePhaseChanged, model.LiaisePhaseChangedPayload{
				PlanID: plan.ID,
				Phase:  nextPhase,
			})
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID,
			"phase":   nextPhase,
		})
	}
}

// clBothKeepSecretsSubmitted checks whether both participants have submitted
// their keep-secret choice by scanning the Choices slice for entries encoded
// as "keep_secret:<playerID>:<assetID>".
func clBothKeepSecretsSubmitted(resData ResolutionData, preparerID int64) bool {
	const prefix = "keep_secret:"
	prepSubmitted := false
	partnerSubmitted := false
	for _, c := range resData.Choices {
		if !strings.HasPrefix(c, prefix) {
			continue
		}
		rest := c[len(prefix):]
		var playerID int64
		_, err := fmt.Sscanf(rest, "%d", &playerID)
		if err != nil {
			continue // TODO: code smell
		}
		if playerID == preparerID {
			prepSubmitted = true
		} else if resData.PartnerID != nil && playerID == *resData.PartnerID {
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

		if resData.LiaisePhase != LiaiseSecretsWeKeep {
			respondErr(w, http.StatusConflict, fmt.Sprintf("keep-secret requires phase %q, currently %q",
				LiaiseSecretsWeKeep, resData.LiaisePhase))
			return
		}

		// Only preparer or partner may submit.
		if !clIsParticipant(plan, player.ID, resData) {
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
			respondErr(w, http.StatusInternalServerError, "could not write secret on asset")
			return
		}

		// Record keep-secret choice.
		entry := fmt.Sprintf("keep_secret:%d:%d", player.ID, body.AssetID)
		resData.Choices = append(resData.Choices, entry)

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save keep-secret choice")
			return
		}

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

		if resData.LiaisePhase != LiaiseThingsWeShare {
			respondErr(w, http.StatusConflict, fmt.Sprintf("share-choice requires phase %q", LiaiseThingsWeShare))
			return
		}
		if !clIsParticipant(plan, player.ID, resData) {
			respondErr(w, http.StatusForbidden, "only the preparer and partner may submit share-choice")
			return
		}

		var body struct {
			Choice        string `json:"choice"`
			TargetAssetID *int64 `json:"target_asset_id"`
			DieFace       *int16 `json:"die_face"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Choice == "" {
			respondErr(w, http.StatusBadRequest, "choice is required")
			return
		}

		validChoices := []string{
			"look_at_secret", "update_peer", "break_peer", "take_gift", "leverage_partner",
		}
		validChoice := slices.Contains(validChoices, body.Choice)
		if !validChoice {
			respondErr(w, http.StatusBadRequest, fmt.Sprintf("choice must be one of: %v", validChoices))
			return
		}

		// Validate asset-required choices.
		needsAsset := body.Choice == "look_at_secret" || body.Choice == "take_gift" || body.Choice == "leverage_partner"
		if needsAsset && body.TargetAssetID == nil {
			respondErr(w, http.StatusBadRequest, fmt.Sprintf("target_asset_id is required for choice %q", body.Choice))
			return
		}
		if body.Choice == "leverage_partner" {
			if body.DieFace == nil || *body.DieFace < 1 || *body.DieFace > 6 {
				respondErr(w, http.StatusBadRequest, "die_face (1–6) is required for leverage_partner")
				return
			}
		}

		// Apply immediate mechanical effects that don't need both reveals:
		// (Most effects are deferred until both submit and choices are revealed.)
		// For now, record the choice in liaise_choices.
		var dieFace *int16
		if body.Choice == "leverage_partner" {
			dieFace = body.DieFace
		}

		if _, err := deps.Q.CreateLiaiseChoice(ctx, dbgen.CreateLiaiseChoiceParams{
			PlanID:        plan.ID,
			PlayerID:      player.ID,
			Choice:        body.Choice,
			TargetAssetID: body.TargetAssetID,
			BankedDieFace: dieFace,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not record share-choice")
			return
		}

		// Check if both players have now submitted.
		count, err := deps.Q.CountLiaiseChoicesByPlan(ctx, plan.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not count choices")
			return
		}

		if count >= 2 {
			// Both have submitted — reveal choices and apply effects.
			choices, err := deps.Q.ListLiaiseChoicesByPlan(ctx, plan.ID)
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not load choices")
				return
			}

			if err := clApplyShareChoices(ctx, deps, plan, resData, choices); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not apply share choices: "+err.Error())
				return
			}

			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not save liaise data")
				return
			}

			if h, ok := deps.Manager.Get(plan.GameID); ok {
				h.BroadcastEvent(model.EventLiaiseChoicesRevealed, model.LiaiseChoicesRevealedPayload{
					PlanID:  plan.ID,
					Choices: choices,
				})
			}
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"player_id": player.ID,
			"choice":    body.Choice,
		})
	}
}

// clApplyShareChoices applies the mechanical effects of both players' choices
// after both have submitted.
func clApplyShareChoices(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData ResolutionData,
	choices []dbgen.LiaiseChoice,
) error {
	for _, choice := range choices {
		switch choice.Choice {
		case "look_at_secret":
			if choice.TargetAssetID != nil {
				// Grant the chooser visibility on all secrets of the target asset.
				if err := deps.Q.GrantSecretVisibilityForAsset(ctx, dbgen.GrantSecretVisibilityForAssetParams{
					AssetID:  *choice.TargetAssetID,
					PlayerID: choice.PlayerID,
				}); err != nil {
					return fmt.Errorf("could not grant secret visibility: %w", err)
				}
				if h, ok := deps.Manager.Get(plan.GameID); ok {
					h.BroadcastEvent(model.EventSecretVisibilityGrant, model.SecretVisibilityGrantPayload{
						AssetID:  *choice.TargetAssetID,
						PlayerID: choice.PlayerID,
					})
				}
			}
		case "break_peer":
			// Server tears one marginalia on the player's own peer asset.
			// Finding the peer: use ListAssetsByOwner and pick first non-destroyed peer.
			assets, err := deps.Q.ListAssetsByOwner(ctx, choice.PlayerID)
			if err != nil {
				return fmt.Errorf("could not list player assets: %w", err)
			}
			for _, a := range assets {
				if a.AssetType != model.AssetPeer || a.IsDestroyed {
					continue
				}
				marginalia, err := deps.Q.ListIntactMarginalia(ctx, a.ID)
				if err != nil || len(marginalia) == 0 {
					continue
				}
				m := marginalia[0]
				if err := deps.Q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
					ID:       m.ID,
					TornByID: &choice.PlayerID,
				}); err != nil {
					return fmt.Errorf("could not tear marginalia for break_peer: %w", err)
				}
				if h, ok := deps.Manager.Get(plan.GameID); ok {
					h.BroadcastEvent(model.EventMarginaliaTorn, model.MarginaliaTornPayload{
						AssetID:  a.ID,
						Position: m.Position,
						TornByID: choice.PlayerID,
					})
				}
				break
			}
		case "leverage_partner":
			// Leverage the target asset (belonging to the partner).
			if choice.TargetAssetID != nil {
				if err := deps.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
					ID:          *choice.TargetAssetID,
					IsLeveraged: true,
				}); err != nil {
					return fmt.Errorf("could not leverage partner asset: %w", err)
				}
				if h, ok := deps.Manager.Get(plan.GameID); ok {
					h.BroadcastEvent(model.EventAssetLeveraged, model.AssetIDPayload{
						AssetID:  *choice.TargetAssetID,
						PlayerID: choice.PlayerID,
					})
				}
			}
			// Bank a die for the chooser.
			if choice.BankedDieFace != nil {
				if _, err := deps.Q.CreateBankedDie(ctx, dbgen.CreateBankedDieParams{
					GameID:   plan.GameID,
					PlayerID: choice.PlayerID,
					Face:     *choice.BankedDieFace,
					Source:   "liaise",
				}); err != nil {
					return fmt.Errorf("could not bank die: %w", err)
				}
			}
		case "take_gift":
			// Transfer the target asset to the chooser. Requires the target asset
			// to belong to the partner. Consent is social; the server transfers it.
			if choice.TargetAssetID != nil {
				if err := deps.Q.TransferAsset(ctx, dbgen.TransferAssetParams{
					ID:      *choice.TargetAssetID,
					OwnerID: choice.PlayerID,
				}); err != nil {
					return fmt.Errorf("could not transfer gift asset: %w", err)
				}
				if h, ok := deps.Manager.Get(plan.GameID); ok {
					h.BroadcastEvent(model.EventAssetTaken, model.AssetIDPayload{
						AssetID:  *choice.TargetAssetID,
						PlayerID: choice.PlayerID,
					})
				}
			}
		case "update_peer":
			// Purely narrative — player edits their peer via existing asset endpoints.
		}
	}
	_ = resData // resData may be used by future additions
	return nil
}

// ── Redelay Reveal ────────────────────────────────────────────────────────────

// clRedelayRevealHandler handles POST /api/plans/:planId/redelay-reveal.
//
// Phase 4 — When Will I See You Again?: both players submit a die face (1–6)
// or 0 to cancel the future meeting. Handled by the reveals endpoint, but
// this route is a convenience wrapper that submits to the linked redelay reveal.
//
// Request body: {"face": N}  (0 = cancel, 1–6 = schedule new meeting)
func clRedelayRevealHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanClandestinelyLiaise {
			respondErr(w, http.StatusBadRequest, "redelay-reveal is only for Clandestinely Liaise")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)

		if resData.LiaisePhase != LiaiseWhenWillISeeYouAgain {
			respondErr(
				w,
				http.StatusConflict,
				fmt.Sprintf("redelay-reveal requires phase %q", LiaiseWhenWillISeeYouAgain),
			)
			return
		}
		if !clIsParticipant(plan, player.ID, resData) {
			respondErr(w, http.StatusForbidden, "only the preparer and partner may submit redelay-reveal")
			return
		}
		if resData.RedelayRevealID == nil {
			respondErr(w, http.StatusConflict, "redelay reveal not initialised")
			return
		}

		var body struct {
			Face int16 `json:"face"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Face < 0 || body.Face > 6 {
			respondErr(w, http.StatusBadRequest, "face must be 0–6 (0 = cancel)")
			return
		}

		face := body.Face

		// Record the face in the reveal entry.
		if err := deps.Q.SetRevealEntryFace(ctx, dbgen.SetRevealEntryFaceParams{
			RevealID: *resData.RedelayRevealID,
			PlayerID: player.ID,
			Face:     &face,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not record redelay reveal")
			return
		}

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventRevealSubmitted, model.RevealSubmittedPayload{
				RevealID: *resData.RedelayRevealID,
				PlayerID: player.ID,
			})
		}

		// Check if both players have submitted.
		submitted, err := deps.Q.CountRevealEntriesSubmitted(ctx, *resData.RedelayRevealID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not check reveal status")
			return
		}
		total, err := deps.Q.CountRevealEntries(ctx, *resData.RedelayRevealID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not count reveal entries")
			return
		}

		if submitted >= total {
			// Both submitted — compute result.
			entries, err := deps.Q.ListRevealEntries(ctx, *resData.RedelayRevealID)
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not load reveal entries")
				return
			}

			// If either player cancelled (face 0), no future meeting.
			cancelled := false
			var resultDelay int16
			for _, e := range entries {
				if e.Face == nil || *e.Face == 0 {
					cancelled = true
					break
				}
			}

			if !cancelled {
				resultDelay = clCeilAverage(entries)
			}

			if err := deps.Q.SetRevealComplete(ctx, dbgen.SetRevealCompleteParams{
				ID:          *resData.RedelayRevealID,
				ResultDelay: &resultDelay,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not complete redelay reveal")
				return
			}

			// Broadcast complete event with all faces revealed.
			entryResults := make([]model.RevealEntryResult, 0, len(entries))
			for _, e := range entries {
				var f int16
				if e.Face != nil {
					f = *e.Face
				}
				entryResults = append(entryResults, model.RevealEntryResult{
					PlayerID: e.PlayerID,
					Face:     f,
				})
			}
			if h, ok := deps.Manager.Get(plan.GameID); ok {
				h.BroadcastEvent(model.EventRevealComplete, model.RevealCompletePayload{
					RevealID:    *resData.RedelayRevealID,
					Entries:     entryResults,
					ResultDelay: resultDelay,
				})
			}

			// Schedule a new meeting if not cancelled.
			if !cancelled && resultDelay > 0 {
				game, err := deps.Q.GetGameByID(ctx, plan.GameID)
				if err == nil {
					newRow := game.CurrentRow + resultDelay
					if newRow <= publicRecordRowCount && resData.PartnerID != nil {
						clScheduleNewMeeting(ctx, deps, plan, newRow, *resData.PartnerID)
					}
				}
			}

			// Mark liaise as done.
			resData.LiaisePhase = LiaiseDone
			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not complete liaise")
				return
			}
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"player_id": player.ID,
			"face":      face,
		})
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// clIsParticipant returns true if playerID is the preparer or the partner.
func clIsParticipant(plan *dbgen.Plan, playerID int64, resData ResolutionData) bool {
	if playerID == plan.PreparerID {
		return true
	}
	if resData.PartnerID != nil && *resData.PartnerID == playerID {
		return true
	}
	return false
}

// clCeilAverage returns ceil(average of all faces) from reveal entries.
func clCeilAverage(entries []dbgen.SimultaneousRevealEntry) int16 {
	if len(entries) == 0 {
		return 0
	}
	sum := 0
	count := 0
	for _, e := range entries {
		if e.Face != nil {
			sum += int(*e.Face)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return int16(math.Ceil(float64(sum) / float64(count)))
}

// clScheduleNewMeeting creates a new Clandestinely Liaise plan for the re-delay meeting.
func clScheduleNewMeeting(
	ctx context.Context,
	deps *PlanDeps,
	originalPlan *dbgen.Plan,
	targetRow int16,
	partnerID int64,
) {
	count, err := deps.Q.CountPlansOnRow(ctx, dbgen.CountPlansOnRowParams{
		GameID:    originalPlan.GameID,
		RowNumber: targetRow,
	})
	if err != nil {
		count = 0
	}

	newPlan, err := deps.Q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:           originalPlan.GameID,
		PlanType:         model.PlanClandestinelyLiaise,
		Category:         model.CategoryKnowledge,
		PreparerID:       originalPlan.PreparerID,
		TargetPlayerID:   &partnerID,
		RowNumber:        targetRow,
		RowOrder:         int16(count),
		PreparedAtRow:    targetRow, // prepared "at" the row it resolves on
		PreparationNotes: originalPlan.PreparationNotes,
	})
	if err != nil {
		return // Scheduling the new meeting is best-effort.
	}

	// Initialise resolution data for the new plan.
	newResData := ResolutionData{
		PartnerID: &partnerID,
	}
	_ = saveResolutionData(ctx, deps.Q, newPlan.ID, newResData)

	if h, ok := deps.Manager.Get(originalPlan.GameID); ok {
		h.BroadcastEvent(model.EventPlanPrepared, model.PlanPayload{Plan: newPlan})
	}
}
