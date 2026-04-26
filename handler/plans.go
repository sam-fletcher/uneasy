package handler

// handler/plans.go — Common plan lifecycle handlers (Phase 3a+).
//
// This file contains infrastructure shared by all plan types plus the
// HTTP handlers for the common endpoints (list, prepare, resolve, make-choice,
// complete). Plan-type-specific logic now lives in individual plan_*.go files
// and is dispatched through the registry defined in plan_registry.go.
//
// Phase 3a plans:
//
//	Exchange Courtiers  (power,     delay 5) — plan_exchange_courtiers.go
//	Make Introductions  (knowledge, delay 3) — plan_make_introductions.go
//	Spread Propaganda   (esteem,    delay 3) — plan_spread_propaganda.go
//
// Phase 3b plans:
//
//	Seek Answers        (knowledge, delay 4) — plan_seek_answers.go
//	Spread Rumors       (esteem,    delay 4) — plan_spread_rumors.go
//	Chronicle Histories (knowledge, delay 5) — plan_chronicle_histories.go
//
// Resolution lifecycle:
//
//  1. Focus player calls prepare-plan → plan created at current_row + delay.
//  2. When current_row reaches the plan's row_number, the focus player calls
//     resolve → plan enters 'resolving'. h.OnResolve() is called; for most
//     plans a dice roll is created; for EC nil is returned (fair trade first).
//  3. Dice roll plays out via the existing roll endpoints, plus any plan-
//     specific extra routes registered by h.ExtraRoutes().
//  4. After the roll resolves, focus player calls make-choice; h.ApplyChoice()
//     applies server-side mechanical effects.
//  5. Focus player calls complete; h.CanComplete() guards any pending steps.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// OnPreparer, loadResolutionData, saveResolutionData, and all domain types
// are now defined in the game package and re-exported via plan_registry.go.

// ── Access helpers ────────────────────────────────────────────────────────────

// requirePlanAccess parses planId and verifies the caller belongs to the plan's game.
func requirePlanAccess(
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
) (*dbgen.Plan, *dbgen.Player, bool) {
	planID, err := strconv.ParseInt(chi.URLParam(r, "planId"), 10, 64)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid plan id")
		return nil, nil, false
	}
	plan, err := q.GetPlanByID(r.Context(), planID)
	if err != nil {
		respondErr(w, http.StatusNotFound, "plan not found")
		return nil, nil, false
	}
	player, ok := requirePlayerInGame(w, r, q, plan.GameID)
	if !ok {
		return nil, nil, false
	}
	return &plan, player, true
}

// requirePlanFocus returns the game, plan, and player, verifying the caller is
// the focus player and the game is in main_event phase.
func requirePlanFocus(
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
) (*dbgen.Game, *dbgen.Plan, *dbgen.Player, bool) {
	plan, player, ok := requirePlanAccess(w, r, q)
	if !ok {
		return nil, nil, nil, false
	}
	game, err := q.GetGameByID(r.Context(), plan.GameID)
	if err != nil {
		respondErr(w, http.StatusNotFound, "table not found")
		return nil, nil, nil, false
	}
	if game.Phase != model.PhaseMainEvent {
		respondErr(w, http.StatusConflict, "game is not in the main event phase")
		return nil, nil, nil, false
	}
	if game.FocusPlayerID == nil || *game.FocusPlayerID != player.ID {
		respondErr(w, http.StatusForbidden, "only the focus player can do this")
		return nil, nil, nil, false
	}
	return &game, plan, player, true
}

// requirePlanType writes a 400 and returns false if the plan isn't of the
// expected type. Used by plan-specific extra-route handlers to guard against
// a route being called with the wrong plan ID.
func requirePlanType(w http.ResponseWriter, plan *dbgen.Plan, want model.PlanType) bool {
	if plan.PlanType != want {
		respondErr(w, http.StatusBadRequest, "route is only for "+string(want)+" plans")
		return false
	}
	return true
}

