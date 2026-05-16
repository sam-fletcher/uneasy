package handler

// handler/plan_make_introductions.go — Make Introductions plan handler (Phase 3b).
//
// Make Introductions (knowledge, delay 3): The preparer brings 1–4 new peers
// into the game. Difficulty = 2 + peer_count.
//
// Make options (per peer): "retinue" (peer joins preparer), "independent"
// (peer is unaffiliated), "gift" (peer goes to another player).
//
// TODO(make-demands keep_assets): MI peer assets are created via the generic
// POST /api/tables/{id}/assets endpoint (see handler/assets.go CreateAsset),
// which has no plan context and therefore cannot consult
// gamepkg.AssetRecipientForPlan. If a Make Demands keep_assets winner must
// also claim "retinue" peers gained here, the generic endpoint needs to
// accept an optional plan_id and route ownership through that helper — or
// MI needs to own its own peer-creation sub-route.
// Mar options: "delayed" (peer arrives in d6 rows), "center" (peer goes to
// center of table = owner_id NULL, handled via asset endpoint).
//
// Delayed arrival (Phase 3b): when the focus player chooses the mar option
// "delayed" for a specific peer, they call:
//
//	POST /api/plans/:planId/delayed-arrival  {"peer_asset_id": 123}
//
// The server rolls d6, sets target_row = current_row + d6, and creates a
// synthetic pending plan on that row with ResData.DelayedArrival = true.
// If target_row > 13, the peer asset is destroyed instead.
//
// When the public record reaches the synthetic plan's row, the focus player
// resolves it normally (resolve → complete). OnResolve returns nil (no dice
// roll); CanComplete allows completion immediately.

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanMakeIntroductions, miHandler{})
}

type miHandler struct{}

func (miHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryKnowledge, Delay: 3}
}

func (miHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (int16, string) {
	if v.PeerCount < 1 || v.PeerCount > 4 {
		return 0, "make_introductions requires peer_count between 1 and 4"
	}
	return 0, "" // fixed delay; target row computed from Metadata().Delay
}

func (miHandler) ComputeDifficulty(
	_ context.Context,
	_ *dbgen.Queries,
	_ *dbgen.Plan,
	resData *ResolutionData,
) (int16, error) {
	return gamepkg.MakeIntroductionsDifficulty(*resData), nil
}

// OnResolve creates the dice roll for normal MI plans. Synthetic delayed-arrival
// plans (ResData.DelayedArrival == true) return nil — they complete with no roll.
func (miHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	resData := loadResolutionData(plan.ResolutionData)
	if resData.DelayedArrival {
		return nil, nil // synthetic plan: no roll
	}
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return nil, err
	}
	difficulty := gamepkg.MakeIntroductionsDifficulty(resData)
	return createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
}

func (miHandler) ApplyChoice(
	_ context.Context,
	_ *PlanDeps,
	_ *dbgen.Plan,
	_ *ResolutionData,
	_ []string,
	_ string,
) error {
	return nil // make/mar effects are narrative; "delayed" handled via extra route
}

func (miHandler) CanComplete(_ *dbgen.Plan, _ *ResolutionData) error {
	return nil // no extra prerequisites for either normal or synthetic plans
}

func (miHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"delayed-arrival": delayedArrivalHandler(deps),
	}
}

// miStoreResData stores peer_count in resolution_data during plan preparation.
func miStoreResData(ctx context.Context, q *dbgen.Queries, planID int64, peerCount int16) error {
	d := ResolutionData{PeerCount: peerCount}
	return saveResolutionData(ctx, q, planID, d)
}

// ── Delayed Arrival extra route ───────────────────────────────────────────────

