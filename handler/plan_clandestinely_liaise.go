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
	"net/http"
	"slices"
	"strings"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/model"
)

// Liaise phase aliases — re-export the typed enum from the game package
// so handler code can use the unqualified names.
const (
	LiaiseTogetherAtLast       = game.LiaisePhaseTogetherAtLast
	LiaiseSecretsWeKeep        = game.LiaisePhaseSecretsWeKeep
	LiaiseThingsWeShare        = game.LiaisePhaseThingsWeShare
	LiaiseWhenWillISeeYouAgain = game.LiaisePhaseWhenWillISeeYouAgain
	LiaiseDone                 = game.LiaisePhaseDone
)

// Things We Share choice values.
const (
	liaiseChoiceLookAtSecret    = "look_at_secret"
	liaiseChoiceUpdatePeer      = "update_peer"
	liaiseChoiceBreakPeer       = "break_peer"
	liaiseChoiceTakeGift        = "take_gift"
	liaiseChoiceLeveragePartner = "leverage_partner"
)

func init() {
	RegisterPlan(model.PlanClandestinelyLiaise, clHandler{})
}

type clHandler struct{}

func (clHandler) Metadata() PlanMetadata {
	// Delay -1 = variable; the actual delay is determined by simultaneous reveal.
	return PlanMetadata{Category: model.CategoryKnowledge, Delay: -1}
}

func (clHandler) ValidatePreparation(ctx context.Context, v *ValidationContext) (*int16, string) {
	if v.TargetPlayerID == nil {
		return nil, "clandestinely_liaise requires target_player_id (the partner)"
	}
	if v.Player != nil && *v.TargetPlayerID == v.Player.ID {
		return nil, "you cannot liaise with yourself"
	}
	// A liaison is a meeting between two SPECIFIC peers, one from each player's
	// retinue, chosen at prep. Both are required and each must be a peer owned
	// by the respective player. (The preparer selects both for now; the prep UI
	// recommends coordinating the partner's pick in chat first.)
	if v.Player != nil {
		if msg := clValidateMeetingPeer(ctx, v.Q, v.Player.ID, v.PreparerPeerID,
			"your own"); msg != "" {
			return nil, msg
		}
	}
	if msg := clValidateMeetingPeer(ctx, v.Q, *v.TargetPlayerID, v.PartnerPeerID,
		"your partner's"); msg != "" {
		return nil, msg
	}
	// Row 0 is the placeholder value; the actual row is set after the delay reveal.
	// The row bounds check against row 13 happens in the reveal completion flow.
	return nil, ""
}

// clValidateMeetingPeer checks that peerID names a non-destroyed peer asset
// owned by ownerID. label ("your own" / "your partner's") personalises the
// error. Returns "" when valid.
func clValidateMeetingPeer(
	ctx context.Context,
	q *dbgen.Queries,
	ownerID int64,
	peerID *int64,
	label string,
) string {
	if peerID == nil {
		return fmt.Sprintf("pick %s peer to bring to the meeting", label)
	}
	asset, err := q.GetAssetByID(ctx, *peerID)
	if err != nil {
		return fmt.Sprintf("%s meeting peer not found", label)
	}
	if asset.OwnerID != ownerID {
		return fmt.Sprintf("%s meeting peer must be a peer you each own", label)
	}
	if asset.AssetType != model.AssetPeer {
		return fmt.Sprintf("%s meeting peer must be a peer asset", label)
	}
	if asset.IsDestroyed {
		return fmt.Sprintf("%s meeting peer is destroyed", label)
	}
	return ""
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
	resData.EnsureLiaise().Phase = LiaiseTogetherAtLast

	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return nil, fmt.Errorf("could not set liaise phase: %w", err)
	}

	broadcastEvent(deps.Manager, plan.GameID, model.EventLiaisePhaseChanged, model.LiaisePhaseChangedPayload{
		PlanID: plan.ID,
		Phase:  string(LiaiseTogetherAtLast),
	})

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
	ld := resData.EnsureLiaise()
	if ld.Phase != LiaiseDone {
		return fmt.Errorf("liaise is still in phase %q — all four phases must complete first", ld.Phase)
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
	ld := resData.EnsureLiaise()
	ld.PartnerID = &partnerID
	ld.DelayRevealID = &reveal.ID

	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return fmt.Errorf("could not save liaise resolution data: %w", err)
	}

	return nil
}

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
type liaiseChoiceApplier func(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	choice dbgen.ListLiaiseChoicesByPlanRow,
) error