// requirePlanResolving writes a 409 and returns false if the plan isn't in
// the resolving status. Several plans' extra routes fire only during the
// resolving phase.
func requirePlanResolving(w http.ResponseWriter, plan *dbgen.Plan) bool {
	if plan.Status != model.PlanResolving {
		respondErr(w, http.StatusConflict, "plan is not in resolving status")
		return false
	}
	return true
}

// Pure game-rule helpers (playerRankInCategory, playerHasPeers,
// checkPlanEligible, hasEsteemLockout) are defined in the game package and
// re-exported as handler-package aliases in plan_registry.go.

// ── createPlanRoll ────────────────────────────────────────────────────────────

func createPlanRoll(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	game *dbgen.Game,
	plan *dbgen.Plan,
	difficulty int16,
	actorID int64,
) (*dbgen.DiceRoll, error) {
	roll, err := q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
		GameID:     game.ID,
		PlanID:     new(plan.ID),
		RowNumber:  new(game.CurrentRow),
		ActorID:    actorID,
		Difficulty: difficulty,
	})
	if err != nil {
		return nil, err
	}
	for range 2 {
		if _, err := q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
			RollID:           roll.ID,
			PlayerID:         actorID,
			IsInterference:   false,
			LeveragedAssetID: nil,
		}); err != nil {
			return nil, err
		}
	}
	if h, ok := manager.Get(game.ID); ok {
		h.BroadcastEvent(model.EventRollCreated, model.RollCreatedPayload{Roll: roll})
	}
	return &roll, nil
}

// ── validateExchangeCourtiersPlan ─────────────────────────────────────────────

// validateExchangeCourtiersPlan is kept here because it's called from
// ecHandler.ValidatePreparation (in plan_exchange_courtiers.go) and both live
// in package handler. Moving it to the EC file avoids a circular dependency.
func validateExchangeCourtiersPlan(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
	targetPlayerID *int64,
	targetAssetID *int64,
) string {
	if targetPlayerID == nil || targetAssetID == nil {
		return "exchange_courtiers requires target_player_id and target_asset_id"
	}

	asset, err := q.GetAssetByID(ctx, *targetAssetID)
	if err != nil {
		return "target asset not found"
	}
	if asset.OwnerID != *targetPlayerID {
		return "target asset does not belong to target player"
	}
	if asset.AssetType != model.AssetPeer {
		return "exchange_courtiers target must be a peer asset"
	}

	targetHasPeers, err := playerHasPeers(ctx, q, gameID, *targetPlayerID)
	if err != nil {
		return "could not check target peer assets"
	}
	if !targetHasPeers {
		return "target player has no peers"
	}

	return ""
}

// ── preparePlanValidation ─────────────────────────────────────────────────────

type preparePlanValidation struct {
	Status    int
	ErrMsg    string
	TargetRow int16
	Meta      PlanMetadata
}

