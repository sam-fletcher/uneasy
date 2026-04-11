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
//  6. Focus player calls complete to mark the plan resolved.
//
// Ranking update (engrailed lines) is also implemented here:
//   runRankingUpdate(ctx, q, gameID) is called by advanceRowInner in turn.go.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
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

// computeDifficulty returns the base difficulty for a plan.
func computeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	resData planResData,
) (int16, error) {
	switch plan.PlanType {
	case model.PlanExchangeCourtiers:
		// Difficulty = 6 − target player's rank on the power track.
		if plan.TargetPlayerID == nil {
			return 0, errors.New("exchange courtiers plan has no target player")
		}
		targetRank, err := playerRankInCategory(ctx, q, plan.GameID, *plan.TargetPlayerID, model.CategoryPower)
		if err != nil {
			return 0, fmt.Errorf("could not determine target player ranking: %w", err)
		}
		d := max(int16(6)-targetRank, 1)
		return d, nil

	case model.PlanMakeIntroductions:
		// Difficulty = 2 + peer_count (1–4 peers → difficulty 3–6).
		pc := max(resData.PeerCount, 1)
		return 2 + pc, nil

	case model.PlanSpreadPropaganda:
		// Difficulty = preparer's rank on the esteem track.
		preparerRank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryEsteem)
		if err != nil {
			return 0, fmt.Errorf("could not determine preparer ranking: %w", err)
		}
		return preparerRank, nil

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

		meta, supported := phase2PlanMeta[body.PlanType]
		if !supported {
			respondErr(w, http.StatusBadRequest, "unsupported plan type for Phase 2")
			return
		}

		ctx := r.Context()

		// Eligibility check.
		eligible, reason, err := checkPlanEligible(ctx, q, game.ID, player.ID, body.PlanType, meta.category)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not check eligibility")
			return
		}
		if !eligible {
			respondErr(w, http.StatusForbidden, reason)
			return
		}

		// Target row bounds check.
		targetRow := game.CurrentRow + meta.delay
		if targetRow > publicRecordRowCount {
			respondErr(w, http.StatusConflict, "plan would be placed past row 13")
			return
		}

		// Exchange Courtiers: target player and asset required.
		if body.PlanType == model.PlanExchangeCourtiers {
			if body.TargetPlayerID == nil || body.TargetAssetID == nil {
				respondErr(w, http.StatusBadRequest, "exchange_courtiers requires target_player_id and target_asset_id")
				return
			}
			// Verify the asset belongs to the target player and is a peer.
			asset, err := q.GetAssetByID(ctx, *body.TargetAssetID)
			if err != nil {
				respondErr(w, http.StatusNotFound, "target asset not found")
				return
			}
			if asset.OwnerID != *body.TargetPlayerID {
				respondErr(w, http.StatusBadRequest, "target asset does not belong to target player")
				return
			}
			if asset.AssetType != model.AssetPeer {
				respondErr(w, http.StatusBadRequest, "exchange_courtiers target must be a peer asset")
				return
			}
		}

		// Make Introductions: peer count required (1–4).
		if body.PlanType == model.PlanMakeIntroductions {
			if body.PeerCount < 1 || body.PeerCount > 4 {
				respondErr(w, http.StatusBadRequest, "make_introductions requires peer_count between 1 and 4")
				return
			}
		}

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
			resultStr := "make"
			if err := q.SetPlanResult(ctx, dbgen.SetPlanResultParams{
				ID:     plan.ID,
				Result: &resultStr,
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
					Result: resultStr,
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
		if body.Result != "make" && body.Result != "mar" {
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
		if plan.PlanType == model.PlanExchangeCourtiers && body.Result == "make" {
			if plan.TargetAssetID != nil && plan.TargetPlayerID != nil {
				// Only transfer if not already done via fair trade accept.
				if resData.FairTradeAccepted == nil || !*resData.FairTradeAccepted {
					if err := q.TransferAsset(ctx, dbgen.TransferAssetParams{
						ID:      *plan.TargetAssetID,
						OwnerID: plan.PreparerID,
					}); err != nil {
						respondErr(w, http.StatusInternalServerError, "could not transfer asset")
						return
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
			}
		}

		if err := saveResData(ctx, q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save choices")
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID,
			"choices": body.Choices,
			"result":  body.Result,
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

// ── Ranking update algorithm ──────────────────────────────────────────────────

// runRankingUpdate executes the engrailed-line ranking update algorithm and
// returns the updated rankings. Called from advanceRowInner (turn.go) whenever
// current_row crosses an engrailed line (after rows 4, 8, 12).
//
// Algorithm (per RULES_SUMMARY.md §"Updating Rankings"):
//
//  1. For each category (power, knowledge, esteem):
//     a. Gather all player tokens on plans in that category.
//     b. Process tokens from worst rank to best rank (highest rank# to lowest).
//     c. For each token player, swap them with whoever occupies the slot
//     one rank above. In static dummy mode, cannot swap past a dummy.
//  2. After processing all tokens for a category: if every plan type in that
//     category has at least one token, clear all tokens for that category.
//  3. Upsert all modified ranking slots.
func runRankingUpdate(ctx context.Context, q *dbgen.Queries, gameID int64) ([]dbgen.Ranking, error) {
	rankings, err := q.ListRankingsByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	tokens, err := q.ListPlanTokensByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}

	// Represent each category as a mutable [5]int64 array (0-indexed = rank-1).
	// -1 is the sentinel for "dummy token" (player_id IS NULL in DB).
	const dummySentinel = int64(-1)
	type catSlots [5]int64

	slots := map[model.RankingCategory]*catSlots{
		model.CategoryPower:     new(catSlots),
		model.CategoryKnowledge: new(catSlots),
		model.CategoryEsteem:    new(catSlots),
	}
	// Initialize all as dummy by default; fill with real player IDs from DB.
	for _, s := range slots {
		for i := range s {
			s[i] = dummySentinel
		}
	}
	for _, rk := range rankings {
		if rk.Rank < 1 || rk.Rank > 5 {
			continue
		}
		s := slots[rk.Category]
		if rk.PlayerID == nil {
			s[rk.Rank-1] = dummySentinel
		} else {
			s[rk.Rank-1] = *rk.PlayerID
		}
	}

	// Reverse map: player ID → rank per category (for sorting tokens).
	playerRank := make(map[int64]map[model.RankingCategory]int16)
	for _, rk := range rankings {
		if rk.PlayerID == nil {
			continue
		}
		if _, ok := playerRank[*rk.PlayerID]; !ok {
			playerRank[*rk.PlayerID] = make(map[model.RankingCategory]int16)
		}
		playerRank[*rk.PlayerID][rk.Category] = rk.Rank
	}

	// Phase 2: one plan type per category.
	categoryPlanTypes := map[model.RankingCategory][]model.PlanType{
		model.CategoryPower:     {model.PlanExchangeCourtiers},
		model.CategoryKnowledge: {model.PlanMakeIntroductions},
		model.CategoryEsteem:    {model.PlanSpreadPropaganda},
	}

	for cat, planTypes := range categoryPlanTypes {
		s := slots[cat]

		// Gather unique token players for this category.
		seen := make(map[int64]struct{})
		var tokenPlayers []int64
		for _, pt := range planTypes {
			for _, tok := range tokens {
				if tok.PlanType == pt {
					if _, dup := seen[tok.PlayerID]; !dup {
						seen[tok.PlayerID] = struct{}{}
						tokenPlayers = append(tokenPlayers, tok.PlayerID)
					}
				}
			}
		}
		if len(tokenPlayers) == 0 {
			continue
		}

		// Sort worst → best (highest rank# first = process upward swaps correctly).
		sort.Slice(tokenPlayers, func(i, j int) bool {
			ri := playerRank[tokenPlayers[i]][cat]
			rj := playerRank[tokenPlayers[j]][cat]
			return ri > rj
		})

		for _, pid := range tokenPlayers {
			rankMap, ok := playerRank[pid]
			if !ok {
				continue
			}
			myRank := rankMap[cat] // 1-indexed
			if myRank <= 1 {
				continue // already at top
			}
			aboveIdx := myRank - 2 // 0-indexed slot one rank above
			myIdx := myRank - 1    // 0-indexed current slot

			above := s[aboveIdx]
			if above == dummySentinel {
				continue // static mode: can't swap past a dummy
			}

			// Swap.
			s[aboveIdx] = pid
			s[myIdx] = above

			// Update in-memory rank map so subsequent swaps are aware.
			playerRank[pid][cat] = int16(aboveIdx + 1)
			if _, ok := playerRank[above]; ok {
				playerRank[above][cat] = int16(myIdx + 1)
			}
		}

		// Clear tokens if every plan type in this category has at least one token.
		allHaveTokens := true
		for _, pt := range planTypes {
			found := false
			for _, tok := range tokens {
				if tok.PlanType == pt {
					found = true
					break
				}
			}
			if !found {
				allHaveTokens = false
				break
			}
		}
		if allHaveTokens {
			if err := q.DeletePlanTokensByCategory(ctx, dbgen.DeletePlanTokensByCategoryParams{
				GameID:   gameID,
				Category: cat,
			}); err != nil {
				return nil, err
			}
		}
	}

	// Write all modified slots back. UpsertRanking handles the ON CONFLICT update.
	for cat, s := range slots {
		for i, pid := range s {
			rank := int16(i + 1)
			var playerIDPtr *int64
			if pid != dummySentinel {
				p := pid
				playerIDPtr = &p
			}
			if err := q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
				GameID:   gameID,
				PlayerID: playerIDPtr,
				Category: cat,
				Rank:     rank,
			}); err != nil {
				return nil, err
			}
		}
	}

	return q.ListRankingsByGame(ctx, gameID)
}