var liaiseChoiceAppliers = map[string]liaiseChoiceApplier{
	liaiseChoiceLookAtSecret:    applyLookAtSecret,
	liaiseChoiceBreakPeer:       applyBreakPeer,
	liaiseChoiceLeveragePartner: applyLeveragePartner,
	liaiseChoiceTakeGift:        applyTakeGift,
	liaiseChoiceUpdatePeer:      applyUpdatePeer,
}

// clApplyShareChoices applies the mechanical effects of both players' choices
// after both have submitted.
func clApplyShareChoices(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	_ ResolutionData,
	choices []dbgen.ListLiaiseChoicesByPlanRow,
) error {
	for _, choice := range choices {
		applier, ok := liaiseChoiceAppliers[choice.Choice]
		if !ok {
			continue
		}
		if err := applier(ctx, deps, plan, choice); err != nil {
			return err
		}
	}
	return nil
}

func applyLookAtSecret(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	choice dbgen.ListLiaiseChoicesByPlanRow,
) error {
	if choice.TargetAssetID == nil {
		return nil
	}
	if err := deps.Q.GrantSecretVisibilityForAsset(ctx, dbgen.GrantSecretVisibilityForAssetParams{
		AssetID:  *choice.TargetAssetID,
		PlayerID: choice.PlayerID,
	}); err != nil {
		return fmt.Errorf("could not grant secret visibility: %w", err)
	}
	broadcastEvent(deps.Manager, plan.GameID, model.EventSecretVisibilityGrant, model.SecretVisibilityGrantPayload{
		AssetID:  *choice.TargetAssetID,
		PlayerID: choice.PlayerID,
	})
	clLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf("%s looked at the secrets of %s.",
		playerDisplayName(ctx, deps.Q, choice.PlayerID), clAssetName(ctx, deps, choice.TargetAssetID)))
	return nil
}

// applyUpdatePeer rewrites one marginalia on the partner's meeting peer with
// the actor-authored replacement text (recorded on the choice). "Updating" an
// asset means editing one of its marginalia — tearing is reserved for break.
// If the peer was destroyed, or the chosen marginalia was torn, before
// resolution (e.g. broken in another plan), there is nothing to update: log a
// no-op rather than implying a change.
func applyUpdatePeer(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	choice dbgen.ListLiaiseChoicesByPlanRow,
) error {
	noop := func() {
		clLog(ctx, deps, plan, model.SeverityMinor, fmt.Sprintf(
			"%s could not update their partner's meeting peer — it no longer exists.",
			playerDisplayName(ctx, deps.Q, choice.PlayerID)))
	}

	if choice.TargetAssetID == nil || choice.TargetMarginaliaID == nil || choice.UpdateText == nil {
		noop()
		return nil
	}
	asset, err := deps.Q.GetAssetByID(ctx, *choice.TargetAssetID)
	if err != nil {
		return fmt.Errorf("update_peer: target asset not found: %w", err)
	}
	if asset.IsDestroyed {
		noop()
		return nil
	}
	m, err := deps.Q.GetMarginaliaByID(ctx, *choice.TargetMarginaliaID)
	if err != nil {
		return fmt.Errorf("update_peer: marginalia not found: %w", err)
	}
	if m.IsTorn {
		noop()
		return nil
	}

	newText := strings.TrimSpace(*choice.UpdateText)
	if newText == "" {
		noop()
		return nil
	}
	if err := deps.Q.UpdateMarginaliaText(ctx, dbgen.UpdateMarginaliaTextParams{
		ID:   m.ID,
		Text: newText,
	}); err != nil {
		return fmt.Errorf("could not update partner's peer marginalia: %w", err)
	}
	m.Text = newText
	broadcastEvent(deps.Manager, plan.GameID, model.EventMarginaliaUpdated, model.MarginaliaPayload{
		AssetID:    asset.ID,
		Marginalia: m,
	})

	clLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf("%s rewrote a marginalia on their partner's peer %q.",
		playerDisplayName(ctx, deps.Q, choice.PlayerID), asset.Name))
	return nil
}

