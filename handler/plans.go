package handler

// handler/plans.go — Plan preparation and resolution (Phase 2f).
//
// Three plans are supported in Phase 2:
//
//	Exchange Courtiers  (power,     delay 5)
//	Make Introductions  (knowledge, delay 3)
//	Spread Propaganda   (esteem,    delay 3)
//
// Resolution lifecycle:
//
//  1. Focus player calls prepare-plan → plan created at current_row + delay.
//  2. When current_row reaches the plan's row_number, the focus player calls
//     resolve → plan enters 'resolving' state.
//     • Make Introductions / Spread Propaganda: dice roll auto-created.
//     • Exchange Courtiers: fair trade step first (see fair-trade endpoint).
//  3. EC only — target player offers a peer via fair-trade (action: "offer").
//     Preparer accepts (action: "accept") or declines (action: "decline").
//     Accept → plan resolves without a roll. Decline → dice roll created.
//  4. Dice roll plays out via the existing roll endpoints.
//  5. After the roll resolves (outcome: make/mar), focus player calls
//     make-choice to apply the mechanical effects and record option choices.
//     EC make + "messy": the target player must then break a marginalia via
//     the messy-break endpoint before the plan can be completed.
//  6. Focus player calls complete to mark the plan resolved.
//
// Ranking update (engrailed lines): see handler/ranking.go.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// ── Plan type metadata ────────────────────────────────────────────────────────

type planMetadata struct {
	category model.RankingCategory
	delay    int16
}

//nolint:exhaustive,gochecknoglobals // Only Phase 2 plans are supported (future phases will extend); must be global for metadata lookup.
var phase2PlanMeta = map[model.PlanType]planMetadata{
	model.PlanExchangeCourtiers: {model.CategoryPower, 5},
	model.PlanMakeIntroductions: {model.CategoryKnowledge, 3},
	model.PlanSpreadPropaganda:  {model.CategoryEsteem, 3},
}

// ── Resolution data ───────────────────────────────────────────────────────────

// planResData holds plan-specific state stored as JSON in plans.resolution_data.
type planResData struct {
	// Make Introductions: number of peers being introduced (1–4).
	PeerCount int16 `json:"peer_count,omitempty"`
	// Exchange Courtiers: the peer asset the target offered as fair trade.
	FairTradeAssetID *int64 `json:"fair_trade_asset_id,omitempty"`
	// Exchange Courtiers: nil = no decision yet; true = accepted; false = declined.
	FairTradeAccepted *bool `json:"fair_trade_accepted,omitempty"`
	// Option keys chosen by the focus player from the make/mar list.
	Choices []string `json:"choices,omitempty"`
	// Exchange Courtiers, make + "messy": the target must break a marginalia.
	// MessyBreakRequired is set to true by MakeChoice when "messy" is chosen.
	MessyBreakRequired bool `json:"messy_break_required,omitempty"`
	// MessyBreakDone is set to true by the MessyBreak endpoint once complete.
	MessyBreakDone bool `json:"messy_break_done,omitempty"`
}

func loadResData(raw *string) planResData {
	if raw == nil || *raw == "" {
		return planResData{}
	}
	var d planResData
	_ = json.Unmarshal([]byte(*raw), &d)
	return d
}

