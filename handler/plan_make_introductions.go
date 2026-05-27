package handler

// handler/plan_make_introductions.go — Make Introductions plan handler (Phase 3b).
//
// Make Introductions (knowledge, delay 3): The preparer brings 1–4 new peers
// into the game. Difficulty = 2 + peer_count.
//
// Make options (per peer): "retinue" (peer joins preparer), "independent"
// (peer is unaffiliated), "gift" (peer goes to another player).
//
// Pre-roll flow: the focus player names each peer one at a time via
// POST /api/plans/:planId/create-peer, which routes ownership through
// game.AssetRecipientForPlan (so a resolved Make Demands keep_assets
// winner claims them) and records each new asset ID in
// resolution_data.make_introductions.created_peer_ids. Once peer_count
// peers exist, POST /api/plans/:planId/finalize-peers creates the dice
// roll and resolution proceeds normally.
//
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
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"

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

func (miHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (*int16, string) {
	if v.PeerCount < 1 || v.PeerCount > 4 {
		return nil, "make_introductions requires peer_count between 1 and 4"
	}
	return nil, "" // fixed delay; target row computed from Metadata().Delay
}

func (miHandler) ComputeDifficulty(
	_ context.Context,
	_ *dbgen.Queries,
	_ *dbgen.Plan,
	resData *ResolutionData,
) (int16, error) {
	return gamepkg.MakeIntroductionsDifficulty(*resData), nil
}

// OnResolve defers the dice roll until the focus player has named each of
// the peer_count peers via /create-peer and called /finalize-peers. That
// matches the rule's "pre-roll: create new peer assets with names only"
// step. Synthetic delayed-arrival plans skip the roll entirely.
func (miHandler) OnResolve(_ context.Context, _ *PlanDeps, _ *dbgen.Plan) (*dbgen.DiceRoll, error) {
	return nil, nil
}

func (miHandler) CanComplete(_ *dbgen.Plan, _ *ResolutionData) error {
	return nil // no extra prerequisites for either normal or synthetic plans
}

func (miHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"create-peer":     createPeerHandler(deps),
		"finalize-peers":  finalizePeersHandler(deps),
		"delayed-arrival": delayedArrivalHandler(deps),
	}
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

// miStoreResData stores peer_count in resolution_data during plan preparation.
func miStoreResData(ctx context.Context, q *dbgen.Queries, planID int64, peerCount int16) error {
	d := ResolutionData{
		MakeIntroductions: &MakeIntroductionsResolutionData{PeerCount: peerCount},
	}
	return saveResolutionData(ctx, q, planID, d)
}

// ── Pre-roll peer creation extra routes ──────────────────────────────────────

// createPeerHandler handles POST /api/plans/:planId/create-peer.
//
// Called once per peer during the pre-roll naming step. The focus player
// (= preparer) submits a peer name and optional marginalia; the server
// creates the peer asset (routed through AssetRecipientForPlan so a
// resolved Make Demands keep_assets winner claims it) and appends the new
// asset ID to resolution_data.make_introductions.created_peer_ids.
//
// Request body: {"name": "...", "marginalia": ["text", ...]}
//

func createPeerHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanMakeIntroductions {
			respondErr(w, http.StatusBadRequest, "create-peer is only for Make Introductions")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the focus player can name peers")
			return
		}

		var body struct {
			Name       string   `json:"name"`
			Marginalia []string `json:"marginalia"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		body.Name = strings.TrimSpace(body.Name)
		if body.Name == "" {
			respondErr(w, http.StatusBadRequest, "name is required")
			return
		}
		if len(body.Marginalia) > maxMarginalia {
			respondErr(w, http.StatusBadRequest,
				fmt.Sprintf("at most %d marginalia", maxMarginalia))
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		mi := resData.EnsureMakeIntroductions()
		if int16(len(mi.CreatedPeerIDs)) >= mi.PeerCount {
			respondErr(w, http.StatusConflict, "all peers have already been named")
			return
		}

		recipient, err := gamepkg.AssetRecipientForPlan(ctx, deps.Q, plan)
		if err != nil {
			respondInternalErr(w, r, "could not resolve asset recipient", err)
			return
		}

		var asset dbgen.Asset
		var marginalia []dbgen.Marginalium
		err = deps.InTx(ctx, func(q *dbgen.Queries) error {
			a, caErr := q.CreateAsset(ctx, dbgen.CreateAssetParams{
				GameID:    plan.GameID,
				OwnerID:   recipient,
				CreatorID: player.ID,
				AssetType: model.AssetPeer,
				Name:      body.Name,
			})
			if caErr != nil {
				return errors.New("could not create peer")
			}
			asset = a
			marginalia = make([]dbgen.Marginalium, 0, len(body.Marginalia))
			for i, text := range body.Marginalia {
				text = strings.TrimSpace(text)
				if text == "" {
					continue
				}
				m, mErr := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
					AssetID:  asset.ID,
					Position: int16(i + 1),
					Text:     text,
				})
				if mErr != nil {
					return errors.New("could not create marginalia")
				}
				marginalia = append(marginalia, m)
			}
			mi.CreatedPeerIDs = append(mi.CreatedPeerIDs, asset.ID)
			return saveResolutionData(ctx, q, plan.ID, resData)
		})
		if err != nil {
			respondInternalErr(w, r, "could not create peer", err)
			return
		}

		result := assetWithMarginalia{Asset: asset, Marginalia: marginalia}
		broadcastEvent(deps.Manager, plan.GameID, model.EventAssetCreated,
			model.AssetPayload{Asset: result})
		respond(w, http.StatusCreated, map[string]any{
			"plan_id":          plan.ID,
			"asset":            result,
			"created_peer_ids": mi.CreatedPeerIDs,
		})
	}
}

// finalizePeersHandler handles POST /api/plans/:planId/finalize-peers.
//
// Called once after all peer_count peers have been named via /create-peer.
// Creates the dice roll that drives the rest of MI resolution. Idempotent
// in the sense that calling it twice 409s the second time (the plan now
// has a roll).
func finalizePeersHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanMakeIntroductions {
			respondErr(w, http.StatusBadRequest, "finalize-peers is only for Make Introductions")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the focus player can finalize peers")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		mi := resData.EnsureMakeIntroductions()
		if int16(len(mi.CreatedPeerIDs)) != mi.PeerCount {
			respondErr(w, http.StatusConflict,
				fmt.Sprintf("expected %d peers named, got %d", mi.PeerCount, len(mi.CreatedPeerIDs)))
			return
		}
		if _, err := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID); err == nil {
			respondErr(w, http.StatusConflict, "plan roll already exists")
			return
		}

		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load game", err)
			return
		}
		difficulty := gamepkg.MakeIntroductionsDifficulty(resData)
		roll, err := createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
		if err != nil {
			respondInternalErr(w, r, "could not create dice roll", err)
			return
		}
		respond(w, http.StatusCreated, map[string]any{"plan_id": plan.ID, "roll": roll})
	}
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
			respondInternalErr(w, r, "could not load game", err)
			return
		}

		delay := int16(rand.IntN(diceSides) + 1) // 1–6
		targetRow := game.CurrentRow + delay

		parentResData := loadResolutionData(plan.ResolutionData)

		// If target row exceeds the public record, the peer is lost.
		if targetRow > publicRecordRowCount {
			if err = deps.Q.DestroyAsset(ctx, body.PeerAssetID); err != nil {
				respondInternalErr(w, r, "could not destroy peer asset", err)
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
		parentPeerCount := int16(0)
		if pmi := parentResData.MakeIntroductions; pmi != nil {
			parentPeerCount = pmi.PeerCount
		}
		syntheticResData := ResolutionData{
			MakeIntroductions: &MakeIntroductionsResolutionData{
				DelayedArrival:     true,
				DelayedPeerAssetID: &body.PeerAssetID,
				OriginalPlanID:     &parentPlanID,
				PeerCount:          parentPeerCount,
			},
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
			pmi := parentResData.EnsureMakeIntroductions()
			pmi.DelayedPeerPlanIDs = append(pmi.DelayedPeerPlanIDs, syntheticPlan.ID)
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
