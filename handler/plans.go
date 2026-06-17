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
	"slices"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// The plan orchestration contract (PlanHandler, OnPreparer, PlanDeps,
// ValidationContext, the registry, saveResolutionData) lives in
// plan_contract.go; pure domain data types live in the game package and are
// re-exported via plan_registry.go.

// broadcastEvent sends an event to all subscribers of gameID, if a hub exists
// for it. Replaces the repetitive `if h, ok := manager.Get(gameID); ok { ... }`
// pattern at every broadcast site.
func broadcastEvent(manager *hub.Manager, gameID int64, eventType string, payload any) {
	if h, ok := manager.Get(gameID); ok {
		h.BroadcastEvent(eventType, payload)
	}
}

// The DB-backed eligibility/ranking helpers (playerRankInCategory,
// playerHasPeers, checkPlanEligible, hasEsteemLockout) live in
// handler/eligibility.go — they query Postgres, so they belong in the shell.

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
		Stage:      "decide_vote",
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
	if err := seedRollParticipants(ctx, q, game.ID, roll.ID, actorID); err != nil {
		return nil, err
	}
	broadcastEvent(manager, game.ID, model.EventRollCreated, model.RollCreatedPayload{Roll: roll})
	return &roll, nil
}

// ── ListPlans ─────────────────────────────────────────────────────────────────

// ListPlans handles GET /api/tables/:id/plans.
func ListPlans(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		plans, err := s.Q.ListPlansByGame(r.Context(), gameID)
		if err != nil {
			respondInternalErr(w, r, "could not load plans", err)
			return
		}
		respond(w, http.StatusOK, map[string]any{"plans": plans})
	}
}

// ListPlanTokens handles GET /api/tables/:id/plan-tokens.
//
// Returns the plan tokens placed in the game — one per (plan_type, player)
// — so every client can render which players currently hold a token on each
// plan's shield. Tokens are placed when a player prepares a plan and cleared
// per-category at ranking updates, so this drives the prep-grid pips for all
// viewers (including read-only ones). Slimmed to the two fields the UI needs.
func ListPlanTokens(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		tokens, err := s.Q.ListPlanTokensByGame(r.Context(), gameID)
		if err != nil {
			respondInternalErr(w, r, "could not load plan tokens", err)
			return
		}
		type tokenEntry struct {
			PlanType model.PlanType `json:"plan_type"`
			PlayerID int64          `json:"player_id"`
		}
		out := make([]tokenEntry, len(tokens))
		for i, t := range tokens {
			out[i] = tokenEntry{PlanType: t.PlanType, PlayerID: t.PlayerID}
		}
		respond(w, http.StatusOK, map[string]any{"tokens": out})
	}
}

// ── PlanEligibility ───────────────────────────────────────────────────────────