func saveResData(ctx context.Context, q *dbgen.Queries, planID int64, d planResData) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	s := string(b)
	return q.SetPlanResolutionData(ctx, dbgen.SetPlanResolutionDataParams{ID: planID, ResolutionData: &s})
}

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
	player := appMiddleware.PlayerFromContext(r.Context())
	if player == nil || player.GameID != plan.GameID {
		respondErr(w, http.StatusForbidden, "not a member of this table")
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

// ── Ranking helpers ───────────────────────────────────────────────────────────

// playerRankInCategory returns the player's rank (1–5) in the given category.
// Returns 0 and a non-nil error if the player has no ranking.
func playerRankInCategory(
	ctx context.Context,
	q *dbgen.Queries,
	gameID, playerID int64,
	category model.RankingCategory,
) (int16, error) {
	r, err := q.GetRanking(ctx, dbgen.GetRankingParams{
		GameID:   gameID,
		PlayerID: &playerID,
		Category: category,
	})
	if err != nil {
		return 0, err
	}
	return r.Rank, nil
}

// ── Peer helpers ─────────────────────────────────────────────────────────────

// playerHasPeers reports whether a player has at least one non-destroyed peer
// asset in the game. Players with no peers cannot be focus player or prepare plans.
func playerHasPeers(ctx context.Context, q *dbgen.Queries, gameID, playerID int64) (bool, error) {
	count, err := q.CountPeerAssets(ctx, dbgen.CountPeerAssetsParams{
		GameID:  gameID,
		OwnerID: playerID,
	})
	return count > 0, err
}

// ── Eligibility ───────────────────────────────────────────────────────────────

// checkPlanEligible reports whether playerID may prepare planType.
// Returns (true, "", nil) if eligible, (false, reason, nil) if not.
func checkPlanEligible(
	ctx context.Context,
	q *dbgen.Queries,
	gameID, playerID int64,
	planType model.PlanType,
	category model.RankingCategory,
) (bool, string, error) {
	// A player can't have a duplicate token on the same plan type.
	_, err := q.GetPlanTokenByTypeAndPlayer(ctx, dbgen.GetPlanTokenByTypeAndPlayerParams{
		GameID:   gameID,
		PlanType: planType,
		PlayerID: playerID,
	})
	if err == nil {
		return false, "you already have a token on this plan type", nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, "", err
	}

	myRank, err := playerRankInCategory(ctx, q, gameID, playerID, category)
	if err != nil {
		return false, "could not determine your ranking", err
	}

	// Ineligible if any other player with a better rank (lower rank#) has a token.
	tokens, err := q.ListPlanTokensByType(ctx, dbgen.ListPlanTokensByTypeParams{
		GameID:   gameID,
		PlanType: planType,
	})
	if err != nil {
		return false, "", err
	}
	for _, tok := range tokens {
		theirRank, err := playerRankInCategory(ctx, q, gameID, tok.PlayerID, category)
		if err != nil {
			continue
		}
		if theirRank < myRank {
			return false, "a higher-ranked player already has a token on this plan's shield", nil
		}
	}
	return true, "", nil
}

// ── Difficulty computation ────────────────────────────────────────────────────

// computeDifficultyPure returns the base difficulty for a plan given the relevant rank.
// For Exchange Courtiers: relevantRank is the target player's rank on the power track.
// For Make Introductions: relevantRank is ignored; difficulty depends on peer_count.
// For Spread Propaganda: relevantRank is the preparer's rank on the esteem track.
func computeDifficultyPure(planType model.PlanType, resData planResData, relevantRank int16) (int16, error) {
	//nolint:exhaustive // Only Phase 2 plans are supported; future phases will extend this.
	switch planType {
	case model.PlanExchangeCourtiers:
		// Difficulty = target player's status on the power track.
		// Status is the inverse of rank: status = 6 - rank.
		targetStatus := max(int16(diceSides)-relevantRank, 1)
		return targetStatus, nil

	case model.PlanMakeIntroductions:
		// Difficulty = 2 + peer_count (1–4 peers → difficulty 3–6).
		const baseDifficulty = int16(2)
		pc := max(resData.PeerCount, 1)
		return baseDifficulty + pc, nil

	case model.PlanSpreadPropaganda:
		// Difficulty = preparer's rank on the esteem track.
		return relevantRank, nil

	default:
		return 0, fmt.Errorf("unsupported plan type: %s", planType)
	}
}

// computeDifficulty returns the base difficulty for a plan.
func computeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	resData planResData,
) (int16, error) {
	//nolint:exhaustive // Only Phase 2 plans are supported; future phases will extend this.
	switch plan.PlanType {
	case model.PlanExchangeCourtiers:
		// Difficulty = target player's status on the power track.
		// Status is the inverse of rank: status = 6 - rank.
		if plan.TargetPlayerID == nil {
			return 0, errors.New("exchange courtiers plan has no target player")
		}
		targetRank, err := playerRankInCategory(ctx, q, plan.GameID, *plan.TargetPlayerID, model.CategoryPower)
		if err != nil {
			return 0, fmt.Errorf("could not determine target player ranking: %w", err)
		}
		return computeDifficultyPure(plan.PlanType, resData, targetRank)

	case model.PlanMakeIntroductions:
		// Difficulty = 2 + peer_count (1–4 peers → difficulty 3–6).
		return computeDifficultyPure(plan.PlanType, resData, 0)

	case model.PlanSpreadPropaganda:
		// Difficulty = preparer's rank on the esteem track.
		preparerRank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryEsteem)
		if err != nil {
			return 0, fmt.Errorf("could not determine preparer ranking: %w", err)
		}
		return computeDifficultyPure(plan.PlanType, resData, preparerRank)

	default:
		return 0, fmt.Errorf("unsupported plan type: %s", plan.PlanType)
	}
}