// delayedArrivalHandler handles POST /api/plans/:planId/delayed-arrival.
//
// Called by the focus player during MI resolution when they choose the mar
// option "delayed" for a peer. The player calls this once per delayed peer.
//
// Request body: {"peer_asset_id": 123}
//
// Effects:
//   - Rolls d6 to determine delay.
//   - If current_row + d6 > 13: destroys the peer asset (lost in transit).
//   - Otherwise: creates a synthetic pending plan on the target row with
//     ResData.DelayedArrival = true, and records its ID in the parent
//     plan's ResData.DelayedPeerPlanIDs.
//
//nolint:funlen // introductions delayed-arrival with target-asset destroy/create
func delayedArrivalHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanMakeIntroductions {
			respondErr(w, http.StatusBadRequest, "delayed-arrival is only for Make Introductions")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the focus player can schedule delayed arrivals")
			return
		}

		var body struct {
			PeerAssetID int64 `json:"peer_asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PeerAssetID == 0 {
			respondErr(w, http.StatusBadRequest, "peer_asset_id is required")
			return
		}

		ctx := r.Context()

		// Validate: must be a peer asset in this game.
		asset, err := deps.Q.GetAssetByID(ctx, body.PeerAssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "peer asset not found")
			return
		}
		if asset.GameID != plan.GameID {
			respondErr(w, http.StatusBadRequest, "asset does not belong to this game")
			return
		}
		if asset.AssetType != model.AssetPeer {
			respondErr(w, http.StatusBadRequest, "target asset must be a peer")
			return
		}

		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load game")
			return
		}

		delay := int16(rand.IntN(diceSides) + 1) // 1–6
		targetRow := game.CurrentRow + delay

		parentResData := loadResolutionData(plan.ResolutionData)

		// If target row exceeds the public record, the peer is lost.
		if targetRow > publicRecordRowCount {
			if err = deps.Q.DestroyAsset(ctx, body.PeerAssetID); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not destroy peer asset")
				return
			}
			broadcastEvent(
				deps.Manager,
				game.ID,
				model.EventAssetDestroyed,
				model.AssetIDPayload{AssetID: body.PeerAssetID},
			)
			respond(w, http.StatusOK, map[string]any{
				"peer_asset_id": body.PeerAssetID,
				"delay":         delay,
				"target_row":    targetRow,
				"outcome":       "lost",
				"note":          "peer was lost — target row exceeds row 13",
			})
			return
		}

		// Count existing plans on the target row for row_order.
		count, err := deps.Q.CountPlansOnRow(ctx, dbgen.CountPlansOnRowParams{
			GameID:    game.ID,
			RowNumber: new(targetRow),
		})
		if err != nil {
			count = 0
		}

		// Build the synthetic plan's resolution_data.
		parentPlanID := plan.ID
		syntheticResData := ResolutionData{
			DelayedArrival:     true,
			DelayedPeerAssetID: &body.PeerAssetID,
			OriginalPlanID:     &parentPlanID,
			PeerCount:          parentResData.PeerCount,
		}

		var syntheticPlan dbgen.Plan
		err = deps.InTx(ctx, func(q *dbgen.Queries) error {
			sp, cErr := q.CreatePlan(ctx, dbgen.CreatePlanParams{
				GameID:           game.ID,
				PlanType:         model.PlanMakeIntroductions,
				Category:         model.CategoryKnowledge,
				PreparerID:       plan.PreparerID,
				TargetPlayerID:   nil,
				TargetAssetID:    nil,
				RowNumber:        new(targetRow),
				RowOrder:         int16(count),
				PreparedAtRow:    game.CurrentRow,
				PreparationNotes: nil,
			})
			if cErr != nil {
				return errors.New("could not create delayed arrival plan")
			}
			syntheticPlan = sp
			if sErr := saveResolutionData(ctx, q, syntheticPlan.ID, syntheticResData); sErr != nil {
				return errors.New("could not save delayed arrival data")
			}
			parentResData.DelayedPeerPlanIDs = append(parentResData.DelayedPeerPlanIDs, syntheticPlan.ID)
			if sErr := saveResolutionData(ctx, q, plan.ID, parentResData); sErr != nil {
				return errors.New("could not update parent plan data")
			}
			return nil
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		broadcastEvent(deps.Manager, game.ID, model.EventPlanDelayedArrival, model.PlanDelayedArrivalPayload{
			PlanID:      syntheticPlan.ID,
			PeerAssetID: body.PeerAssetID,
			ArrivalRow:  targetRow,
		})

		respond(w, http.StatusCreated, map[string]any{
			"peer_asset_id":     body.PeerAssetID,
			"delay":             delay,
			"target_row":        targetRow,
			"synthetic_plan_id": syntheticPlan.ID,
		})
	}
}