// validatePlanPreparation performs all common checks for plan preparation
// and delegates plan-specific validation to the registered handler.
func validatePlanPreparation(
	ctx context.Context,
	q *dbgen.Queries,
	game *dbgen.Game,
	player *dbgen.Player,
	planType model.PlanType,
	targetPlayerID *int64,
	targetAssetID *int64,
	targetPlanID *int64,
	peerCount int16,
	enemyPlayerIDs []int64,
	notes string,
) preparePlanValidation {
	// Check game phase.
	if game.Phase != model.PhaseMainEvent {
		return preparePlanValidation{
			Status: http.StatusConflict,
			ErrMsg: "game is not in the main event phase",
		}
	}

	// Resolve handler from registry.
	h, supported := GetHandler(planType)
	if !supported {
		return preparePlanValidation{
			Status: http.StatusBadRequest,
			ErrMsg: "unsupported plan type",
		}
	}
	meta := h.Metadata()

	// Check esteem lockout (SP mar option b "censured") before eligibility.
	// Any esteem-category plan is blocked while a lockout is active.
	if meta.Category == model.CategoryEsteem {
		locked, lockErr := hasEsteemLockout(ctx, q, game.ID, player.ID)
		if lockErr == nil && locked {
			return preparePlanValidation{
				Status: http.StatusForbidden,
				ErrMsg: "esteem lockout: your next plan must be a non-esteem plan (Spread Propaganda mar censured)",
			}
		}
	}

	// Check eligibility.
	eligible, reason, err := checkPlanEligible(ctx, q, game.ID, player.ID, planType, meta.Category)
	if err != nil {
		return preparePlanValidation{
			Status: http.StatusInternalServerError,
			ErrMsg: "could not check eligibility",
		}
	}
	if !eligible {
		return preparePlanValidation{
			Status: http.StatusForbidden,
			ErrMsg: reason,
		}
	}

	// Compute target row.
	// For variable-delay plans (Delay == -1), ValidatePreparation returns the row.
	// For fixed-delay plans, we compute it from the metadata.
	vc := &ValidationContext{
		Q:              q,
		Game:           game,
		Player:         player,
		TargetPlayerID: targetPlayerID,
		TargetAssetID:  targetAssetID,
		TargetPlanID:   targetPlanID,
		PeerCount:      peerCount,
		EnemyPlayerIDs: enemyPlayerIDs,
		Notes:          notes,
	}
	handlerTargetRow, errMsg := h.ValidatePreparation(ctx, vc)
	if errMsg != "" {
		return preparePlanValidation{
			Status: http.StatusBadRequest,
			ErrMsg: errMsg,
		}
	}

	var targetRow int16
	if meta.Delay == -1 {
		targetRow = handlerTargetRow
	} else {
		targetRow = game.CurrentRow + meta.Delay
	}

	// Check target row bounds.
	if targetRow > publicRecordRowCount {
		return preparePlanValidation{
			Status: http.StatusConflict,
			ErrMsg: "plan would be placed past row 13",
		}
	}

	// Check preparer has peers.
	hasPeers, err := playerHasPeers(ctx, q, game.ID, player.ID)
	if err != nil {
		return preparePlanValidation{
			Status: http.StatusInternalServerError,
			ErrMsg: "could not check peer assets",
		}
	}
	if !hasPeers {
		return preparePlanValidation{
			Status: http.StatusForbidden,
			ErrMsg: "you have no peers — a player without peers cannot prepare plans",
		}
	}

	return preparePlanValidation{
		Status:    http.StatusOK,
		TargetRow: targetRow,
		Meta:      meta,
	}
}

// ── ListPlans ─────────────────────────────────────────────────────────────────

// ListPlans handles GET /api/tables/:id/plans.
func ListPlans(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, q)
		if !ok {
			return
		}
		plans, err := q.ListPlansByGame(r.Context(), gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load plans")
			return
		}
		respond(w, http.StatusOK, map[string]any{"plans": plans})
	}
}

// ── PlanEligibility ───────────────────────────────────────────────────────────