// ── createPlanRoll creates a dice roll linked to a plan ───────────────────────

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
	// Two base dice for the actor.
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

// ── ListPlans ─────────────────────────────────────────────────────────────────

// ListPlans handles GET /api/tables/:id/plans.
func ListPlans(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r)
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
// Returns which Phase 2 plan types the current player can prepare, and
// the computed target row for each eligible plan. Ineligible plans include
// a human-readable reason.
func PlanEligibility(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r)
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

		// A player with no peers cannot prepare any plans (they have no characters
		// to act through and cannot be the focus player in future rows).
		hasPeers, err := playerHasPeers(ctx, q, gameID, player.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not check peer assets")
			return
		}
		if !hasPeers {
			for planType, meta := range phase2PlanMeta {
				ineligible = append(ineligible, ineligibleEntry{
					PlanType: planType,
					Category: meta.category,
					Reason:   "you have no peers — a player without peers cannot prepare plans",
				})
			}
			respond(w, http.StatusOK, map[string]any{
				"eligible":   eligible,
				"ineligible": ineligible,
			})
			return
		}

		for planType, meta := range phase2PlanMeta {
			targetRow := game.CurrentRow + meta.delay
			if targetRow > publicRecordRowCount {
				ineligible = append(ineligible, ineligibleEntry{
					PlanType: planType,
					Category: meta.category,
					Reason:   "no room on the public record (would exceed row 13)",
				})
				continue
			}
			ok, reason, err := checkPlanEligible(ctx, q, gameID, player.ID, planType, meta.category)
			if err != nil {
				ineligible = append(ineligible, ineligibleEntry{
					PlanType: planType,
					Category: meta.category,
					Reason:   "could not check eligibility",
				})
				continue
			}
			if ok {
				eligible = append(eligible, eligibleEntry{
					PlanType:  planType,
					Category:  meta.category,
					Delay:     meta.delay,
					TargetRow: targetRow,
				})
			} else {
				ineligible = append(ineligible, ineligibleEntry{
					PlanType: planType,
					Category: meta.category,
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

// validateExchangeCourtiersPlan checks that the target player and asset are valid
// for an Exchange Courtiers plan. Returns a non-nil error message if validation fails.
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

	// Verify the asset belongs to the target player and is a peer.
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

	// Target player must also have at least one peer (main characters count as peers).
	targetHasPeers, err := playerHasPeers(ctx, q, gameID, *targetPlayerID)
	if err != nil {
		return "could not check target peer assets"
	}
	if !targetHasPeers {
		return "target player has no peers"
	}

	return ""
}

// preparePlanValidation holds results from validating a plan preparation request.
type preparePlanValidation struct {
	Status      int
	ErrMsg      string
	TargetRow   int16
	Meta        planMetadata
	PlanTypeKey model.PlanType
}

// validatePlanPreparation performs all checks for plan preparation, returning early
// if any validation fails.
func validatePlanPreparation(
	ctx context.Context,
	q *dbgen.Queries,
	game *dbgen.Game,
	player *dbgen.Player,
	planType model.PlanType,
	targetPlayerID *int64,
	targetAssetID *int64,
	peerCount int16,
) preparePlanValidation {
	// Check game phase.
	if game.Phase != model.PhaseMainEvent {
		return preparePlanValidation{
			Status: http.StatusConflict,
			ErrMsg: "game is not in the main event phase",
		}
	}

	// Check plan type is supported.
	meta, supported := phase2PlanMeta[planType]
	if !supported {
		return preparePlanValidation{
			Status: http.StatusBadRequest,
			ErrMsg: "unsupported plan type for Phase 2",
		}
	}

	// Check eligibility.
	eligible, reason, err := checkPlanEligible(ctx, q, game.ID, player.ID, planType, meta.category)
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

	// Check target row bounds.
	targetRow := game.CurrentRow + meta.delay
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

	// Plan type specific validations.
	if planType == model.PlanExchangeCourtiers {
		if errMsg := validateExchangeCourtiersPlan(ctx, q, game.ID, targetPlayerID, targetAssetID); errMsg != "" {
			return preparePlanValidation{
				Status: http.StatusBadRequest,
				ErrMsg: errMsg,
			}
		}
	}

	if planType == model.PlanMakeIntroductions {
		if peerCount < 1 || peerCount > 4 {
			return preparePlanValidation{
				Status: http.StatusBadRequest,
				ErrMsg: "make_introductions requires peer_count between 1 and 4",
			}
		}
	}

	return preparePlanValidation{
		Status:    http.StatusOK,
		TargetRow: targetRow,
		Meta:      meta,
	}
}

// ── PreparePlan ───────────────────────────────────────────────────────────────

// PreparePlan handles POST /api/tables/:id/prepare-plan.
//
// Request body:
//
//	{
//	  "plan_type": "exchange_courtiers"|"make_introductions"|"spread_propaganda",
//	  "target_player_id": 123,   // Exchange Courtiers: target peer owner
//	  "target_asset_id":  456,   // Exchange Courtiers: the peer being courted
//	  "peer_count":       2,     // Make Introductions: number of peers (1–4)
//	  "preparation_notes": "..." // optional flavor text
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
			PeerCount        int16          `json:"peer_count"`
			PreparationNotes *string        `json:"preparation_notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()

		// Perform comprehensive validation of the plan preparation request.
		validation := validatePlanPreparation(
			ctx, q, game, player,
			body.PlanType,
			body.TargetPlayerID,
			body.TargetAssetID,
			body.PeerCount,
		)
		if validation.Status != http.StatusOK {
			respondErr(w, validation.Status, validation.ErrMsg)
			return
		}

		meta := validation.Meta
		targetRow := validation.TargetRow

		// Count existing plans on the target row to assign row_order.
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
			Category:         meta.category,
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
			d := planResData{PeerCount: body.PeerCount}
			if err := saveResData(ctx, q, plan.ID, d); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not save plan data")
				return
			}
		}

		// Place the preparer's token on the plan shield.
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

		respond(w, http.StatusCreated, map[string]any{"plan": plan})
	}
}

// ── GetPlan ───────────────────────────────────────────────────────────────────

// GetPlan handles GET /api/plans/:planId.
//
// Returns the plan, its computed difficulty, and (for EC) fair trade state.
func GetPlan(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, _, ok := requirePlanAccess(w, r, q)
		if !ok {
			return
		}

		resData := loadResData(plan.ResolutionData)
		difficulty, _ := computeDifficulty(r.Context(), q, plan, resData)

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
// Focus player begins resolution of a plan that is scheduled for the current
// row. Sets the plan to 'resolving' and broadcasts plan.resolving.
//
//   - Make Introductions and Spread Propaganda: dice roll created immediately.
//   - Exchange Courtiers: no dice roll yet (fair trade step first).
func ResolvePlan(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, plan, player, ok := requirePlanFocus(w, r, q)
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

		ctx := r.Context()

		if err := q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
			ID:     plan.ID,
			Status: model.PlanResolving,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not update plan status")
			return
		}

		h, hasHub := manager.Get(game.ID)
		if hasHub {
			h.BroadcastEvent(model.EventPlanResolving, model.PlanPayload{Plan: *plan})
		}

		// For MI and SP, create the dice roll immediately.
		var roll *dbgen.DiceRoll
		if plan.PlanType != model.PlanExchangeCourtiers {
			resData := loadResData(plan.ResolutionData)
			difficulty, err := computeDifficulty(ctx, q, plan, resData)
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not compute difficulty: "+err.Error())
				return
			}
			roll, err = createPlanRoll(ctx, q, manager, game, plan, difficulty, player.ID)
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not create dice roll")
				return
			}
		}

		resp := map[string]any{"plan_id": plan.ID}
		if roll != nil {
			resp["roll"] = roll
		}
		respond(w, http.StatusOK, resp)
	}
}

// ── FairTrade ─────────────────────────────────────────────────────────────────

// FairTrade handles POST /api/plans/:planId/fair-trade.
//
// Exchange Courtiers only. Three sub-actions in the body:
//
//	{"action": "offer",   "offered_asset_id": 123} — target player offers a peer
//	{"action": "accept"}                           — preparer accepts the trade
//	{"action": "decline"}                          — preparer declines; dice roll created
//
// On accept: targeted asset and offered asset are exchanged; plan resolves.
// On decline: dice roll is created and the normal roll flow proceeds.
func FairTrade(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanExchangeCourtiers {
			respondErr(w, http.StatusBadRequest, "fair trade is only for Exchange Courtiers")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		var body struct {
			Action         string `json:"action"`
			OfferedAssetID *int64 `json:"offered_asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()
		game, err := q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load game")
			return
		}

		resData := loadResData(plan.ResolutionData)

		switch body.Action {
		case "offer":
			// Target player names a peer as fair trade.
			if plan.TargetPlayerID == nil || player.ID != *plan.TargetPlayerID {
				respondErr(w, http.StatusForbidden, "only the target player can offer a fair trade")
				return
			}
			if body.OfferedAssetID == nil {
				respondErr(w, http.StatusBadRequest, "offered_asset_id is required")
				return
			}
			// Validate: must be a peer owned by the target player.
			asset, err := q.GetAssetByID(ctx, *body.OfferedAssetID)
			if err != nil {
				respondErr(w, http.StatusNotFound, "offered asset not found")
				return
			}
			if asset.OwnerID != player.ID {
				respondErr(w, http.StatusForbidden, "you can only offer your own assets")
				return
			}
			if asset.AssetType != model.AssetPeer {
				respondErr(w, http.StatusBadRequest, "fair trade offer must be a peer asset")
				return
			}
			resData.FairTradeAssetID = body.OfferedAssetID
			if err := saveResData(ctx, q, plan.ID, resData); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not save offer")
				return
			}
			respond(w, http.StatusOK, map[string]any{
				"plan_id":          plan.ID,
				"offered_asset_id": body.OfferedAssetID,
			})

		case "accept":
			// Preparer accepts: exchange assets, resolve plan.
			if player.ID != plan.PreparerID {
				respondErr(w, http.StatusForbidden, "only the preparer can accept or decline")
				return
			}
			if resData.FairTradeAssetID == nil {
				respondErr(w, http.StatusConflict, "no fair trade offer has been made yet")
				return
			}
			if plan.TargetAssetID == nil || plan.TargetPlayerID == nil {
				respondErr(w, http.StatusConflict, "plan is missing target asset or player")
				return
			}

			// Transfer targeted asset to preparer.
			if err := q.TransferAsset(ctx, dbgen.TransferAssetParams{
				ID:      *plan.TargetAssetID,
				OwnerID: plan.PreparerID,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not transfer targeted asset")
				return
			}
			// Transfer offered asset to target player.
			if err := q.TransferAsset(ctx, dbgen.TransferAssetParams{
				ID:      *resData.FairTradeAssetID,
				OwnerID: *plan.TargetPlayerID,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not transfer offered asset")
				return
			}

			accepted := true
			resData.FairTradeAccepted = &accepted
			resData.Choices = []string{"fair_trade_accepted"}
			if err := saveResData(ctx, q, plan.ID, resData); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not save decision")
				return
			}

			// Resolve plan.
			if err := q.SetPlanResult(ctx, dbgen.SetPlanResultParams{
				ID:     plan.ID,
				Result: new(makeOutcome),
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not resolve plan")
				return
			}

			h, hasHub := manager.Get(game.ID)
			if hasHub {
				// Broadcast asset transfers.
				ta, _ := q.GetAssetByID(ctx, *plan.TargetAssetID)
				h.BroadcastEvent(model.EventAssetTaken, model.AssetTakenPayload{
					Asset:      ta,
					OldOwnerID: *plan.TargetPlayerID,
					NewOwnerID: plan.PreparerID,
				})
				oa, _ := q.GetAssetByID(ctx, *resData.FairTradeAssetID)
				h.BroadcastEvent(model.EventAssetTaken, model.AssetTakenPayload{
					Asset:      oa,
					OldOwnerID: plan.PreparerID,
					NewOwnerID: *plan.TargetPlayerID,
				})
				h.BroadcastEvent(model.EventPlanResolved, model.PlanResolvedPayload{
					PlanID: plan.ID,
					Result: makeOutcome,
				})
			}

			respond(w, http.StatusOK, map[string]any{
				"plan_id": plan.ID,
				"result":  "make",
				"note":    "fair trade accepted; assets exchanged",
			})

		case "decline":
			// Preparer declines: create dice roll.
			if player.ID != plan.PreparerID {
				respondErr(w, http.StatusForbidden, "only the preparer can accept or decline")
				return
			}
			declined := false
			resData.FairTradeAccepted = &declined
			if err := saveResData(ctx, q, plan.ID, resData); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not save decision")
				return
			}

			difficulty, err := computeDifficulty(ctx, q, plan, resData)
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not compute difficulty")
				return
			}
			roll, err := createPlanRoll(ctx, q, manager, &game, plan, difficulty, player.ID)
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not create dice roll")
				return
			}
			respond(w, http.StatusOK, map[string]any{
				"plan_id": plan.ID,
				"roll":    roll,
			})

		default:
			respondErr(w, http.StatusBadRequest, "action must be 'offer', 'accept', or 'decline'")
		}
	}
}