// PlanEligibility handles GET /api/tables/:id/plan-eligibility.
//
// Returns which plan types the current player can prepare, and the computed
// target row for each eligible plan. Ineligible plans include a reason.
func PlanEligibility(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		game, err := s.Q.GetGameByID(r.Context(), gameID)
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
		hasPeers, err := playerHasPeers(ctx, s.Q, gameID, player.ID)
		if err != nil {
			respondInternalErr(w, r, "could not check peer assets", err)
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
			ok, reason, err := checkPlanEligible(ctx, s.Q, gameID, player.ID, planType, meta.Category)
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

// PreparePlanRequest is the request body for POST /api/tables/:id/prepare-plan.
type PreparePlanRequest struct {
	PlanType       model.PlanType `json:"plan_type"`
	TargetPlayerID *int64         `json:"target_player_id"`
	TargetAssetID  *int64         `json:"target_asset_id"`
	TargetPlanID   *int64         `json:"target_plan_id"`
	PeerCount      int16          `json:"peer_count"`
	EnemyPlayerIDs []int64        `json:"enemy_player_ids"`
	DuelType       string         `json:"duel_type"`
	// Clandestinely Liaise: the two SPECIFIC peers meeting — the preparer's own
	// peer and the partner's peer. Both required (see clHandler.ValidatePreparation).
	PreparerPeerID   *int64  `json:"preparer_peer_id"`
	PartnerPeerID    *int64  `json:"partner_peer_id"`
	PreparationNotes *string `json:"preparation_notes"`
	// SecretAssetID, when set on a Spread Rumors prep, marks "keep the rumor
	// secret for now": the rumor text is stored as a hidden Secret on this
	// (own) asset rather than the public plan note. See createPlanInTx.
	SecretAssetID *int64 `json:"secret_asset_id"`
}

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
func PreparePlan(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, player, ok := requireFocusPlayer(w, r, s.Q)
		if !ok {
			return
		}
		if game.Phase != model.PhaseMainEvent {
			respondErr(w, http.StatusConflict, "game is not in the main event phase")
			return
		}

		var body PreparePlanRequest
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
			ctx, s.Q, game, player,
			body.PlanType,
			body.TargetPlayerID,
			body.TargetAssetID,
			body.TargetPlanID,
			body.PeerCount,
			body.EnemyPlayerIDs,
			body.PreparerPeerID,
			body.PartnerPeerID,
			notes,
		)
		if validation.Status != http.StatusOK {
			if validation.EndgameChoiceRequired {
				respond(w, validation.Status, map[string]any{
					"error":                   validation.ErrMsg,
					"endgame_choice_required": true,
					"modes":                   []string{EndingModeSmoothLanding, EndingModeExplosiveFinale},
				})
				return
			}
			respondErr(w, validation.Status, validation.ErrMsg)
			return
		}

		// Reject duel_type early so we don't open a transaction for an
		// avoidable validation failure.
		if body.PlanType == model.PlanProposeDuel && body.DuelType != "arms" && body.DuelType != "wits" {
			respondErr(w, http.StatusBadRequest, "duel_type must be 'arms' or 'wits'")
			return
		}

		meta := validation.Meta
		// validation.TargetRow is nil for plans whose row is decided by a
		// post-prep simultaneous reveal (Make War, Clandestinely Liaise).
		// For these the row-count query is meaningless and row_number stays
		// NULL until the reveal closes (see applyMakeWarDelayResult /
		// reveals.go); row_order will be fixed up at that point.
		targetRow := validation.TargetRow

		var count int64
		if targetRow != nil {
			c, err := s.Q.CountPlansOnRow(ctx, dbgen.CountPlansOnRowParams{
				GameID:    game.ID,
				RowNumber: targetRow,
			})
			if err == nil {
				count = c
			}
		}

		var plan dbgen.Plan
		err := s.InTx(ctx, func(q *dbgen.Queries) error {
			var txErr error
			plan, txErr = createPlanInTx(ctx, q, s, game, player, &body, meta, targetRow, count, manager)
			return txErr
		})
		if err != nil {
			respondHTTPErr(w, r, err)
			return
		}

		broadcastEvent(manager, game.ID, model.EventPlanPrepared, model.PlanPayload{Plan: plan})
		EmitPlanPrepared(ctx, s.Q, manager, plan)
		broadcastRowState(ctx, s.Q, manager, game.ID)

		// Consume any pending counter-demand waiting on this player: the
		// target of a previously marred demand deferred their free counter
		// to "the next plan you prepare" — synthesize it now.
		counterPlanID := consumePendingCounterDemandFor(ctx, s.Q, manager, game, &plan)

		// Preparing a plan is the focus player's step-5 action; pass the
		// focus marker automatically so the prepare button is a one-click
		// commit rather than requiring a separate Pass Focus press. The
		// plan has already committed, so a failure here is logged and
		// recovered via the manual /pass-focus endpoint rather than
		// failing the request.
		if err := autoPassFocus(r, s, manager, game); err != nil {
			loggerFromContext(r.Context()).Error("auto pass-focus after prepare-plan", "err", err)
		}

		resp := map[string]any{"plan": plan}
		if counterPlanID != nil {
			resp["counter_demand_plan_id"] = *counterPlanID
		}
		respond(w, http.StatusCreated, resp)
	}
}