// PlanEligibility handles GET /api/tables/:id/plan-eligibility.
//
// Returns which plan types the current player can prepare, and the computed
// target row for each eligible plan. Ineligible plans include a reason.
func PlanEligibility(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, q)
		if !ok {
			return
		}
		game, err := q.GetGameByID(r.Context(), gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}
		if game.Phase != model.PhaseMainEvent {
			respond(w, http.StatusOK, map[string]any{
				"eligible":   []any{},
				"ineligible": []any{},
			})
			return
		}

		type eligibleEntry struct {
			PlanType  model.PlanType        `json:"plan_type"`
			Category  model.RankingCategory `json:"category"`
			Delay     int16                 `json:"delay"`
			TargetRow int16                 `json:"target_row"`
		}
		type ineligibleEntry struct {
			PlanType model.PlanType        `json:"plan_type"`
			Category model.RankingCategory `json:"category"`
			Reason   string                `json:"reason"`
		}

		var eligible []eligibleEntry
		var ineligible []ineligibleEntry

		ctx := r.Context()

		// A player with no peers cannot prepare any plans.
		hasPeers, err := playerHasPeers(ctx, q, gameID, player.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not check peer assets")
			return
		}
		if !hasPeers {
			for planType, h := range AllHandlers() {
				meta := h.Metadata()
				ineligible = append(ineligible, ineligibleEntry{
					PlanType: planType,
					Category: meta.Category,
					Reason:   "you have no peers — a player without peers cannot prepare plans",
				})
			}
			respond(w, http.StatusOK, map[string]any{
				"eligible":   eligible,
				"ineligible": ineligible,
			})
			return
		}

		for planType, h := range AllHandlers() {
			meta := h.Metadata()

			// Compute target row (variable-delay plans return -1 delay; skip row
			// check here since we can't compute without player input).
			var targetRow int16
			if meta.Delay == -1 {
				eligible = append(eligible, eligibleEntry{
					PlanType:  planType,
					Category:  meta.Category,
					Delay:     -1,
					TargetRow: -1, // variable; depends on player input
				})
				continue
			}
			targetRow = game.CurrentRow + meta.Delay

			if targetRow > publicRecordRowCount {
				ineligible = append(ineligible, ineligibleEntry{
					PlanType: planType,
					Category: meta.Category,
					Reason:   "no room on the public record (would exceed row 13)",
				})
				continue
			}
			ok, reason, err := checkPlanEligible(ctx, q, gameID, player.ID, planType, meta.Category)
			if err != nil {
				ineligible = append(ineligible, ineligibleEntry{
					PlanType: planType,
					Category: meta.Category,
					Reason:   "could not check eligibility",
				})
				continue
			}
			if ok {
				eligible = append(eligible, eligibleEntry{
					PlanType:  planType,
					Category:  meta.Category,
					Delay:     meta.Delay,
					TargetRow: targetRow,
				})
			} else {
				ineligible = append(ineligible, ineligibleEntry{
					PlanType: planType,
					Category: meta.Category,
					Reason:   reason,
				})
			}
		}

		respond(w, http.StatusOK, map[string]any{
			"eligible":   eligible,
			"ineligible": ineligible,
		})
	}
}

// ── PreparePlan ───────────────────────────────────────────────────────────────

