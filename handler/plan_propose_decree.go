package handler

// handler/plan_propose_decree.go — Propose Decree plan handler (Phase 3c).
//
// Propose Decree (power, delay 4): The preparer convenes a council, drafts
// a decree, and rallies the powerful to put it into law.
//
// Difficulty: preparer's rank on the power track.
//
// Pre-Roll (Council Meeting):
//   - The preparer states the drafted decree.
//   - The Monarch (rank 1) and any player ranked above the preparer may join
//     the council by leveraging ≥1 asset.
//   - The highest-power player present becomes the signatory.
//   - The signatory calls the roll when discussion is done.
//
// Make: Law goes into effect with the signatory's addendum. Server creates a
//   `laws` row and a resource asset representing the law.
// Mar: Other players present amend the law (narrative). Signatory still adds
//   addendum. Server creates a `laws` row but NO resource asset.
//
// Follow Scene: Your character interacting with a law.
//
// Extra routes:
//   POST /api/plans/:planId/join-council   Join + leverage assets; recalculate signatory.
//   POST /api/plans/:planId/call-roll      Signatory closes council; creates dice roll.
//   POST /api/plans/:planId/set-addendum   Signatory sets addendum text.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanProposeDecree, pdHandler{})
}

type pdHandler struct{}

func (pdHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryPower, Delay: 4}
}

func (pdHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (int16, string) {
	if v.Notes == "" {
		return 0, "propose_decree requires preparation_notes describing the proposed decree"
	}
	return 0, ""
}

func (pdHandler) ComputeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	rank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryPower)
	if err != nil {
		return 0, fmt.Errorf("could not determine preparer power rank: %w", err)
	}
	return gamepkg.ProposeDecreeDifficulty(rank), nil
}

// OnResolve initialises the council: sets the default signatory to the
// preparer and returns nil (the dice roll is created later by call-roll,
// once the council meeting is complete).
func (pdHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	resData := loadResolutionData(plan.ResolutionData)

	// Default signatory is the preparer. Council members may displace them.
	if resData.SignatoryID == nil {
		resData.SignatoryID = &plan.PreparerID
		// The preparer is implicitly in the council — track for completeness.
		resData.SignatoryPlayerIDs = append(resData.SignatoryPlayerIDs, plan.PreparerID)
	}

	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return nil, fmt.Errorf("could not initialise decree resolution data: %w", err)
	}

	// Return nil — the roll is created later when the signatory calls call-roll.
	return nil, nil
}

// ApplyChoice creates the law record. On make, it also creates a resource
// asset representing the enacted law; on mar it records the law without
// a corresponding asset.
func (pdHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	_ []string,
	result string,
) error {
	var preparationNotes string
	if plan.PreparationNotes != nil {
		preparationNotes = *plan.PreparationNotes
	}

	// Compute the next display_order for laws in this game.
	laws, err := deps.Q.ListLaws(ctx, plan.GameID)
	if err != nil {
		return fmt.Errorf("could not list laws: %w", err)
	}
	displayOrder := int16(len(laws) + 1)

	// Build optional addendum pointer.
	var addendumPtr *string
	if resData.Addendum != "" {
		s := resData.Addendum
		addendumPtr = &s
	}

	planID := plan.ID
	law, err := deps.Q.CreateLaw(ctx, dbgen.CreateLawParams{
		GameID:       plan.GameID,
		Text:         preparationNotes,
		Addendum:     addendumPtr,
		OriginPlanID: &planID,
		SignatoryID:  resData.SignatoryID,
		DisplayOrder: displayOrder,
	})
	if err != nil {
		return fmt.Errorf("could not create law: %w", err)
	}

	resData.LawID = &law.ID

	// On make: also create a resource asset representing the law.
	if result == makeOutcome {
		assetName := fmt.Sprintf("Law: %s", preparationNotes)
		if len(assetName) > 120 {
			assetName = assetName[:120] + "…"
		}

		// The asset is owned by the signatory (or preparer if none).
		// When no signatory, a resolved Make Demands with a keep_assets
		// winner may redirect the preparer's share.
		var ownerID int64
		if resData.SignatoryID != nil {
			ownerID = *resData.SignatoryID
		} else {
			recipient, err := gamepkg.AssetRecipientForPlan(ctx, deps.Q, plan)
			if err != nil {
				return fmt.Errorf("resolve asset recipient: %w", err)
			}
			ownerID = recipient
		}

		asset, err := deps.Q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID:    plan.GameID,
			OwnerID:   ownerID,
			CreatorID: plan.PreparerID,
			AssetType: model.AssetResource,
			Name:      assetName,
		})
		if err != nil {
			return fmt.Errorf("could not create law resource asset: %w", err)
		}

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventAssetCreated, model.AssetPayload{Asset: asset})
		}
	}

	if h, ok := deps.Manager.Get(plan.GameID); ok {
		h.BroadcastEvent(model.EventLawEnacted, model.LawEnactedPayload{
			PlanID: plan.ID,
			Law:    law,
		})
	}

	return nil
}

