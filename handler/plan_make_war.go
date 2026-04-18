package handler

// handler/plan_make_war.go — Make War plan handler (Phase 3d).
//
// Make War (power, delay: variable) declares war against one or more other
// players. All war participants simultaneously reveal a die; the plan's
// delay equals ceil(average) of the revealed faces.
//
// The plan's resolution is purely narrative (no dice roll, no make/mar).
// The focus player posts one scene when the plan reaches its row, then
// completes. The war itself persists across rows in the `wars` table until
// peace is agreed or one side fully surrenders.
//
// Cost of battle — the recurring mechanic — is implemented in two places:
//   - turn.go's advanceRowInner gates row advance on unpaid costs.
//   - /pay-battle-cost, /propose-peace, /vote-peace, /accept-peace are the
//     endpoints players use to resolve those costs and negotiate an ending.
//
// Extra routes:
//
//	POST /api/plans/:planId/join-war           — non-preparer joins a side.
//	POST /api/plans/:planId/post-war-scene     — focus player marks the
//	                                              one-time declaration scene.
//	POST /api/plans/:planId/pay-battle-cost    — pay one opponent's cost.
//	POST /api/plans/:planId/propose-peace      — open a peace proposal.
//	POST /api/plans/:planId/vote-peace         — accept/reject open proposal.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanMakeWar, mwHandler{})
}

type mwHandler struct{}

func (mwHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryPower, Delay: -1}
}

func (mwHandler) ValidatePreparation(ctx context.Context, v *ValidationContext) (int16, string) {
	if len(v.EnemyPlayerIDs) == 0 {
		return 0, "make_war requires at least one entry in enemy_player_ids"
	}
	seen := map[int64]bool{v.Player.ID: true}
	for _, id := range v.EnemyPlayerIDs {
		if id == v.Player.ID {
			return 0, "cannot declare war on yourself"
		}
		if seen[id] {
			return 0, "enemy_player_ids contains duplicates"
		}
		seen[id] = true
		p, err := v.Q.GetPlayerByID(ctx, id)
		if err != nil || p.GameID != v.Game.ID {
			return 0, fmt.Sprintf("enemy player %d is not in this game", id)
		}
	}
	// Row placeholder; the actual row_number is set after the delay reveal.
	return 0, ""
}

// OnPrepare creates the war row, enrols every participant on the correct
// side, and opens a simultaneous reveal for the delay. Enemy list is read
// from resolution_data (persisted by PreparePlan before this hook fires).
func (mwHandler) OnPrepare(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) error {
	resData := loadResolutionData(plan.ResolutionData)
	if len(resData.WarEnemyPlayerIDs) == 0 {
		return errors.New("make_war: missing enemy list in resolution_data")
	}

	war, err := deps.Q.CreateWar(ctx, dbgen.CreateWarParams{
		GameID:       plan.GameID,
		OriginPlanID: plan.ID,
		StartedAtRow: plan.PreparedAtRow,
	})
	if err != nil {
		return fmt.Errorf("create war: %w", err)
	}

	// Side 1: preparer.
	err = deps.Q.AddWarParticipant(ctx, dbgen.AddWarParticipantParams{
		WarID:       war.ID,
		PlayerID:    plan.PreparerID,
		Side:        gamepkg.WarSideDeclarer,
		JoinedAtRow: plan.PreparedAtRow,
	})
	if err != nil {
		return fmt.Errorf("enrol preparer: %w", err)
	}
	// Side 2: declared enemies.
	for _, enemyID := range resData.WarEnemyPlayerIDs {
		if err := deps.Q.AddWarParticipant(ctx, dbgen.AddWarParticipantParams{
			WarID:       war.ID,
			PlayerID:    enemyID,
			Side:        gamepkg.WarSideEnemy,
			JoinedAtRow: plan.PreparedAtRow,
		}); err != nil {
			return fmt.Errorf("enrol enemy %d: %w", enemyID, err)
		}
	}

	// Open the simultaneous reveal (all participants submit a die face).
	reveal, errReveal := deps.Q.CreateSimultaneousReveal(ctx, dbgen.CreateSimultaneousRevealParams{
		GameID:     plan.GameID,
		PlanID:     &plan.ID,
		RevealType: "make_war_delay",
	})
	if errReveal != nil {
		return fmt.Errorf("create delay reveal: %w", errReveal)
	}
	all := append([]int64{plan.PreparerID}, resData.WarEnemyPlayerIDs...)
	for _, pid := range all {
		if err := deps.Q.CreateRevealEntry(ctx, dbgen.CreateRevealEntryParams{
			RevealID: reveal.ID,
			PlayerID: pid,
		}); err != nil {
			return fmt.Errorf("add reveal entry for %d: %w", pid, err)
		}
	}

	resData.WarID = &war.ID
	resData.DelayRevealID = &reveal.ID
	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return fmt.Errorf("save war resolution data: %w", err)
	}
	return nil
}