// ── MakeChoice ────────────────────────────────────────────────────────────────

// MakeChoice handles POST /api/plans/:planId/make-choice.
//
// Called after the dice roll resolves (or after fair trade is declined then
// rolled). Records the make/mar option choices and executes any server-side
// mechanical effects.
//
// Request body:
//
//	{
//	  "choices": ["legal"],  // option key strings (plan-specific)
//	  "result": "make"       // "make" or "mar" — must match the roll outcome
//	}
//
// For EC make: server automatically transfers the target asset to the preparer.
// For EC mar option "riposte" or "forfeit": preparer's peer transfer handled
// by the caller using existing asset endpoints.
// For MI and SP: choices are recorded narratively; players use existing
// endpoints (create asset, add marginalia, etc.) for mechanical effects.

// applyExchangeCourtiersMechanic handles the asset transfer and messy flag
// for a successful Exchange Courtiers make result.
func applyExchangeCourtiersMechanic(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	game *dbgen.Game,
	plan *dbgen.Plan,
	choices []string,
	resData *planResData,
) error {
	if plan.TargetAssetID == nil || plan.TargetPlayerID == nil {
		return nil // nothing to do
	}

	// Only transfer if not already done via fair trade accept.
	if resData.FairTradeAccepted == nil || !*resData.FairTradeAccepted {
		if err := q.TransferAsset(ctx, dbgen.TransferAssetParams{
			ID:      *plan.TargetAssetID,
			OwnerID: plan.PreparerID,
		}); err != nil {
			return err
		}
		ta, _ := q.GetAssetByID(ctx, *plan.TargetAssetID)
		if h, ok := manager.Get(game.ID); ok {
			h.BroadcastEvent(model.EventAssetTaken, model.AssetTakenPayload{
				Asset:      ta,
				OldOwnerID: *plan.TargetPlayerID,
				NewOwnerID: plan.PreparerID,
			})
		}
	}

	// "Messy" requires the target to break a marginalia on any of their assets.
	if slices.Contains(choices, "messy") {
		resData.MessyBreakRequired = true
	}

	return nil
}

