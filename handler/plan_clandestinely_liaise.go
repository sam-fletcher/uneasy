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
	"errors"
	"fmt"
	"net/http"

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

// PreparedDescriptor names the two peers about to meet. The liaison itself is
// "secret" in the fiction, but it's more fun for the table to know a clandestine
// meeting is brewing (dramatic irony) — so the prepared log leans into it.
func (clHandler) PreparedDescriptor(
	ctx context.Context,
	q *dbgen.Queries,
	plan dbgen.Plan,
	resData *ResolutionData,
) (string, bool) {
	l := resData.Liaise
	if l == nil || l.PreparerPeerID == nil || l.PartnerPeerID == nil {
		return "", false
	}
	return fmt.Sprintf("prepared Clandestinely Liaise — a secret meeting between %s and %s%s",
		assetDisplayName(ctx, q, *l.PreparerPeerID),
		assetDisplayName(ctx, q, *l.PartnerPeerID),
		notesSuffix(plan)), true
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