// createPlanInTx handles the database transaction for plan creation, including
// plan-specific initialization (resolution data, tokens, handler hooks).
//
// targetRow is nil for plans whose row is decided by a post-prep reveal
// (Make War, Clandestinely Liaise); the row stays NULL on creation and is
// filled in when the reveal closes.
//
// each branch is a sibling stash block (war enemies, liaise peers, duel type,
// demand target, secret rumor) and splitting the sequence obscures the order.
//
//nolint:funlen,gocognit // a flat per-plan-type dispatch of ordered prep steps;
func createPlanInTx(
	ctx context.Context,
	q *dbgen.Queries,
	s *db.Store,
	game *dbgen.Game,
	player *dbgen.Player,
	body *PreparePlanRequest,
	meta PlanMetadata,
	targetRow *int16,
	count int64,
	manager *hub.Manager,
) (dbgen.Plan, error) {
	rowOrder := int16(count)
	// Make Demands is an exception: instead of appending to the target's row,
	// it slots in immediately *before* the target so it resolves first. Take
	// the target's row_order, then shift the target and any later plans on
	// that row up by one. (See game.DemandPlacement.)
	if body.PlanType == model.PlanMakeDemands && body.TargetPlanID != nil && targetRow != nil {
		target, err := q.GetPlanByID(ctx, *body.TargetPlanID)
		if err != nil {
			return dbgen.Plan{}, httpErr(http.StatusBadRequest, "demand target not found")
		}
		rowOrder = target.RowOrder
		if err := q.ShiftRowOrderAtOrAfter(ctx, dbgen.ShiftRowOrderAtOrAfterParams{
			GameID:    game.ID,
			RowNumber: targetRow,
			RowOrder:  rowOrder,
		}); err != nil {
			return dbgen.Plan{}, httpErr(http.StatusInternalServerError, "could not shift row order: "+err.Error())
		}
	}

	plan, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:           game.ID,
		PlanType:         body.PlanType,
		Category:         meta.Category,
		PreparerID:       player.ID,
		TargetPlayerID:   body.TargetPlayerID,
		TargetAssetID:    body.TargetAssetID,
		RowNumber:        targetRow,
		RowOrder:         rowOrder,
		PreparedAtRow:    game.CurrentRow,
		PreparationNotes: body.PreparationNotes,
	})
	if err != nil {
		return dbgen.Plan{}, httpErr(http.StatusInternalServerError, "could not create plan: "+err.Error())
	}

	if body.PlanType == model.PlanMakeIntroductions {
		if err = miStoreResData(ctx, q, plan.ID, body.PeerCount); err != nil {
			return dbgen.Plan{}, httpErr(http.StatusInternalServerError, "could not save plan data")
		}
	}

	if body.PlanType == model.PlanMakeWar {
		resData := loadResolutionData(plan.ResolutionData)
		resData.EnsureMakeWar().EnemyPlayerIDs = body.EnemyPlayerIDs
		if err = saveResolutionData(ctx, q, plan.ID, resData); err != nil {
			return dbgen.Plan{}, httpErr(http.StatusInternalServerError, "could not save war enemies")
		}
		if refreshed, err := q.GetPlanByID(ctx, plan.ID); err == nil {
			plan = refreshed
		}
	}

	if body.PlanType == model.PlanClandestinelyLiaise {
		// Stash the two meeting peers before OnPrepare runs (OnPrepare has no
		// access to the request body, so structured prep data is stored here —
		// same pattern as Make War's enemy list above). OnPrepare then loads and
		// augments this resolution_data with the partner pointer + delay reveal.
		resData := loadResolutionData(plan.ResolutionData)
		ld := resData.EnsureLiaise()
		ld.PreparerPeerID = body.PreparerPeerID
		ld.PartnerPeerID = body.PartnerPeerID
		if err = saveResolutionData(ctx, q, plan.ID, resData); err != nil {
			return dbgen.Plan{}, httpErr(http.StatusInternalServerError, "could not save liaise meeting peers")
		}
		if refreshed, err := q.GetPlanByID(ctx, plan.ID); err == nil {
			plan = refreshed
		}
	}

	if body.PlanType == model.PlanProposeDuel {
		resData := loadResolutionData(plan.ResolutionData)
		resData.EnsureDuel().DuelType = body.DuelType
		if err = saveResolutionData(ctx, q, plan.ID, resData); err != nil {
			return dbgen.Plan{}, httpErr(http.StatusInternalServerError, "could not save duel type")
		}
		if refreshed, err := q.GetPlanByID(ctx, plan.ID); err == nil {
			plan = refreshed
		}
	}

	// Spread Rumors "keep it secret for now": move the rumor text into a hidden
	// Secret on one of the preparer's own assets (see stashSecretRumor) so other
	// players can't read it until a Make publishes it.
	if body.PlanType == model.PlanSpreadRumors && body.SecretAssetID != nil {
		if err = stashSecretRumor(ctx, q, game.ID, player.ID, *body.SecretAssetID, &plan); err != nil {
			return dbgen.Plan{}, err
		}
	}

	if body.PlanType == model.PlanMakeDemands && body.TargetPlanID != nil {
		if err = q.SetPlanTargetedPlan(ctx, dbgen.SetPlanTargetedPlanParams{
			ID:             plan.ID,
			TargetedPlanID: body.TargetPlanID,
		}); err != nil {
			return dbgen.Plan{}, httpErr(http.StatusInternalServerError, "could not persist demand target")
		}
		if refreshed, gErr := q.GetPlanByID(ctx, plan.ID); gErr == nil {
			plan = refreshed
		}
	}

	h, _ := GetHandler(body.PlanType)
	if preparer, ok := h.(OnPreparer); ok {
		deps := &PlanDeps{Store: s.WithQ(q), Manager: manager}
		if err := preparer.OnPrepare(ctx, deps, &plan); err != nil {
			return dbgen.Plan{}, httpErr(http.StatusInternalServerError, "could not initialise plan: "+err.Error())
		}
		// OnPrepare writes additional resolution_data directly to the DB (e.g.
		// liaise partner_id + delay_reveal_id, make_war delay_reveal_id) without
		// touching the in-memory struct. Refresh so the plan.prepared broadcast
		// carries the complete resolution_data — otherwise non-preparer clients,
		// which rely solely on that event, render with the fields missing.
		if refreshed, err := q.GetPlanByID(ctx, plan.ID); err == nil {
			plan = refreshed
		}
	}

	if _, err = q.CreatePlanToken(ctx, dbgen.CreatePlanTokenParams{
		GameID:   game.ID,
		PlanType: body.PlanType,
		PlayerID: player.ID,
		PlanID:   plan.ID,
	}); err != nil {
		return dbgen.Plan{}, httpErr(http.StatusInternalServerError, "could not place plan token")
	}
	return plan, nil
}