func MakeChoice(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, plan, player, ok := requirePlanFocus(w, r, q)
		if !ok {
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

		ctx := r.Context()

		// Verify result matches the linked dice roll's outcome (if one exists).
		roll, rollErr := q.GetDiceRollByPlanID(ctx, &plan.ID)
		if rollErr == nil && roll.Outcome != nil && *roll.Outcome != body.Result {
			respondErr(w, http.StatusConflict,
				fmt.Sprintf("result '%s' does not match roll outcome '%s'", body.Result, *roll.Outcome))
			return
		}

		resData := loadResData(plan.ResolutionData)
		resData.Choices = body.Choices

		// Server-side mechanical effects for Exchange Courtiers make.
		if plan.PlanType == model.PlanExchangeCourtiers && body.Result == makeOutcome {
			if err := applyExchangeCourtiersMechanic(ctx, q, manager, game, plan, body.Choices, &resData); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not apply exchange courtiers mechanic")
				return
			}
		}

		if err := saveResData(ctx, q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save choices")
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":              plan.ID,
			"choices":              body.Choices,
			"result":               body.Result,
			"messy_break_required": resData.MessyBreakRequired,
		})
		_ = player // used via requirePlanFocus validation
	}
}

// ── CompletePlan ──────────────────────────────────────────────────────────────