// applyBreakPeer tears the breaker's chosen marginalia on the partner's chosen
// peer (auto-destroy if it was the last) via the canonical break helper. The
// target asset + marginalia were validated at submission time.
func applyBreakPeer(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	choice dbgen.ListLiaiseChoicesByPlanRow,
) error {
	if choice.TargetAssetID == nil || choice.TargetMarginaliaID == nil {
		return nil
	}
	asset, err := deps.Q.GetAssetByID(ctx, *choice.TargetAssetID)
	if err != nil {
		return fmt.Errorf("break_peer: target asset not found: %w", err)
	}
	if asset.IsDestroyed {
		// The meeting peer was destroyed before resolution (e.g. broken in
		// another plan). Nothing left to tear — log a no-op.
		clLog(ctx, deps, plan, model.SeverityMinor, fmt.Sprintf(
			"%s could not break their partner's meeting peer — it was already destroyed.",
			playerDisplayName(ctx, deps.Q, choice.PlayerID)))
		return nil
	}
	m, err := deps.Q.GetMarginaliaByID(ctx, *choice.TargetMarginaliaID)
	if err != nil {
		return fmt.Errorf("break_peer: marginalia not found: %w", err)
	}
	if m.IsTorn {
		return nil // Already torn (e.g. both players targeted the same one).
	}
	destroyed, err := breakMarginalia(ctx, deps.Q, deps.Manager, &asset, &m, choice.PlayerID)
	if err != nil {
		return fmt.Errorf("could not break partner's peer: %w", err)
	}
	clLog(ctx, deps, plan, model.SeverityImportant, fmt.Sprintf("%s %s their partner's peer %q.",
		playerDisplayName(ctx, deps.Q, choice.PlayerID), breakVerb(destroyed), asset.Name))
	return nil
}

func applyLeveragePartner(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	choice dbgen.ListLiaiseChoicesByPlanRow,
) error {
	if choice.TargetAssetID != nil {
		if err := deps.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
			ID:          *choice.TargetAssetID,
			IsLeveraged: true,
		}); err != nil {
			return fmt.Errorf("could not leverage partner asset: %w", err)
		}
		broadcastEvent(deps.Manager, plan.GameID, model.EventAssetLeveraged, model.AssetIDPayload{
			AssetID:  *choice.TargetAssetID,
			PlayerID: choice.PlayerID,
		})
	}
	// Bank a die for a future roll. The die rolls a random face at resolution
	// time like any other die — banked dice do not carry a pre-determined face.
	if _, err := deps.Q.CreateBankedDie(ctx, dbgen.CreateBankedDieParams{
		GameID:   plan.GameID,
		PlayerID: choice.PlayerID,
		Source:   "liaise",
	}); err != nil {
		return fmt.Errorf("could not bank die: %w", err)
	}
	clLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf("%s leveraged %s and banked a die for a future roll.",
		playerDisplayName(ctx, deps.Q, choice.PlayerID), clAssetName(ctx, deps, choice.TargetAssetID)))
	return nil
}