func (mwHandler) ComputeDifficulty(
	_ context.Context,
	_ *dbgen.Queries,
	_ *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	// Make War has no dice roll; difficulty is N/A.
	return 0, nil
}

// OnResolve marks the plan as pre-resolved narratively. No roll is created;
// the focus player posts a declaration scene via /post-war-scene, then
// calls /complete. Setting the plan's result up-front lets CompletePlan
// finalise without a roll outcome.
func (mwHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	res := makeOutcome
	if err := deps.Q.SetPlanResultPreserveStatus(ctx, dbgen.SetPlanResultPreserveStatusParams{
		ID:     plan.ID,
		Result: &res,
	}); err != nil {
		return nil, fmt.Errorf("pre-record war result: %w", err)
	}
	return nil, nil
}

// ApplyChoice is unused — Make War has no make/mar options.
func (mwHandler) ApplyChoice(
	_ context.Context,
	_ *PlanDeps,
	_ *dbgen.Plan,
	_ *ResolutionData,
	_ []string,
	_ string,
) error {
	return nil
}

// CanComplete requires the focus player to have posted the war declaration
// scene so the record isn't finalised before the narrative moment.
func (mwHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	if !resData.WarScenePosted {
		return errors.New("post the war's declaration scene before completing")
	}
	return nil
}

func (mwHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"join-war":        mwJoinHandler(deps),
		"post-war-scene":  mwPostSceneHandler(deps),
		"pay-battle-cost": mwPayBattleCostHandler(deps),
		"propose-peace":   mwProposePeaceHandler(deps),
		"vote-peace":      mwVotePeaceHandler(deps),
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func mwCheckPlan(w http.ResponseWriter, plan *dbgen.Plan) bool {
	if plan.PlanType != model.PlanMakeWar {
		respondErr(w, http.StatusBadRequest, "route is only for Make War plans")
		return false
	}
	return true
}

// mwLoadWar resolves the war for a Make War plan, or writes a 500.
func mwLoadWar(
	ctx context.Context,
	w http.ResponseWriter,
	q *dbgen.Queries,
	plan *dbgen.Plan,
) (dbgen.War, bool) {
	war, err := q.GetWarByOriginPlan(ctx, plan.ID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "could not load war")
		return dbgen.War{}, false
	}
	return war, true
}