// CompletePlan handles POST /api/plans/:planId/complete.
//
// Marks the plan as resolved. The result is taken from the linked dice roll's
// outcome; if no roll exists (e.g. Exchange Courtiers fair trade accepted), it
// reads the result from resolution_data.Choices.
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

		ctx := r.Context()

		// Block completion if the target still needs to break a marginalia (EC messy).
		resData := loadResData(plan.ResolutionData)
		if resData.MessyBreakRequired && !resData.MessyBreakDone {
			respondErr(w, http.StatusConflict,
				"cannot complete plan: target player must first break a marginalia (POST /plans/{planId}/messy-break)")
			return
		}

		// Determine result from roll outcome or existing plan result (fair trade).
		resultStr := ""
		roll, rollErr := q.GetDiceRollByPlanID(ctx, &plan.ID)
		if rollErr == nil && roll.Outcome != nil {
			resultStr = *roll.Outcome
		} else if plan.Result != nil {
			// Already set (e.g. fair trade accept path).
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

// ── MessyBreak ────────────────────────────────────────────────────────────────

// MessyBreak handles POST /api/plans/:planId/messy-break.
//
// Exchange Courtiers only. After a make result with the "messy" option, the
// target player must tear one marginalia from any asset in the game. This
// endpoint enforces that requirement before CompletePlan is allowed.
//
// Request body:
//
//	{"marginalia_id": 123}
//
// Only the target player of the plan may call this. The marginalia is torn
// and MessyBreakDone is recorded in resolution_data.
func MessyBreak(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanExchangeCourtiers {
			respondErr(w, http.StatusBadRequest, "messy break is only for Exchange Courtiers")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if plan.TargetPlayerID == nil || player.ID != *plan.TargetPlayerID {
			respondErr(w, http.StatusForbidden, "only the target player can perform the messy break")
			return
		}

		resData := loadResData(plan.ResolutionData)
		if !resData.MessyBreakRequired {
			respondErr(w, http.StatusConflict, "no messy break is required for this plan")
			return
		}
		if resData.MessyBreakDone {
			respondErr(w, http.StatusConflict, "messy break has already been completed")
			return
		}

		var body struct {
			MarginaliaID int64 `json:"marginalia_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.MarginaliaID == 0 {
			respondErr(w, http.StatusBadRequest, "marginalia_id is required")
			return
		}

		ctx := r.Context()

		// Load the marginalia by ID (requires the new GetMarginaliaByID query).
		m, err := q.GetMarginaliaByID(ctx, body.MarginaliaID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "marginalia not found")
			return
		}
		if m.IsTorn {
			respondErr(w, http.StatusConflict, "marginalia is already torn")
			return
		}

		// Verify the marginalia's parent asset is in the same game.
		asset, err := q.GetAssetByID(ctx, m.AssetID)
		if err != nil || asset.GameID != plan.GameID {
			respondErr(w, http.StatusBadRequest, "marginalia does not belong to this game")
			return
		}

		// Tear the marginalia.
		if err := q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
			ID:       m.ID,
			TornByID: &player.ID,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not tear marginalia")
			return
		}

		// Mark the messy break complete in resolution_data.
		resData.MessyBreakDone = true
		if err := saveResData(ctx, q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not record messy break")
			return
		}

		if h, ok := manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventMarginaliaTorn, model.MarginaliaTornPayload{
				AssetID:  asset.ID,
				Position: m.Position,
				TornByID: player.ID,
			})
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"marginalia_id": m.ID,
			"asset_id":      asset.ID,
		})
	}
}