// applyTakeGift transfers the partner-owned target asset to the chooser.
// Consent is social; the server transfers ownership.
func applyTakeGift(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	choice dbgen.ListLiaiseChoicesByPlanRow,
) error {
	if choice.TargetAssetID == nil {
		return nil
	}
	asset, err := deps.Q.GetAssetByID(ctx, *choice.TargetAssetID)
	if err != nil {
		return fmt.Errorf("take_gift: target asset not found: %w", err)
	}
	oldOwner := asset.OwnerID
	if err := deps.Q.TransferAsset(ctx, dbgen.TransferAssetParams{
		ID:      *choice.TargetAssetID,
		OwnerID: choice.PlayerID,
	}); err != nil {
		return fmt.Errorf("could not transfer gift asset: %w", err)
	}
	updated, _ := deps.Q.GetAssetByID(ctx, *choice.TargetAssetID)
	broadcastEvent(deps.Manager, plan.GameID, model.EventAssetTaken, model.AssetTakenPayload{
		Asset:      updated,
		OldOwnerID: oldOwner,
		NewOwnerID: choice.PlayerID,
	})
	clLog(ctx, deps, plan, model.SeverityImportant, fmt.Sprintf("%s took %q from their partner as a gift.",
		playerDisplayName(ctx, deps.Q, choice.PlayerID), asset.Name))
	return nil
}

// clAssetName resolves an asset id to its name for log bodies; "an asset" on
// any failure so the log line still reads cleanly.
func clAssetName(ctx context.Context, deps *PlanDeps, assetID *int64) string {
	if assetID == nil {
		return "an asset"
	}
	a, err := deps.Q.GetAssetByID(ctx, *assetID)
	if err != nil {
		return "an asset"
	}
	return fmt.Sprintf("%q", a.Name)
}