// PreparePlan handles POST /api/tables/:id/prepare-plan.
//
// Request body:
//
//	{
//	  "plan_type":          "exchange_courtiers"|"make_introductions"|...,
//	  "target_player_id":   123,   // plan-type-specific; optional
//	  "target_asset_id":    456,   // plan-type-specific; optional
//	  "peer_count":         2,     // Make Introductions: number of peers (1–4)
//	  "preparation_notes":  "..."  // optional flavor text
//	}
func PreparePlan(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, player, ok := requireFocusPlayer(w, r, q)
		if !ok {
			return
		}
		if game.Phase != model.PhaseMainEvent {
			respondErr(w, http.StatusConflict, "game is not in the main event phase")
			return
		}

		var body struct {
			PlanType         model.PlanType `json:"plan_type"`
			TargetPlayerID   *int64         `json:"target_player_id"`
			TargetAssetID    *int64         `json:"target_asset_id"`
			TargetPlanID     *int64         `json:"target_plan_id"`
			PeerCount        int16          `json:"peer_count"`
			EnemyPlayerIDs   []int64        `json:"enemy_player_ids"`
			DuelType         string         `json:"duel_type"`
			PreparationNotes *string        `json:"preparation_notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()

		notes := ""
		if body.PreparationNotes != nil {
			notes = *body.PreparationNotes
		}

		validation := validatePlanPreparation(
			ctx, q, game, player,
			body.PlanType,
			body.TargetPlayerID,
			body.TargetAssetID,
			body.TargetPlanID,
			body.PeerCount,
			body.EnemyPlayerIDs,
			notes,
		)
		if validation.Status != http.StatusOK {
			respondErr(w, validation.Status, validation.ErrMsg)
			return
		}

		meta := validation.Meta
		targetRow := validation.TargetRow

		count, err := q.CountPlansOnRow(ctx, dbgen.CountPlansOnRowParams{
			GameID:    game.ID,
			RowNumber: targetRow,
		})
		if err != nil {
			count = 0
		}

		plan, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
			GameID:           game.ID,
			PlanType:         body.PlanType,
			Category:         meta.Category,
			PreparerID:       player.ID,
			TargetPlayerID:   body.TargetPlayerID,
			TargetAssetID:    body.TargetAssetID,
			RowNumber:        targetRow,
			RowOrder:         int16(count),
			PreparedAtRow:    game.CurrentRow,
			PreparationNotes: body.PreparationNotes,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create plan")
			return
		}

		// Store peer_count for Make Introductions.
		if body.PlanType == model.PlanMakeIntroductions {
			if err := miStoreResData(ctx, q, plan.ID, body.PeerCount); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not save plan data")
				return
			}
		}

		// Persist enemy list for Make War so OnPrepare can enrol participants.
		if body.PlanType == model.PlanMakeWar {
			resData := loadResolutionData(plan.ResolutionData)
			resData.WarEnemyPlayerIDs = body.EnemyPlayerIDs
			if err := saveResolutionData(ctx, q, plan.ID, resData); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not save war enemies")
				return
			}
			// Reload so OnPrepare sees the persisted enemy list.
			refreshed, err := q.GetPlanByID(ctx, plan.ID)
			if err == nil {
				plan = refreshed
			}
		}

		// Persist duel_type for Propose Duel so it survives into resolution.
		if body.PlanType == model.PlanProposeDuel {
			if body.DuelType != "arms" && body.DuelType != "wits" {
				respondErr(w, http.StatusBadRequest, "duel_type must be 'arms' or 'wits'")
				return
			}
			resData := loadResolutionData(plan.ResolutionData)
			resData.DuelType = body.DuelType
			if err := saveResolutionData(ctx, q, plan.ID, resData); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not save duel type")
				return
			}
			refreshed, err := q.GetPlanByID(ctx, plan.ID)
			if err == nil {
				plan = refreshed
			}
		}

		// Persist targeted_plan_id for Make Demands so OnPrepare / ComputeDifficulty
		// can resolve the target without re-reading resolution_data.
		if body.PlanType == model.PlanMakeDemands && body.TargetPlanID != nil {
			if err := q.SetPlanTargetedPlan(ctx, dbgen.SetPlanTargetedPlanParams{
				ID:             plan.ID,
				TargetedPlanID: body.TargetPlanID,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not persist demand target")
				return
			}
			refreshed, err := q.GetPlanByID(ctx, plan.ID)
			if err == nil {
				plan = refreshed
			}
		}

		// Run optional post-creation setup (e.g. CL creates its simultaneous reveal).
		h, _ := GetHandler(body.PlanType)
		if preparer, ok := h.(OnPreparer); ok {
			deps := &PlanDeps{Q: q, Manager: manager}
			if err := preparer.OnPrepare(ctx, deps, &plan); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not initialise plan: "+err.Error())
				return
			}
		}

		if _, err := q.CreatePlanToken(ctx, dbgen.CreatePlanTokenParams{
			GameID:   game.ID,
			PlanType: body.PlanType,
			PlayerID: player.ID,
			PlanID:   plan.ID,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not place plan token")
			return
		}

		if h, ok := manager.Get(game.ID); ok {
			h.BroadcastEvent(model.EventPlanPrepared, model.PlanPayload{Plan: plan})
		}

		// Consume any pending counter-demand waiting on this player: the
		// target of a previously marred demand deferred their free counter
		// to "the next plan you prepare" — synthesize it now.
		counterPlanID := consumePendingCounterDemandFor(ctx, q, manager, game, &plan)

		resp := map[string]any{"plan": plan}
		if counterPlanID != nil {
			resp["counter_demand_plan_id"] = *counterPlanID
		}
		respond(w, http.StatusCreated, resp)
	}
}

// ── GetPlan ───────────────────────────────────────────────────────────────────

// GetPlan handles GET /api/plans/:planId.
func GetPlan(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, _, ok := requirePlanAccess(w, r, q)
		if !ok {
			return
		}

		resData := loadResolutionData(plan.ResolutionData)

		var difficulty int16
		if h, supported := GetHandler(plan.PlanType); supported {
			difficulty, _ = h.ComputeDifficulty(r.Context(), q, plan, &resData)
		}

		respond(w, http.StatusOK, map[string]any{
			"plan":            plan,
			"difficulty":      difficulty,
			"resolution_data": resData,
		})
	}
}

// ── ResolvePlan ───────────────────────────────────────────────────────────────

// ResolvePlan handles POST /api/plans/:planId/resolve.
//
// Focus player begins resolution. Sets the plan to 'resolving', then calls
// h.OnResolve() which creates the dice roll (or returns nil for plans that
// have a custom pre-roll flow, like Exchange Courtiers).
func ResolvePlan(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, plan, _, ok := requirePlanFocus(w, r, q)
		if !ok {
			return
		}
		if plan.Status != model.PlanPending {
			respondErr(w, http.StatusConflict, "plan is not in pending status")
			return
		}
		if plan.RowNumber != game.CurrentRow {
			respondErr(w, http.StatusConflict, "plan is not scheduled for the current row")
			return
		}

		h, supported := GetHandler(plan.PlanType)
		if !supported {
			respondErr(w, http.StatusInternalServerError, "no handler for this plan type")
			return
		}

		ctx := r.Context()

		if err := q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
			ID:     plan.ID,
			Status: model.PlanResolving,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not update plan status")
			return
		}

		if h, hasHub := manager.Get(game.ID); hasHub {
			h.BroadcastEvent(model.EventPlanResolving, model.PlanPayload{Plan: *plan})
		}

		deps := &PlanDeps{Q: q, Manager: manager}
		roll, err := h.OnResolve(ctx, deps, plan)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not begin resolution: "+err.Error())
			return
		}

		resp := map[string]any{"plan_id": plan.ID}
		if roll != nil {
			resp["roll"] = roll
		}
		respond(w, http.StatusOK, resp)
	}
}

// ── MakeChoice ────────────────────────────────────────────────────────────────

// MakeChoice handles POST /api/plans/:planId/make-choice.
//
// Called after the dice roll resolves. Records the make/mar option choices
// and executes any server-side mechanical effects via h.ApplyChoice().
//
// Request body:
//
//	{
//	  "choices": ["legal"],  // option key strings (plan-specific)
//	  "result": "make"       // "make" or "mar" — must match the roll outcome
//	}
func MakeChoice(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, q)
		if !ok {
			return
		}
		game, err := q.GetGameByID(r.Context(), plan.GameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}
		if game.Phase != model.PhaseMainEvent {
			respondErr(w, http.StatusConflict, "game is not in the main event phase")
			return
		}
		// Normally only the focus player may make plan choices. A Make Demands
		// perform_steps winner can submit preparer-equivalent choices on the
		// target plan even if they're not the focus player.
		isFocus := game.FocusPlayerID != nil && *game.FocusPlayerID == player.ID
		if !isFocus {
			_, winners, werr := gamepkg.DemandWinnersForTargetPlan(r.Context(), q, plan)
			allowed := false
			if werr == nil {
				if winnerID, ok := winners[gamepkg.DemandOptionPerformSteps]; ok && winnerID != 0 &&
					winnerID == player.ID &&
					winnerID != plan.PreparerID {
					allowed = true
				}
			}
			// Propose Duel mar: the target takes staked assets from the preparer,
			// so the target picks which stakes to claim.
			if !allowed && plan.PlanType == model.PlanProposeDuel && plan.TargetPlayerID != nil &&
				*plan.TargetPlayerID == player.ID {
				if roll, rerr := q.GetDiceRollByPlanID(r.Context(), &plan.ID); rerr == nil &&
					roll.Outcome != nil && *roll.Outcome == marOutcome {
					allowed = true
				}
			}

			// Spread Rumors mar: the target-asset owner drives make-choice with
			// the counter-rumor options (applied to preparer's assets).
			if !allowed && plan.PlanType == model.PlanSpreadRumors && plan.TargetAssetID != nil {
				if asset, aerr := q.GetAssetByID(
					r.Context(),
					*plan.TargetAssetID,
				); aerr == nil &&
					asset.OwnerID == player.ID {
					if roll, rerr := q.GetDiceRollByPlanID(r.Context(), &plan.ID); rerr == nil &&
						roll.Outcome != nil && *roll.Outcome == marOutcome {
						allowed = true
					}
				}
			}
			if !allowed {
				respondErr(w, http.StatusForbidden, "only the focus player can do this")
				return
			}
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		var body struct {
			Choices []string `json:"choices"`
			Result  string   `json:"result"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Result != makeOutcome && body.Result != marOutcome {
			respondErr(w, http.StatusBadRequest, "result must be 'make' or 'mar'")
			return
		}

		ctx := r.Context()

		// Verify result matches the linked dice roll's outcome (if one exists).
		roll, rollErr := q.GetDiceRollByPlanID(ctx, &plan.ID)
		if rollErr == nil && roll.Outcome != nil && *roll.Outcome != body.Result {
			respondErr(w, http.StatusConflict,
				fmt.Sprintf("result '%s' does not match roll outcome '%s'", body.Result, *roll.Outcome))
			return
		}

		h, supported := GetHandler(plan.PlanType)
		if !supported {
			respondErr(w, http.StatusInternalServerError, "no handler for this plan type")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		resData.Choices = body.Choices

		deps := &PlanDeps{Q: q, Manager: manager}
		if err := h.ApplyChoice(ctx, deps, plan, &resData, body.Choices, body.Result); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not apply plan effects: "+err.Error())
			return
		}

		if err := saveResolutionData(ctx, q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save choices")
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":              plan.ID,
			"choices":              body.Choices,
			"result":               body.Result,
			"messy_break_required": resData.MessyBreakRequired,
		})
	}
}