// ── GetPlan ───────────────────────────────────────────────────────────────────

// GetPlan handles GET /api/plans/:planId.
func GetPlan(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, _, ok := requirePlanAccess(w, r, s.Q)
		if !ok {
			return
		}

		resData := loadResolutionData(plan.ResolutionData)

		var difficulty int16
		if h, supported := GetHandler(plan.PlanType); supported {
			difficulty, _ = h.ComputeDifficulty(r.Context(), s.Q, plan, &resData)
		}

		respond(w, http.StatusOK, map[string]any{
			"plan":            plan,
			"difficulty":      difficulty,
			"resolution_data": resData,
		})
	}
}

// ── ResolvePlan ───────────────────────────────────────────────────────────────

// kickoffPlanResolution flips a pending plan to 'resolving', broadcasts the
// plan.resolving event, and invokes the plan handler's OnResolve hook (which
// usually creates a dice roll, but for some plan types performs other
// initialization or even fully resolves the plan, e.g. Make War).
//
// Caller responsibilities:
//   - plan must be in 'pending' status with row_number == game.current_row.
//     Callers that come from a freshly-computed RowState (kind=plan_pending)
//     satisfy this by construction.
//   - The caller is responsible for any row_state broadcast that should
//     follow. Most callers go through broadcastRowState which handles both
//     the kickoff and the final broadcast in a single helper call.
//
// Returns the dice roll if one was created, or nil. Errors are returned
// verbatim — the auto-kickoff path logs and leaves the plan pending; the
// HTTP endpoint surfaces them as 500.
func kickoffPlanResolution(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	plan *dbgen.Plan,
) (*dbgen.DiceRoll, error) {
	h, supported := GetHandler(plan.PlanType)
	if !supported {
		return nil, fmt.Errorf("no handler for plan type %q", plan.PlanType)
	}

	if err := q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID:     plan.ID,
		Status: model.PlanResolving,
	}); err != nil {
		return nil, fmt.Errorf("set plan status: %w", err)
	}
	// Refresh the local copy so the broadcast payload reflects the new status.
	plan.Status = model.PlanResolving

	if hub, hasHub := manager.Get(plan.GameID); hasHub {
		hub.BroadcastEvent(model.EventPlanResolving, model.PlanPayload{Plan: *plan})
	}
	EmitPlanResolving(ctx, q, manager, *plan)

	deps := &PlanDeps{Store: &db.Store{Q: q}, Manager: manager}
	return h.OnResolve(ctx, deps, plan)
}