// ── join-war ─────────────────────────────────────────────────────────────────
//
// A non-enrolled player joins a side. Allowed free-of-charge while the
// delay reveal is still open (declaration phase). Once the reveal has
// completed (war is active) joiners are still admitted here but must
// complete the full cost-of-battle sequence (one payment per opposing
// opponent) before they are counted as an active participant for peace
// voting — that gating lives in the peace-voting endpoint.
func mwJoinHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if !mwCheckPlan(w, plan) {
			return
		}
		var body struct {
			Side int16 `json:"side"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Side != gamepkg.WarSideDeclarer && body.Side != gamepkg.WarSideEnemy {
			respondErr(w, http.StatusBadRequest, "side must be 1 or 2")
			return
		}

		ctx := r.Context()
		war, ok := mwLoadWar(ctx, w, deps.Q, plan)
		if !ok {
			return
		}
		if _, err := deps.Q.GetWarParticipant(ctx, dbgen.GetWarParticipantParams{
			WarID: war.ID, PlayerID: player.ID,
		}); err == nil {
			respondErr(w, http.StatusConflict, "you are already a participant in this war")
			return
		}

		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load game")
			return
		}

		if err := deps.Q.AddWarParticipant(ctx, dbgen.AddWarParticipantParams{
			WarID:       war.ID,
			PlayerID:    player.ID,
			Side:        body.Side,
			JoinedAtRow: game.CurrentRow,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not join war")
			return
		}

		// If the delay reveal is still open, add them so they reveal too.
		resData := loadResolutionData(plan.ResolutionData)
		if resData.DelayRevealID != nil {
			reveal, err := deps.Q.GetSimultaneousReveal(ctx, *resData.DelayRevealID)
			if err == nil && !reveal.IsComplete {
				_ = deps.Q.CreateRevealEntry(ctx, dbgen.CreateRevealEntryParams{
					RevealID: reveal.ID,
					PlayerID: player.ID,
				})
			}
		}

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventWarPlayerJoined, model.WarPlayerJoinedPayload{
				WarID: war.ID, PlayerID: player.ID, Side: body.Side,
			})
		}
		respond(w, http.StatusOK, map[string]any{
			"war_id": war.ID, "player_id": player.ID, "side": body.Side,
		})
	}
}

// ── post-war-scene ───────────────────────────────────────────────────────────
//
// Focus player marks the one-time narrative beat complete. Gates /complete.
func mwPostSceneHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, plan, _, ok := requirePlanFocus(w, r, deps.Q)
		if !ok {
			return
		}
		if !mwCheckPlan(w, plan) {
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		resData.WarScenePosted = true
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save scene state")
			return
		}
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "scene_posted": true})
	}
}

// ── pay-battle-cost / propose-peace / vote-peace ─────────────────────────────
//
// Cost of battle is paid in reverse power order, one (payer, opponent) pair
// at a time. The caller's turn is determined by asking MissingBattleCosts
// for the first outstanding pair — if its PayerID matches the caller, they
// are up next. Surrender and asset-taking after surrender are not yet wired;
// see the TODO at the top of mwPayBattleCostHandler.

// applyMakeWarDelayResult is invoked by reveals.go when the make_war_delay
// simultaneous reveal completes. It sets the plan's row_number to
// current_row + resultDelay (cancelling the plan — and the nascent war — if
// the target exceeds row 13) and broadcasts war.declared.
func applyMakeWarDelayResult(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	planID int64,
	resultDelay int16,
) {
	plan, err := q.GetPlanByID(ctx, planID)
	if err != nil {
		return
	}
	game, err := q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return
	}

	targetRow := game.CurrentRow + resultDelay

	resData := loadResolutionData(plan.ResolutionData)

	if targetRow > publicRecordRowCount {
		_ = q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
			ID:     planID,
			Status: model.PlanCancelled,
		})
		if resData.WarID != nil {
			_ = q.EndWar(ctx, dbgen.EndWarParams{
				ID:         *resData.WarID,
				EndReason:  stringPtr(gamepkg.WarEndPeace),
				EndedAtRow: &game.CurrentRow,
			})
		}
		if h, ok := manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventPlanResolved, model.PlanResolvedPayload{
				PlanID: planID,
				Result: "cancelled",
			})
		}
		return
	}

	_ = q.SetPlanRowNumber(ctx, dbgen.SetPlanRowNumberParams{
		ID:        planID,
		RowNumber: targetRow,
	})

	if resData.WarID == nil {
		return
	}

	parts, err := q.ListWarParticipants(ctx, *resData.WarID)
	if err == nil {
		infos := make([]model.WarParticipantInfo, 0, len(parts))
		for _, p := range parts {
			infos = append(infos, model.WarParticipantInfo{PlayerID: p.PlayerID, Side: p.Side})
		}
		if h, ok := manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventWarDeclared, model.WarDeclaredPayload{
				PlanID:       planID,
				WarID:        *resData.WarID,
				Participants: infos,
				TargetRow:    targetRow,
			})
		}
	}
}

//go:fix inline
func stringPtr(s string) *string { return new(s) }

// mwPayBattleCostHandler handles POST /api/plans/:planId/pay-battle-cost.
//
// Body:
//
//	{
//	  "opponent_id":    int64,
//	  "choice":         "break_asset" | "leverage_two",
//	  "marginalia_id":  int64,   // break_asset only — tear one marginalia
//	  "asset_id_1":     int64,   // leverage_two only
//	  "asset_id_2":     int64    // leverage_two only
//	}
//
// TODO: surrender (do one of the above then surrender unconditionally; each
// opponent takes one asset). Asset-taking after surrender is not implemented.
func mwPayBattleCostHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if !mwCheckPlan(w, plan) {
			return
		}
		var body struct {
			OpponentID   int64  `json:"opponent_id"`
			Choice       string `json:"choice"`
			MarginaliaID int64  `json:"marginalia_id"`
			AssetID1     int64  `json:"asset_id_1"`
			AssetID2     int64  `json:"asset_id_2"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !gamepkg.IsValidBattleCostChoice(body.Choice) {
			respondErr(w, http.StatusBadRequest, "choice must be break_asset or leverage_two")
			return
		}

		ctx := r.Context()
		war, ok := mwLoadWar(ctx, w, deps.Q, plan)
		if !ok {
			return
		}
		if war.Status != "active" {
			respondErr(w, http.StatusConflict, "war is no longer active")
			return
		}
		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load game")
			return
		}

		snap, err := mwSnapshotWar(ctx, deps.Q, war)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load war participants")
			return
		}
		ranks, err := mwPowerRanks(ctx, deps.Q, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load rankings")
			return
		}
		missing, err := mwOutstandingCostsForWar(ctx, deps.Q, snap, ranks, game.CurrentRow)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not compute outstanding costs")
			return
		}
		if len(missing) == 0 {
			respondErr(w, http.StatusConflict, "no battle costs are outstanding this row")
			return
		}
		// Strict reverse-power order: the first outstanding payer must be the caller.
		if missing[0].PayerID != player.ID {
			respondErr(w, http.StatusConflict,
				"another participant must pay their battle cost first (reverse power order)")
			return
		}
		// The requested opponent must be among the caller's outstanding opponents.
		owesOpponent := false
		for _, k := range missing {
			if k.PayerID == player.ID && k.OpponentID == body.OpponentID {
				owesOpponent = true
				break
			}
		}
		if !owesOpponent {
			respondErr(w, http.StatusConflict, "you do not owe a cost to that opponent this row")
			return
		}

		// Apply the chosen cost.
		var assetID1, assetID2 *int64
		switch body.Choice {
		case gamepkg.WarCostBreakAsset:
			m, err := deps.Q.GetMarginaliaByID(ctx, body.MarginaliaID)
			if err != nil {
				respondErr(w, http.StatusNotFound, "marginalia not found")
				return
			}
			asset, err := deps.Q.GetAssetByID(ctx, m.AssetID)
			if err != nil || asset.OwnerID != player.ID {
				respondErr(w, http.StatusForbidden, "marginalia must belong to an asset you own")
				return
			}
			if asset.IsDestroyed {
				respondErr(w, http.StatusConflict, "asset is already destroyed")
				return
			}
			if m.IsTorn {
				respondErr(w, http.StatusConflict, "marginalia is already torn")
				return
			}
			if err := deps.Q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
				ID:       m.ID,
				TornByID: &player.ID,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not tear marginalia")
				return
			}
			if h, ok := deps.Manager.Get(plan.GameID); ok {
				h.BroadcastEvent(model.EventMarginaliaTorn, model.MarginaliaTornPayload{
					AssetID: asset.ID, Position: m.Position, TornByID: player.ID,
				})
			}
			// If every marginalia is now torn, destroy the asset.
			intact, _ := deps.Q.CountIntactMarginalia(ctx, asset.ID)
			if intact == 0 {
				total, _ := deps.Q.CountMarginalia(ctx, asset.ID)
				if total > 0 {
					_ = deps.Q.DestroyAsset(ctx, asset.ID)
					if h, ok := deps.Manager.Get(plan.GameID); ok {
						h.BroadcastEvent(model.EventAssetDestroyed, model.AssetIDPayload{AssetID: asset.ID})
					}
				}
			}
			assetID1 = &asset.ID

		case gamepkg.WarCostLeverageTwo:
			if body.AssetID1 == 0 || body.AssetID2 == 0 || body.AssetID1 == body.AssetID2 {
				respondErr(w, http.StatusBadRequest, "must specify two distinct assets to leverage")
				return
			}
			for _, id := range []int64{body.AssetID1, body.AssetID2} {
				a, err := deps.Q.GetAssetByID(ctx, id)
				if err != nil {
					respondErr(w, http.StatusNotFound, "asset not found")
					return
				}
				if a.OwnerID != player.ID {
					respondErr(w, http.StatusForbidden, "you can only leverage your own assets")
					return
				}
				if a.IsDestroyed {
					respondErr(w, http.StatusConflict, "asset is destroyed")
					return
				}
				if a.IsLeveraged {
					respondErr(w, http.StatusConflict, "asset is already leveraged")
					return
				}
			}
			for _, id := range []int64{body.AssetID1, body.AssetID2} {
				if err := deps.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
					ID: id, IsLeveraged: true,
				}); err != nil {
					respondErr(w, http.StatusInternalServerError, "could not leverage asset")
					return
				}
			}
			assetID1 = &body.AssetID1
			assetID2 = &body.AssetID2
		}

		// Record the payment.
		if _, err := deps.Q.CreateBattleCost(ctx, dbgen.CreateBattleCostParams{
			WarID:       war.ID,
			RowNumber:   game.CurrentRow,
			PayerID:     player.ID,
			OpponentID:  body.OpponentID,
			Choice:      body.Choice,
			AssetID1:    assetID1,
			AssetID2:    assetID2,
			Surrendered: false,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not record battle cost")
			return
		}

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventWarBattleCostPaid, model.WarBattleCostPaidPayload{
				WarID: war.ID, RowNumber: game.CurrentRow,
				PayerID: player.ID, OpponentID: body.OpponentID,
				Choice: body.Choice, Surrendered: false,
			})
		}
		respond(w, http.StatusOK, map[string]any{
			"war_id":      war.ID,
			"row_number":  game.CurrentRow,
			"opponent_id": body.OpponentID,
			"choice":      body.Choice,
		})
	}
}