// ── CompletePlan ──────────────────────────────────────────────────────────────

// CompletePlan handles POST /api/plans/:planId/complete.
//
// Marks the plan as resolved. Calls h.CanComplete() to check for any pending
// prerequisites (e.g. EC messy break). The result is taken from the linked
// dice roll's outcome; if no roll exists the result is read from the plan's
// stored result field (e.g. EC fair trade accept path).
func CompletePlan(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, plan, _, ok := requirePlanFocus(w, r, q)
		if !ok {
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		h, supported := GetHandler(plan.PlanType)
		if !supported {
			respondErr(w, http.StatusInternalServerError, "no handler for this plan type")
			return
		}

		ctx := r.Context()

		resData := loadResolutionData(plan.ResolutionData)
		if err := h.CanComplete(plan, &resData); err != nil {
			respondErr(w, http.StatusConflict, "cannot complete plan: "+err.Error())
			return
		}

		// Determine result from roll outcome or existing plan result (fair trade).
		resultStr := ""
		roll, rollErr := q.GetDiceRollByPlanID(ctx, &plan.ID)
		if rollErr == nil && roll.Outcome != nil {
			resultStr = *roll.Outcome
		} else if plan.Result != nil {
			resultStr = *plan.Result
		}
		if resultStr == "" {
			respondErr(w, http.StatusConflict, "cannot complete plan: no roll outcome and no stored result")
			return
		}

		if err := q.SetPlanResult(ctx, dbgen.SetPlanResultParams{
			ID:     plan.ID,
			Result: &resultStr,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not complete plan")
			return
		}

		if h, ok := manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventPlanResolved, model.PlanResolvedPayload{
				PlanID: plan.ID,
				Result: resultStr,
			})
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID,
			"result":  resultStr,
		})
	}
}