// clLog emits a Clandestinely Liaise action-log entry anchored to the plan row.
func clLog(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, severity int32, body string) {
	planID := plan.ID
	EmitSystemPost(ctx, deps.Q, deps.Manager, plan.GameID, "plan.clandestinely_liaise",
		severity, body, plan.RowNumber, &planID, nil,
		map[string]any{"plan_id": plan.ID})
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
		ld := resData.EnsureLiaise()

		if ld.Phase != LiaiseWhenWillISeeYouAgain {
			respondErr(
				w,
				http.StatusConflict,
				fmt.Sprintf("redelay-reveal requires phase %q", LiaiseWhenWillISeeYouAgain),
			)
			return
		}
		if !clIsParticipant(plan, player.ID, *ld) {
			respondErr(w, http.StatusForbidden, "only the preparer and partner may submit redelay-reveal")
			return
		}
		if ld.RedelayRevealID == nil {
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
			RevealID: *ld.RedelayRevealID,
			PlayerID: player.ID,
			Face:     &face,
		}); err != nil {
			respondInternalErr(w, r, "could not record redelay reveal", err)
			return
		}

		broadcastEvent(deps.Manager, plan.GameID, model.EventRevealSubmitted, model.RevealSubmittedPayload{
			RevealID: *ld.RedelayRevealID,
			PlayerID: player.ID,
		})

		// Check if both players have submitted.
		submitted, err := deps.Q.CountRevealEntriesSubmitted(ctx, *ld.RedelayRevealID)
		if err != nil {
			respondInternalErr(w, r, "could not check reveal status", err)
			return
		}
		total, err := deps.Q.CountRevealEntries(ctx, *ld.RedelayRevealID)
		if err != nil {
			respondInternalErr(w, r, "could not count reveal entries", err)
			return
		}

		if submitted >= total {
			if !clFinalizeRedelayReveal(r, ctx, w, deps, plan, &resData) {
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

// clFinalizeRedelayReveal runs the "both submitted" branch of redelay-reveal:
// computes the result delay (or detects a cancellation), marks the reveal
// complete, broadcasts the result, schedules a follow-up meeting if needed,
// and marks the liaise done. Writes HTTP errors and returns false on failure.
func clFinalizeRedelayReveal(
	r *http.Request,
	ctx context.Context,
	w http.ResponseWriter,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
) bool {
	ld := resData.EnsureLiaise()
	entries, err := deps.Q.ListRevealEntries(ctx, *ld.RedelayRevealID)
	if err != nil {
		respondInternalErr(w, r, "could not load reveal entries", err)
		return false
	}

	cancelled := false
	var resultDelay int16
	for _, e := range entries {
		if e.Face == nil || *e.Face == 0 {
			cancelled = true
			break
		}
	}
	if !cancelled {
		resultDelay = revealCeilAverage(entries)
	}

	err = deps.Q.SetRevealComplete(ctx, dbgen.SetRevealCompleteParams{
		ID:          *ld.RedelayRevealID,
		ResultDelay: &resultDelay,
	})
	if err != nil {
		respondInternalErr(w, r, "could not complete redelay reveal", err)
		return false
	}

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
	broadcastEvent(deps.Manager, plan.GameID, model.EventRevealComplete, model.RevealCompletePayload{
		RevealID:    *ld.RedelayRevealID,
		Entries:     entryResults,
		ResultDelay: resultDelay,
	})

	scheduled := false
	if !cancelled && resultDelay > 0 && ld.PartnerID != nil {
		if game, gerr := deps.Q.GetGameByID(ctx, plan.GameID); gerr == nil {
			newRow := game.CurrentRow + resultDelay
			if newRow <= publicRecordRowCount {
				clScheduleNewMeeting(ctx, deps, plan, newRow, *ld.PartnerID)
				scheduled = true
			}
		}
	}
	if scheduled {
		clLog(ctx, deps, plan, model.SeverityDefault, "The pair arranged to meet again.")
	} else {
		clLog(ctx, deps, plan, model.SeverityDefault, "The pair parted ways with no future meeting planned.")
	}

	ld.Phase = LiaiseDone
	if err = saveResolutionData(ctx, deps.Q, plan.ID, *resData); err != nil {
		respondInternalErr(w, r, "could not complete liaise", err)
		return false
	}
	return true
}

// clIsParticipant returns true if playerID is the preparer or the partner.
func clIsParticipant(plan *dbgen.Plan, playerID int64, ld LiaiseResolutionData) bool {
	if playerID == plan.PreparerID {
		return true
	}
	if ld.PartnerID != nil && *ld.PartnerID == playerID {
		return true
	}
	return false
}

// clOtherParticipantID returns the participant who is NOT playerID (the
// partner from playerID's perspective). Returns 0 if it can't be determined.
func clOtherParticipantID(plan *dbgen.Plan, playerID int64, ld LiaiseResolutionData) int64 {
	if playerID == plan.PreparerID {
		if ld.PartnerID != nil {
			return *ld.PartnerID
		}
		return 0
	}
	return plan.PreparerID
}

// clPartnerMeetingPeerID returns the meeting peer of playerID's PARTNER — the
// peer the other player brought to the liaison. From the preparer's seat that's
// PartnerPeerID; from the partner's seat it's PreparerPeerID. Returns nil if
// the relevant peer was never set.
func clPartnerMeetingPeerID(plan *dbgen.Plan, playerID int64, ld LiaiseResolutionData) *int64 {
	if playerID == plan.PreparerID {
		return ld.PartnerPeerID
	}
	return ld.PreparerPeerID
}

// clValidateShareTarget enforces that a Things We Share choice targets the
// partner's assets per the rules (all five options are second-person — "your
// partner's …"). Returns (0, "") when valid, or an HTTP status + message.
//
//   - look_at_secret / leverage_partner: any partner-owned, non-destroyed asset.
//   - take_gift:                          a partner-owned, non-destroyed NON-peer.
//   - update_peer / break_peer:           the partner's MEETING PEER specifically
//     (meetingPeerID — the peer they brought to this liaison), non-destroyed.
//     Both require a marginalia on that peer: break_peer tears it; update_peer
//     rewrites it (and so also requires non-empty updateText).
func clValidateShareTarget(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	partnerID int64,
	meetingPeerID *int64,
	choice string,
	targetAssetID, targetMargID *int64,
	updateText string,
) (status int, msg string) {
	if partnerID == 0 {
		return http.StatusConflict, "liaison partner is not set"
	}
	if targetAssetID == nil {
		return http.StatusBadRequest, fmt.Sprintf("target_asset_id is required for choice %q", choice)
	}
	asset, err := deps.Q.GetAssetByID(ctx, *targetAssetID)
	if err != nil {
		return http.StatusNotFound, "target asset not found"
	}
	if asset.GameID != plan.GameID {
		return http.StatusBadRequest, "target asset does not belong to this game"
	}
	if asset.OwnerID != partnerID {
		return http.StatusForbidden, "Things We Share options target your partner's assets"
	}
	if asset.IsDestroyed {
		return http.StatusBadRequest, "target asset is destroyed"
	}

	switch choice {
	case liaiseChoiceTakeGift:
		if asset.AssetType == model.AssetPeer {
			return http.StatusBadRequest, "a gift must be a non-peer asset"
		}
	case liaiseChoiceUpdatePeer, liaiseChoiceBreakPeer:
		if asset.AssetType != model.AssetPeer {
			return http.StatusBadRequest, fmt.Sprintf("%q targets a peer asset", choice)
		}
		// These options must target the partner's MEETING PEER — the specific
		// peer they brought to the liaison — not an arbitrary partner peer.
		if meetingPeerID == nil {
			return http.StatusConflict, fmt.Sprintf(
				"%q is unavailable — the meeting peer no longer exists", choice)
		}
		if *targetAssetID != *meetingPeerID {
			return http.StatusBadRequest, fmt.Sprintf(
				"%q must target your partner's meeting peer", choice)
		}
		// Both update_peer and break_peer act on a specific marginalia of the
		// meeting peer. break_peer tears it; update_peer rewrites it.
		if targetMargID == nil {
			verb := "the marginalia to tear"
			if choice == liaiseChoiceUpdatePeer {
				verb = "the marginalia to rewrite"
			}
			return http.StatusBadRequest, fmt.Sprintf("%s requires target_marginalia_id (%s)", choice, verb)
		}
		m, err := deps.Q.GetMarginaliaByID(ctx, *targetMargID)
		if err != nil {
			return http.StatusNotFound, "marginalia not found"
		}
		if m.AssetID != *targetAssetID {
			return http.StatusBadRequest, "marginalia does not belong to the target peer"
		}
		if m.IsTorn {
			return http.StatusConflict, "marginalia is already torn"
		}
		if choice == liaiseChoiceUpdatePeer && updateText == "" {
			return http.StatusBadRequest, "update_peer requires update_text (the rewritten marginalia)"
		}
	}
	return 0, ""
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
		RowNumber: new(targetRow),
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
		RowNumber:        new(targetRow),
		RowOrder:         int16(count),
		PreparedAtRow:    targetRow, // prepared "at" the row it resolves on
		PreparationNotes: originalPlan.PreparationNotes,
	})
	if err != nil {
		return // Scheduling the new meeting is best-effort.
	}

	// Initialise resolution data for the new plan. Carry forward the original
	// meeting peers — the same two peers reconvene. If either has been destroyed
	// by the time the follow-up resolves, Things We Share handles it gracefully.
	origLD := game.LoadLiaiseData(originalPlan.ResolutionData)
	newResData := ResolutionData{
		Liaise: &LiaiseResolutionData{
			PartnerID:      &partnerID,
			PreparerPeerID: origLD.PreparerPeerID,
			PartnerPeerID:  origLD.PartnerPeerID,
		},
	}
	_ = saveResolutionData(ctx, deps.Q, newPlan.ID, newResData)

	broadcastEvent(deps.Manager, originalPlan.GameID, model.EventPlanPrepared, model.PlanPayload{Plan: newPlan})
	EmitPlanPrepared(ctx, deps.Q, deps.Manager, newPlan)
}