// ResolvePlan handles POST /api/plans/:planId/resolve.
//
// Normally the kickoff happens automatically inside advanceAndBroadcastRowState
// whenever the table enters kind=plan_pending. This endpoint remains as a
// retry/escape hatch for the rare case where OnResolve fails — the row state
// stays pending and the focus player can re-trigger via this endpoint.
func ResolvePlan(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, plan, ok := requirePlanPreparer(w, r, s.Q)
		if !ok {
			return
		}
		if plan.Status != model.PlanPending {
			respondErr(w, http.StatusConflict, "plan is not in pending status")
			return
		}
		if plan.RowNumber == nil || *plan.RowNumber != game.CurrentRow {
			respondErr(w, http.StatusConflict, "plan is not scheduled for the current row")
			return
		}

		ctx := r.Context()
		roll, err := kickoffPlanResolution(ctx, s.Q, manager, plan)
		if err != nil {
			respondInternalErr(w, r, "could not begin resolution", err)
			return
		}
		broadcastRowState(ctx, s.Q, manager, game.ID)

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
func MakeChoice(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, s.Q)
		if !ok {
			return
		}
		game, err := s.Q.GetGameByID(r.Context(), plan.GameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}
		if game.Phase != model.PhaseMainEvent {
			respondErr(w, http.StatusConflict, "game is not in the main event phase")
			return
		}
		// Normally only the plan's preparer (the resolver) may make plan
		// choices. A few specific roles may also drive make-choice on the
		// preparer's plan; see makeChoiceAllowedNonPreparer for the list.
		isPreparer := player.ID == plan.PreparerID
		if !isPreparer && !makeChoiceAllowedNonPreparer(r.Context(), s.Q, plan, player) {
			respondErr(w, http.StatusForbidden, "only the plan's preparer can do this")
			return
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

		// Spread Rumors' "take asset" is consent-gated and must go through
		// request-take-consent (which names the specific assets and asks the
		// victim). A direct make-choice carrying it would commit the other
		// choices without ever obtaining consent, so reject it here.
		if plan.PlanType == model.PlanSpreadRumors && slices.Contains(body.Choices, "take_asset") {
			respondErr(w, http.StatusBadRequest,
				"take_asset requires the target's consent; use request-take-consent")
			return
		}

		ctx := r.Context()

		// Verify result matches the linked dice roll's outcome (if one exists).
		roll, rollErr := s.Q.GetDiceRollByPlanID(ctx, &plan.ID)
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

		var rollPtr *dbgen.DiceRoll
		if rollErr == nil {
			rollPtr = &roll
		}
		if !enforceChoiceBudget(w, h, rollPtr, body.Result, body.Choices) {
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		resData.MakeMarChoices = make([]Choice, len(body.Choices))
		for i, opt := range body.Choices {
			resData.MakeMarChoices[i] = Choice{Option: opt}
		}

		deps := &PlanDeps{Store: s, Manager: manager}
		if err := h.ApplyChoice(ctx, deps, plan, &resData, body.Choices, body.Result); err != nil {
			respondInternalErr(w, r, "could not apply plan effects", err)
			return
		}

		if err := saveResolutionData(ctx, s.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save choices", err)
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":              plan.ID,
			"choices":              body.Choices,
			"result":               body.Result,
			"messy_break_required": resData.ExchangeCourtiers != nil && resData.ExchangeCourtiers.MessyBreakRequired,
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
func CompletePlan(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, plan, ok := requirePlanPreparer(w, r, s.Q)
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
		roll, rollErr := s.Q.GetDiceRollByPlanID(ctx, &plan.ID)
		if rollErr == nil && roll.Outcome != nil {
			resultStr = *roll.Outcome
		} else if plan.Result != nil {
			resultStr = *plan.Result
		}
		if resultStr == "" {
			respondErr(w, http.StatusConflict, "cannot complete plan: no roll outcome and no stored result")
			return
		}

		if err := s.Q.SetPlanResult(ctx, dbgen.SetPlanResultParams{
			ID:     plan.ID,
			Result: &resultStr,
		}); err != nil {
			respondInternalErr(w, r, "could not complete plan", err)
			return
		}

		broadcastEvent(manager, plan.GameID, model.EventPlanResolved, model.PlanResolvedPayload{
			PlanID: plan.ID,
			Result: resultStr,
		})
		EmitPlanResolved(ctx, s.Q, manager, *plan, resultStr)
		broadcastRowState(ctx, s.Q, manager, plan.GameID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID,
			"result":  resultStr,
		})
	}
}