// mwProposePeaceHandler handles POST /api/plans/:planId/propose-peace.
//
// Only the active participant whose turn it is to pay cost of battle (first
// in reverse power order among outstanding payers) may propose peace, because
// the proposal is itself the caller's cost-of-battle choice for that pair of
// (payer, opponent). The proposer does not record a battle_cost row until the
// vote concludes: if accepted the war ends (clearing all costs); if rejected
// the proposer must still pay using break_asset or leverage_two.
//
// Body: {"terms": "..."}
func mwProposePeaceHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if !mwCheckPlan(w, plan) {
			return
		}
		var body struct {
			Terms string `json:"terms"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Terms == "" {
			respondErr(w, http.StatusBadRequest, "terms required")
			return
		}
		ctx := r.Context()
		war, ok := mwLoadWar(ctx, w, deps.Q, plan)
		if !ok {
			return
		}
		if war.Status != "active" {
			respondErr(w, http.StatusConflict, "war is no longer active")
			return
		}
		if _, err := deps.Q.GetOpenPeaceProposal(ctx, war.ID); err == nil {
			respondErr(w, http.StatusConflict, "an open peace proposal already exists for this war")
			return
		}

		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load game")
			return
		}
		snap, err := mwSnapshotWar(ctx, deps.Q, war)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load war participants")
			return
		}
		ranks, err := mwPowerRanks(ctx, deps.Q, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load rankings")
			return
		}
		missing, err := mwOutstandingCostsForWar(ctx, deps.Q, snap, ranks, game.CurrentRow)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not compute outstanding costs")
			return
		}
		if len(missing) == 0 || missing[0].PayerID != player.ID {
			respondErr(w, http.StatusConflict,
				"only the active participant whose turn it is to pay cost of battle may propose peace")
			return
		}

		prop, err := deps.Q.CreatePeaceProposal(ctx, dbgen.CreatePeaceProposalParams{
			WarID:      war.ID,
			ProposerID: player.ID,
			Terms:      body.Terms,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create peace proposal")
			return
		}

		// Proposer's vote is implicitly accept.
		_ = deps.Q.UpsertPeaceVote(ctx, dbgen.UpsertPeaceVoteParams{
			ProposalID: prop.ID,
			PlayerID:   player.ID,
			Accepted:   true,
		})

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventWarPeaceProposed, model.WarPeaceProposedPayload{
				WarID: war.ID, ProposalID: prop.ID,
				ProposerID: player.ID, Terms: body.Terms,
			})
		}
		respond(w, http.StatusOK, map[string]any{
			"proposal_id": prop.ID,
			"war_id":      war.ID,
		})
	}
}

// mwVotePeaceHandler handles POST /api/plans/:planId/vote-peace.
//
// Body: {"proposal_id": int64, "accepted": bool}
//
// Any active participant may vote. A single "reject" closes the proposal; the
// proposer must then pay their cost with break_asset or leverage_two. Once
// every active participant has voted accept, the war ends with reason=peace.
func mwVotePeaceHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if !mwCheckPlan(w, plan) {
			return
		}
		var body struct {
			ProposalID int64 `json:"proposal_id"`
			Accepted   bool  `json:"accepted"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		ctx := r.Context()
		war, ok := mwLoadWar(ctx, w, deps.Q, plan)
		if !ok {
			return
		}
		prop, err := deps.Q.GetPeaceProposal(ctx, body.ProposalID)
		if err != nil || prop.WarID != war.ID {
			respondErr(w, http.StatusNotFound, "peace proposal not found")
			return
		}
		if prop.Status != gamepkg.PeaceOpen {
			respondErr(w, http.StatusConflict, "proposal is already resolved")
			return
		}

		// Caller must be an active participant.
		part, err := deps.Q.GetWarParticipant(ctx, dbgen.GetWarParticipantParams{
			WarID: war.ID, PlayerID: player.ID,
		})
		if err != nil {
			respondErr(w, http.StatusForbidden, "you are not a participant in this war")
			return
		}
		if part.SurrenderedAtRow != nil {
			respondErr(w, http.StatusForbidden, "surrendered players cannot vote on peace")
			return
		}

		err = deps.Q.UpsertPeaceVote(ctx, dbgen.UpsertPeaceVoteParams{
			ProposalID: prop.ID,
			PlayerID:   player.ID,
			Accepted:   body.Accepted,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not record vote")
			return
		}
		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventWarPeaceVote, model.WarPeaceVotePayload{
				WarID: war.ID, ProposalID: prop.ID,
				PlayerID: player.ID, Accepted: body.Accepted,
			})
		}

		// If this was a rejection, close the proposal immediately.
		if !body.Accepted {
			_ = deps.Q.SetPeaceProposalStatus(ctx, dbgen.SetPeaceProposalStatusParams{
				ID:     prop.ID,
				Status: gamepkg.PeaceRejected,
			})
			respond(w, http.StatusOK, map[string]any{
				"proposal_id": prop.ID,
				"status":      gamepkg.PeaceRejected,
			})
			return
		}

		// Check unanimity among active participants.
		active, err := deps.Q.ListActiveWarParticipants(ctx, war.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load active participants")
			return
		}
		votes, err := deps.Q.ListPeaceVotes(ctx, prop.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load votes")
			return
		}
		accepted := map[int64]bool{}
		for _, v := range votes {
			if v.Accepted {
				accepted[v.PlayerID] = true
			}
		}
		for _, a := range active {
			if !accepted[a.PlayerID] {
				respond(w, http.StatusOK, map[string]any{
					"proposal_id": prop.ID,
					"status":      gamepkg.PeaceOpen,
					"awaiting":    a.PlayerID,
				})
				return
			}
		}

		// Unanimous — accept the proposal and end the war.
		game, _ := deps.Q.GetGameByID(ctx, plan.GameID)
		_ = deps.Q.SetPeaceProposalStatus(ctx, dbgen.SetPeaceProposalStatusParams{
			ID:     prop.ID,
			Status: gamepkg.PeaceAccepted,
		})
		_ = deps.Q.EndWar(ctx, dbgen.EndWarParams{
			ID:         war.ID,
			EndReason:  stringPtr(gamepkg.WarEndPeace),
			EndedAtRow: &game.CurrentRow,
		})
		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventWarEnded, model.WarEndedPayload{
				WarID: war.ID, Reason: gamepkg.WarEndPeace, RowNumber: game.CurrentRow,
			})
		}
		respond(w, http.StatusOK, map[string]any{
			"proposal_id": prop.ID,
			"status":      gamepkg.PeaceAccepted,
			"war_ended":   true,
		})
	}
}