// CanComplete verifies that the law has been created before completing.
func (pdHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	if resData.LawID == nil {
		return errors.New("make-choice must be submitted before the plan can be completed")
	}
	return nil
}

func (pdHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"join-council": pdJoinCouncilHandler(deps),
		"call-roll":    pdCallRollHandler(deps),
		"set-addendum": pdSetAddendumHandler(deps),
	}
}

// ── Join Council ──────────────────────────────────────────────────────────────

// pdJoinCouncilHandler handles POST /api/plans/:planId/join-council.
//
// A player joins the council by leveraging ≥1 of their assets. Eligible
// players: rank 1 on the power track (Monarch), or any player ranked above
// the preparer on the power track.
//
// After joining, the signatory is recalculated: the player with the best
// (lowest) power rank among all council members becomes the new signatory.
//
// Request body: {"asset_ids": [N, ...]}
func pdJoinCouncilHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanProposeDecree {
			respondErr(w, http.StatusBadRequest, "join-council is only for Propose Decree")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		var body struct {
			AssetIDs []int64 `json:"asset_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.AssetIDs) == 0 {
			respondErr(w, http.StatusBadRequest, "asset_ids (non-empty array) is required")
			return
		}

		ctx := r.Context()

		// Determine preparer's power rank.
		preparerRank, err := playerRankInCategory(ctx, deps.Q, plan.GameID, plan.PreparerID, model.CategoryPower)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not determine preparer power rank")
			return
		}

		// Check if the joining player is the preparer themselves — they're already in.
		if player.ID == plan.PreparerID {
			respondErr(w, http.StatusConflict, "the preparer is already in the council")
			return
		}

		// Determine the joiner's power rank.
		joinerRank, err := playerRankInCategory(ctx, deps.Q, plan.GameID, player.ID, model.CategoryPower)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not determine your power rank")
			return
		}

		// Eligibility: joiner must be rank 1 (Monarch) or rank < preparer's rank.
		if joinerRank != 1 && joinerRank >= preparerRank {
			respondErr(w, http.StatusForbidden,
				"only the Monarch or players ranked above the preparer may join the council")
			return
		}

		// Verify and leverage each specified asset.
		for _, assetID := range body.AssetIDs {
			asset, err := deps.Q.GetAssetByID(ctx, assetID)
			if err != nil {
				respondErr(w, http.StatusNotFound, fmt.Sprintf("asset %d not found", assetID))
				return
			}
			if asset.GameID != plan.GameID {
				respondErr(w, http.StatusBadRequest, "asset does not belong to this game")
				return
			}
			if asset.OwnerID != player.ID {
				respondErr(w, http.StatusForbidden, "you do not own this asset")
				return
			}
			if asset.IsLeveraged {
				respondErr(w, http.StatusConflict, fmt.Sprintf("asset %d is already leveraged", assetID))
				return
			}
			if err := deps.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
				ID:          assetID,
				IsLeveraged: true,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not leverage asset")
				return
			}
			if h, ok := deps.Manager.Get(plan.GameID); ok {
				h.BroadcastEvent(model.EventAssetLeveraged, model.AssetIDPayload{
					AssetID:  assetID,
					PlayerID: player.ID,
				})
			}
		}

		resData := loadResolutionData(plan.ResolutionData)

		// Add to council if not already there.
		if !slices.Contains(resData.SignatoryPlayerIDs, player.ID) {
			resData.SignatoryPlayerIDs = append(resData.SignatoryPlayerIDs, player.ID)
		}

		// Recompute signatory: best rank (lowest) among all council members.
		bestRank := preparerRank
		bestPlayerID := plan.PreparerID
		for _, memberID := range resData.SignatoryPlayerIDs {
			memberRank, err := playerRankInCategory(ctx, deps.Q, plan.GameID, memberID, model.CategoryPower)
			if err != nil {
				continue
			}
			if memberRank < bestRank {
				bestRank = memberRank
				bestPlayerID = memberID
			}
		}
		resData.SignatoryID = &bestPlayerID

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save council data")
			return
		}

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventDecreeCouncilJoined, model.DecreeCouncilJoinedPayload{
				PlanID:      plan.ID,
				PlayerID:    player.ID,
				SignatoryID: *resData.SignatoryID,
			})
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":      plan.ID,
			"player_id":    player.ID,
			"signatory_id": *resData.SignatoryID,
			"council":      resData.SignatoryPlayerIDs,
		})
	}
}

// ── Call Roll ─────────────────────────────────────────────────────────────────

// pdCallRollHandler handles POST /api/plans/:planId/call-roll.
//
// The signatory closes the council meeting and triggers the dice roll.
// Only the current signatory may call the roll.
func pdCallRollHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanProposeDecree {
			respondErr(w, http.StatusBadRequest, "call-roll is only for Propose Decree")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		ctx := r.Context()

		resData := loadResolutionData(plan.ResolutionData)
		if resData.SignatoryID == nil || *resData.SignatoryID != player.ID {
			respondErr(w, http.StatusForbidden, "only the current signatory may call the roll")
			return
		}

		// Verify there's no existing roll for this plan.
		existingRoll, rollErr := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID)
		if rollErr == nil && existingRoll.ID != 0 {
			respondErr(w, http.StatusConflict, "a roll has already been created for this plan")
			return
		}

		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load game")
			return
		}

		difficulty, err := pdHandler{}.ComputeDifficulty(ctx, deps.Q, plan, &resData)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not compute difficulty")
			return
		}

		// The preparer is the actor; the roll uses preparer's dice.
		roll, err := createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create dice roll")
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID,
			"roll":    roll,
		})
	}
}

// ── Set Addendum ──────────────────────────────────────────────────────────────

// pdSetAddendumHandler handles POST /api/plans/:planId/set-addendum.
//
// The signatory saves their addendum text. May be called before or after
// make-choice; the text is included when ApplyChoice creates the law.
//
// Request body: {"addendum": "but/and ..."}
func pdSetAddendumHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanProposeDecree {
			respondErr(w, http.StatusBadRequest, "set-addendum is only for Propose Decree")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		ctx := r.Context()

		resData := loadResolutionData(plan.ResolutionData)
		if resData.SignatoryID == nil || *resData.SignatoryID != player.ID {
			respondErr(w, http.StatusForbidden, "only the current signatory may set the addendum")
			return
		}

		var body struct {
			Addendum string `json:"addendum"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		resData.Addendum = body.Addendum

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save addendum")
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":  plan.ID,
			"addendum": resData.Addendum,
		})
	}
}
