package handler

// handler/plan_propose_duel.go — Propose Duel plan handler (Phase 3d).
//
// Propose Duel (esteem, delay 5). The preparer challenges another player to
// a duel of arms or wits. Both sides stake peer assets with hidden dice;
// bouts compare dice until one side runs out of stakes; accumulated winning
// dice feed into the plan's standard dice roll.
//
// Phases (stored in ResolutionData.DuelPhase):
//
//	"setup"        Champions can be elected; both players submit stake counts.
//	               Advances to "staking" when both stake counts are in.
//	"staking"      Each player submits their specific staked asset IDs; server
//	               rolls and stores a hidden d6 under each. Advances to "bouts"
//	               once both players have staked their nominated counts.
//	"bouts"        Declarer/responder bout loop. Ends when one side is out of
//	               unresolved stakes; server creates the standard dice roll
//	               with accumulated dice pre-assigned and advances to "roll".
//	"roll"         Normal dice-roll flow (leverage window, close leverage).
//	               make-choice applies asset transfers and leverages all stakes.
//	"done"         Final state after complete.
//
// Extra routes:
//
//	POST /api/plans/:planId/elect-champion   Elect a peer as champion (narrative).
//	POST /api/plans/:planId/stake-reveal     Submit stake count (simultaneous).
//	POST /api/plans/:planId/select-stakes    Submit specific staked assets.
//	POST /api/plans/:planId/bout-declare     Declarer picks stake + high/low.
//	POST /api/plans/:planId/bout-respond     Responder picks stake; bout resolves.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

const (
	duelPhaseSetup   = gamepkg.DuelPhaseSetup
	duelPhaseStaking = gamepkg.DuelPhaseStaking
	duelPhaseBouts   = gamepkg.DuelPhaseBouts
	duelPhaseRoll    = gamepkg.DuelPhaseRoll
	duelPhaseDone    = gamepkg.DuelPhaseDone
)

func init() {
	RegisterPlan(model.PlanProposeDuel, pduelHandler{})
}

type pduelHandler struct{}

func (pduelHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryEsteem, Delay: 5}
}

func (pduelHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (*int16, string) {
	if v.TargetPlayerID == nil {
		return nil, "propose_duel requires target_player_id (the challenged player)"
	}
	if v.Player != nil && *v.TargetPlayerID == v.Player.ID {
		return nil, "you cannot duel yourself"
	}
	if v.Notes == "" {
		return nil, "propose_duel requires preparation_notes (location and type of duel)"
	}
	return nil, ""
}

func (pduelHandler) ComputeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	if plan.TargetPlayerID == nil {
		return 0, errors.New("propose_duel plan has no target player")
	}
	rank, err := playerRankInCategory(ctx, q, plan.GameID, *plan.TargetPlayerID, model.CategoryEsteem)
	if err != nil {
		return 0, fmt.Errorf("could not determine target esteem rank: %w", err)
	}
	return gamepkg.ProposeDuelDifficulty(rank), nil
}

// OnResolve sets the initial phase and gives initiative to the player with
// the best (lowest rank = highest status) esteem standing.
func (pduelHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	if plan.TargetPlayerID == nil {
		return nil, errors.New("propose_duel plan has no target player")
	}
	resData := loadResolutionData(plan.ResolutionData)
	state := resData.EnsureDuel()
	state.Phase = duelPhaseSetup

	prepRank, err := playerRankInCategory(ctx, deps.Q, plan.GameID, plan.PreparerID, model.CategoryEsteem)
	if err != nil {
		return nil, fmt.Errorf("preparer esteem rank: %w", err)
	}
	targetRank, err := playerRankInCategory(ctx, deps.Q, plan.GameID, *plan.TargetPlayerID, model.CategoryEsteem)
	if err != nil {
		return nil, fmt.Errorf("target esteem rank: %w", err)
	}
	// Lower rank number = higher esteem status = initiative.
	initiative := plan.PreparerID
	if targetRank < prepRank {
		initiative = *plan.TargetPlayerID
	}
	state.InitiativePlayerID = &initiative

	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return nil, fmt.Errorf("save duel setup: %w", err)
	}
	return nil, nil
}

// MaxChoices caps how many of the loser's staked assets the winner may claim:
// "On make: take a number of opponent's staked assets equal to your result. On
// mar: opponent takes staked assets equal to the difficulty." The natural cap
// (you can only pick assets that were actually staked) is enforced separately in
// ApplyChoice.
func (pduelHandler) MaxChoices(result string, rollResult, difficulty int16) int {
	if result == makeOutcome {
		return int(rollResult)
	}
	return int(difficulty)
}

// ApplyChoice transfers assets and leverages all staked assets after the
// standard dice roll resolves. choices is the list of asset IDs the winner
// chose to take (as stringified int64s); each must be one of the loser's own
// staked assets.
func (pduelHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	choices []string,
	result string,
) error {
	stakes, err := deps.Q.ListDuelStakesByPlan(ctx, plan.ID)
	if err != nil {
		return fmt.Errorf("list stakes: %w", err)
	}
	// Leverage every staked asset (both sides, per spec).
	for _, s := range stakes {
		_ = deps.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
			ID:          s.AssetID,
			IsLeveraged: true,
		})
		broadcastEvent(deps.Manager, plan.GameID, model.EventAssetLeveraged, model.AssetIDPayload{
			AssetID: s.AssetID, PlayerID: s.PlayerID,
		})
	}

	// Determine winner/loser: make → preparer wins (takes from target);
	// mar → target wins (takes from preparer).
	var winnerID, loserID int64
	if result == makeOutcome {
		recipient, err := gamepkg.AssetRecipientForPlan(ctx, deps.Q, plan)
		if err != nil {
			return fmt.Errorf("resolve asset recipient: %w", err)
		}
		winnerID = recipient
		if plan.TargetPlayerID != nil {
			loserID = *plan.TargetPlayerID
		}
	} else {
		if plan.TargetPlayerID == nil {
			return errors.New("propose_duel plan has no target player")
		}
		winnerID = *plan.TargetPlayerID
		loserID = plan.PreparerID
	}

	// Only the loser's own staked assets may be claimed (the rules say "take a
	// number of opponent's staked assets").
	loserStakeIDs := make(map[int64]bool)
	for _, s := range stakes {
		if s.PlayerID == loserID {
			loserStakeIDs[s.AssetID] = true
		}
	}

	// Transfer each requested asset. Each choice is a stake asset ID staked by
	// the loser.
	taken := 0
	for _, c := range choices {
		var assetID int64
		if _, err := fmt.Sscanf(c, "%d", &assetID); err != nil || assetID == 0 {
			continue
		}
		if !loserStakeIDs[assetID] {
			return fmt.Errorf("asset %d is not one of the losing side's staked assets", assetID)
		}
		asset, err := deps.Q.GetAssetByID(ctx, assetID)
		if err != nil {
			return fmt.Errorf("asset %d not found", assetID)
		}
		if err := deps.Q.TransferAsset(ctx, dbgen.TransferAssetParams{
			ID: assetID, OwnerID: winnerID,
		}); err != nil {
			return fmt.Errorf("transfer asset %d: %w", assetID, err)
		}
		broadcastEvent(deps.Manager, plan.GameID, model.EventAssetTaken, model.AssetTakenPayload{
			Asset: asset, OldOwnerID: loserID, NewOwnerID: winnerID,
		})
		taken++
	}

	winnerName := playerDisplayName(ctx, deps.Q, winnerID)
	loserName := playerDisplayName(ctx, deps.Q, loserID)
	assetWord := "assets"
	if taken == 1 {
		assetWord = "asset"
	}
	pduelLog(ctx, deps, plan, model.SeverityImportant, fmt.Sprintf(
		"%s won the duel and took %d staked %s from %s; all staked assets are leveraged.",
		winnerName, taken, assetWord, loserName))

	resData.EnsureDuel().Phase = duelPhaseDone
	return nil
}

// pduelLog writes a plan.propose_duel action-log post.
func pduelLog(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, severity int32, body string) {
	planID := plan.ID
	EmitSystemPost(ctx, deps.Q, deps.Manager, plan.GameID, "plan.propose_duel",
		severity, body, plan.RowNumber, &planID, nil,
		map[string]any{"plan_id": plan.ID})
}

func (pduelHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	if d := resData.Duel; d == nil || d.Phase != duelPhaseDone {
		return errors.New("duel has not completed: make-choice must be submitted after the roll resolves")
	}
	return nil
}

func (pduelHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"elect-champion": pduelElectChampionHandler(deps),
		"stake-reveal":   pduelStakeRevealHandler(deps),
		"select-stakes":  pduelSelectStakesHandler(deps),
		"bout-declare":   pduelBoutDeclareHandler(deps),
		"bout-respond":   pduelBoutRespondHandler(deps),
	}
}

// ── Get Duel State ────────────────────────────────────────────────────────────

// duelStakeView is the wire shape of a duel stake. HiddenDie is only populated
// when the caller is allowed to see it: either they own the stake, or the
// stake has been resolved (its die is already public via the bout record).
type duelStakeView struct {
	ID         int64  `json:"id"`
	PlanID     int64  `json:"plan_id"`
	PlayerID   int64  `json:"player_id"`
	AssetID    int64  `json:"asset_id"`
	IsResolved bool   `json:"is_resolved"`
	IsWinner   *bool  `json:"is_winner"`
	HiddenDie  *int16 `json:"hidden_die"`
}

// GetDuelState returns the full duel state a caller is allowed to see:
// all stakes (with hidden dice masked for the opponent's unresolved stakes)
// and the full bout history. GET /api/plans/:planId/duel-state.
func GetDuelState(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, s.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanProposeDuel {
			respondErr(w, http.StatusBadRequest, "plan is not a propose_duel")
			return
		}

		ctx := r.Context()
		stakes, err := s.Q.ListDuelStakesByPlan(ctx, plan.ID)
		if err != nil {
			respondInternalErr(w, r, "could not load stakes", err)
			return
		}
		bouts, err := s.Q.ListDuelBoutsByPlan(ctx, plan.ID)
		if err != nil {
			respondInternalErr(w, r, "could not load bouts", err)
			return
		}

		views := make([]duelStakeView, len(stakes))
		for i, s := range stakes {
			v := duelStakeView{
				ID:         s.ID,
				PlanID:     s.PlanID,
				PlayerID:   s.PlayerID,
				AssetID:    s.AssetID,
				IsResolved: s.IsResolved,
				IsWinner:   s.IsWinner,
			}
			// Only reveal the hidden die to its owner, or when the stake has
			// been resolved in a bout (at which point the die is public).
			if s.PlayerID == player.ID || s.IsResolved {
				face := s.HiddenDie
				v.HiddenDie = &face
			}
			views[i] = v
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID,
			"stakes":  views,
			"bouts":   bouts,
		})
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func pduelIsParticipant(plan *dbgen.Plan, playerID int64) bool {
	if playerID == plan.PreparerID {
		return true
	}
	if plan.TargetPlayerID != nil && *plan.TargetPlayerID == playerID {
		return true
	}
	return false
}

func pduelOpponentID(plan *dbgen.Plan, playerID int64) int64 {
	if playerID == plan.PreparerID && plan.TargetPlayerID != nil {
		return *plan.TargetPlayerID
	}
	return plan.PreparerID
}

func pduelSide(plan *dbgen.Plan, playerID int64) gamepkg.DuelSide {
	if playerID == plan.PreparerID {
		return gamepkg.DuelSidePreparer
	}
	return gamepkg.DuelSideTarget
}

// ── Elect Champion ────────────────────────────────────────────────────────────

// pduelElectChampionHandler: POST /api/plans/:planId/elect-champion
// Body: {"asset_id": N | null}. If asset_id is null or omitted, the player is
// signalling "I'll fight myself." If present, the asset must be a peer owned
// by the caller. The initiative-holder must declare first so the other side's
// UI knows when to unlock.
//
//nolint:gocognit // champion election with eligibility + auto-advance
func pduelElectChampionHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanProposeDuel)
		if !ok {
			return
		}
		if !pduelIsParticipant(plan, player.ID) {
			respondErr(w, http.StatusForbidden, "only duellists may elect a champion")
			return
		}

		var body struct {
			AssetID *int64 `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureDuel()

		if state.Phase != duelPhaseSetup {
			respondErr(w, http.StatusConflict, "champions can only be elected during setup")
			return
		}
		alreadyDeclared := state.PreparerChampionDeclared
		if player.ID != plan.PreparerID {
			alreadyDeclared = state.TargetChampionDeclared
		}
		if alreadyDeclared {
			respondErr(w, http.StatusConflict, "you have already declared your champion choice")
			return
		}
		// Initiative-holder declares first.
		if state.InitiativePlayerID != nil && *state.InitiativePlayerID != player.ID {
			initHas := state.PreparerChampionDeclared
			if *state.InitiativePlayerID != plan.PreparerID {
				initHas = state.TargetChampionDeclared
			}
			if !initHas {
				respondErr(w, http.StatusConflict,
					"the player with initiative must declare their champion choice first")
				return
			}
		}

		if body.AssetID != nil {
			asset, err := deps.Q.GetAssetByID(ctx, *body.AssetID)
			if err != nil {
				respondErr(w, http.StatusNotFound, "asset not found")
				return
			}
			if asset.GameID != plan.GameID || asset.OwnerID != player.ID {
				respondErr(w, http.StatusForbidden, "you do not own this asset")
				return
			}
			if asset.AssetType != model.AssetPeer {
				respondErr(w, http.StatusBadRequest, "champion must be a peer asset")
				return
			}
		}

		if player.ID == plan.PreparerID {
			state.PreparerChampionID = body.AssetID
			state.PreparerChampionDeclared = true
		} else {
			state.TargetChampionID = body.AssetID
			state.TargetChampionDeclared = true
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save champion", err)
			return
		}

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			var aid int64
			if body.AssetID != nil {
				aid = *body.AssetID
			}
			h.BroadcastEvent(model.EventDuelChampionElected, model.DuelChampionElectedPayload{
				PlanID: plan.ID, PlayerID: player.ID, AssetID: aid,
			})
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID, "player_id": player.ID, "asset_id": body.AssetID,
		})
	}
}

// ── Stake Reveal ──────────────────────────────────────────────────────────────

// pduelStakeRevealHandler: POST /api/plans/:planId/stake-reveal
// Body: {"count": N}. Min 1; max 1+esteem status.
// Counts are held until both players submit, then revealed.
func pduelStakeRevealHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanProposeDuel)
		if !ok {
			return
		}
		if !pduelIsParticipant(plan, player.ID) {
			respondErr(w, http.StatusForbidden, "only duellists may reveal stakes")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureDuel()
		if state.Phase != duelPhaseSetup {
			respondErr(w, http.StatusConflict, "stake reveal is only allowed in 'setup' phase")
			return
		}

		var body struct {
			Count int16 `json:"count"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Count < 1 {
			respondErr(w, http.StatusBadRequest, "count must be ≥ 1")
			return
		}

		ctx := r.Context()
		rank, err := playerRankInCategory(ctx, deps.Q, plan.GameID, player.ID, model.CategoryEsteem)
		if err != nil {
			respondInternalErr(w, r, "could not load esteem rank", err)
			return
		}
		if body.Count > gamepkg.MaxStakes(rank) {
			respondErr(w, http.StatusBadRequest,
				fmt.Sprintf("count %d exceeds maximum %d for your esteem status",
					body.Count, gamepkg.MaxStakes(rank)))
			return
		}

		// Accumulate per-player stake counts until both have submitted.
		if state.StakeCounts == nil {
			state.StakeCounts = map[int64]int16{}
		}
		state.StakeCounts[player.ID] = body.Count

		if len(state.StakeCounts) >= 2 {
			// Both submitted — reveal and advance to staking.
			state.PreparerStakeCount = state.StakeCounts[plan.PreparerID]
			if plan.TargetPlayerID != nil {
				state.TargetStakeCount = state.StakeCounts[*plan.TargetPlayerID]
			}
			state.Phase = duelPhaseStaking

			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondInternalErr(w, r, "could not save stake counts", err)
				return
			}

			broadcastEvent(deps.Manager, plan.GameID, model.EventDuelStakesRevealed, model.DuelStakesRevealedPayload{
				PlanID:             plan.ID,
				PreparerStakeCount: state.PreparerStakeCount,
				TargetStakeCount:   state.TargetStakeCount,
			})
		} else {
			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondInternalErr(w, r, "could not save stake reveal", err)
				return
			}
		}

		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "submitted": len(state.StakeCounts)})
	}
}

// ── Select Stakes ─────────────────────────────────────────────────────────────

// pduelSelectStakesHandler: POST /api/plans/:planId/select-stakes
// Body: {"asset_ids": [N, ...]}
// The count must match the player's revealed stake count. Server rolls a
// hidden d6 for each asset and stores it in duel_staked_assets. The hidden
// die is visible only to the asset owner; the opponent sees only that a
// stake has been placed.
//
//nolint:gocognit // stake-selection lifecycle including target-claim path
func pduelSelectStakesHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanProposeDuel)
		if !ok {
			return
		}
		if !pduelIsParticipant(plan, player.ID) {
			respondErr(w, http.StatusForbidden, "only duellists may stake assets")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureDuel()
		if state.Phase != duelPhaseStaking {
			respondErr(w, http.StatusConflict, "select-stakes is only allowed in 'staking' phase")
			return
		}

		var expected int16
		if player.ID == plan.PreparerID {
			expected = state.PreparerStakeCount
		} else {
			expected = state.TargetStakeCount
		}

		var body struct {
			AssetIDs []int64 `json:"asset_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if int16(len(body.AssetIDs)) != expected {
			respondErr(w, http.StatusBadRequest,
				fmt.Sprintf("expected %d asset_ids to match your stake count", expected))
			return
		}

		ctx := r.Context()

		// Check whether this player has already staked.
		existing, err := deps.Q.ListDuelStakesByPlanPlayer(ctx, dbgen.ListDuelStakesByPlanPlayerParams{
			PlanID: plan.ID, PlayerID: player.ID,
		})
		if err != nil {
			respondInternalErr(w, r, "could not load existing stakes", err)
			return
		}
		if len(existing) > 0 {
			respondErr(w, http.StatusConflict, "you have already selected your stakes")
			return
		}

		// Validate each asset: owned, non-destroyed, not already leveraged.
		for _, aid := range body.AssetIDs {
			asset, errAsset := deps.Q.GetAssetByID(ctx, aid)
			if errAsset != nil {
				respondErr(w, http.StatusNotFound, fmt.Sprintf("asset %d not found", aid))
				return
			}
			if asset.GameID != plan.GameID || asset.OwnerID != player.ID {
				respondErr(w, http.StatusForbidden, fmt.Sprintf("you do not own asset %d", aid))
				return
			}
			if asset.IsDestroyed {
				respondErr(w, http.StatusBadRequest, fmt.Sprintf("asset %d is destroyed", aid))
				return
			}
			if asset.IsLeveraged {
				respondErr(w, http.StatusBadRequest, fmt.Sprintf("asset %d is already leveraged", aid))
				return
			}
		}

		// Create stakes with a hidden d6 per asset. Collect them so the caller
		// can see their own hidden dice in the response without polling.
		createdStakes := make([]dbgen.DuelStakedAsset, 0, len(body.AssetIDs))
		for _, aid := range body.AssetIDs {
			face := int16(rand.IntN(gamepkg.DiceSides) + 1)
			stake, errStake := deps.Q.CreateDuelStake(ctx, dbgen.CreateDuelStakeParams{
				PlanID:    plan.ID,
				PlayerID:  player.ID,
				AssetID:   aid,
				HiddenDie: face,
			})
			if errStake != nil {
				respondInternalErr(w, r, "could not create stake", errStake)
				return
			}
			createdStakes = append(createdStakes, stake)
		}

		// If both players have staked, advance to bouts.
		allStakes, err := deps.Q.ListDuelStakesByPlan(ctx, plan.ID)
		if err != nil {
			respondInternalErr(w, r, "could not load stakes", err)
			return
		}
		total := int16(len(allStakes))
		if total == state.PreparerStakeCount+state.TargetStakeCount {
			state.Phase = duelPhaseBouts
			state.CurrentBout = 0
			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondInternalErr(w, r, "could not advance phase", err)
				return
			}
			// Mirror the stake-reveal broadcast: the waiting duellist needs a
			// duel event to refetch the plan and pick up phase=bouts. Without
			// it, broadcastRowState alone leaves them soft-locked on the
			// staking panel even though it's their turn to declare.
			broadcastEvent(deps.Manager, plan.GameID, model.EventDuelStakesSelected, model.DuelStakesSelectedPayload{
				PlanID: plan.ID,
			})
		}

		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID, "staked": len(body.AssetIDs), "stakes": createdStakes,
		})
	}
}

// ── Bout Declare ──────────────────────────────────────────────────────────────

// pduelBoutDeclareHandler: POST /api/plans/:planId/bout-declare
// Body: {"stake_id": N, "declaration": "high"|"low"}
// Only the player with initiative (the current declarer) may call this.
// Starts a new bout; the responder then completes it via bout-respond.
func pduelBoutDeclareHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanProposeDuel)
		if !ok {
			return
		}
		if !pduelIsParticipant(plan, player.ID) {
			respondErr(w, http.StatusForbidden, "only duellists may declare bouts")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureDuel()
		if state.Phase != duelPhaseBouts {
			respondErr(w, http.StatusConflict, "bout-declare is only allowed in 'bouts' phase")
			return
		}
		if state.InitiativePlayerID == nil || *state.InitiativePlayerID != player.ID {
			respondErr(w, http.StatusForbidden, "only the player with initiative may declare")
			return
		}

		// Ensure no unresolved bout exists.
		latest, latestErr := deps.Q.GetLatestDuelBout(r.Context(), plan.ID)
		if latestErr == nil && !latest.ResolvedAt.Valid {
			respondErr(w, http.StatusConflict, "a bout is already in progress — responder must respond first")
			return
		}

		var body struct {
			StakeID     int64  `json:"stake_id"`
			Declaration string `json:"declaration"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Declaration != string(gamepkg.DeclHigh) && body.Declaration != string(gamepkg.DeclLow) {
			respondErr(w, http.StatusBadRequest, "declaration must be 'high' or 'low'")
			return
		}

		ctx := r.Context()
		stake, err := deps.Q.GetDuelStake(ctx, body.StakeID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "stake not found")
			return
		}
		if stake.PlanID != plan.ID || stake.PlayerID != player.ID {
			respondErr(w, http.StatusForbidden, "that stake is not yours")
			return
		}
		if stake.IsResolved {
			respondErr(w, http.StatusConflict, "that stake has already been resolved")
			return
		}

		boutNumber := state.CurrentBout + 1
		_, err = deps.Q.CreateDuelBout(ctx, dbgen.CreateDuelBoutParams{
			PlanID:          plan.ID,
			BoutNumber:      boutNumber,
			DeclarerID:      player.ID,
			DeclarerStakeID: body.StakeID,
			ResponderID:     pduelOpponentID(plan, player.ID),
			Declaration:     &body.Declaration,
			DeclarerDie:     &stake.HiddenDie,
		})
		if err != nil {
			respondInternalErr(w, r, "could not create bout", err)
			return
		}

		state.CurrentBout = boutNumber
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save bout state", err)
			return
		}

		// The responder needs a duel event to refetch the bout list and render
		// the respond UI; broadcastRowState alone won't refresh their panel.
		broadcastEvent(deps.Manager, plan.GameID, model.EventDuelBoutDeclared, model.DuelBoutDeclaredPayload{
			PlanID:      plan.ID,
			BoutNumber:  boutNumber,
			ResponderID: pduelOpponentID(plan, player.ID),
		})
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{
			"plan_id":     plan.ID,
			"bout_number": boutNumber,
			"responder":   pduelOpponentID(plan, player.ID),
		})
	}
}

// ── Bout Respond ──────────────────────────────────────────────────────────────

// pduelBoutRespondHandler: POST /api/plans/:planId/bout-respond
// Body: {"stake_id": N}
// The responder picks their stake. Server compares dice (via game.ResolveBout),
// records the outcome, swaps initiative, and — if this was the final bout —
// creates the standard dice roll with accumulated dice pre-assigned.
//
//nolint:funlen,gocognit // bout state machine (response/auto-advance/result)
func pduelBoutRespondHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanProposeDuel)
		if !ok {
			return
		}
		if !pduelIsParticipant(plan, player.ID) {
			respondErr(w, http.StatusForbidden, "only duellists may respond")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureDuel()
		if state.Phase != duelPhaseBouts {
			respondErr(w, http.StatusConflict, "bout-respond is only allowed in 'bouts' phase")
			return
		}

		ctx := r.Context()
		latest, err := deps.Q.GetLatestDuelBout(ctx, plan.ID)
		if err != nil {
			respondErr(w, http.StatusConflict, "no bout in progress")
			return
		}
		if latest.ResolvedAt.Valid {
			respondErr(w, http.StatusConflict, "most recent bout is already resolved")
			return
		}
		if latest.ResponderID != player.ID {
			respondErr(w, http.StatusForbidden, "you are not the responder for this bout")
			return
		}

		var body struct {
			StakeID int64 `json:"stake_id"`
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		respStake, err := deps.Q.GetDuelStake(ctx, body.StakeID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "stake not found")
			return
		}
		if respStake.PlanID != plan.ID || respStake.PlayerID != player.ID {
			respondErr(w, http.StatusForbidden, "that stake is not yours")
			return
		}
		if respStake.IsResolved {
			respondErr(w, http.StatusConflict, "that stake has already been resolved")
			return
		}

		decSide := pduelSide(plan, latest.DeclarerID)
		declDie := int16(0)
		if latest.DeclarerDie != nil {
			declDie = *latest.DeclarerDie
		}
		outcome, err := gamepkg.ResolveBout(
			decSide, gamepkg.Declaration(*latest.Declaration), declDie, respStake.HiddenDie,
		)
		if err != nil {
			respondInternalErr(w, r, "could not resolve bout", err)
			return
		}

		winnerPtr := gamepkg.BoutWinnerID(outcome, decSide, latest.DeclarerID, latest.ResponderID)

		err = deps.Q.ResolveDuelBout(ctx, dbgen.ResolveDuelBoutParams{
			ID:               latest.ID,
			ResponderStakeID: &respStake.ID,
			ResponderDie:     &respStake.HiddenDie,
			WinnerID:         winnerPtr,
			IsMatch:          outcome.Match,
		})
		if err != nil {
			respondInternalErr(w, r, "could not resolve bout", err)
			return
		}

		// Mark both stakes resolved. Track is_winner per stake only when a
		// stake's die ended up in the winner's accumulated pool.
		declarerIsWinner := winnerPtr != nil && *winnerPtr == latest.DeclarerID
		responderIsWinner := winnerPtr != nil && *winnerPtr == latest.ResponderID
		_ = deps.Q.SetDuelStakeResolved(ctx, dbgen.SetDuelStakeResolvedParams{
			ID: latest.DeclarerStakeID, IsWinner: &declarerIsWinner,
		})
		_ = deps.Q.SetDuelStakeResolved(ctx, dbgen.SetDuelStakeResolvedParams{
			ID: respStake.ID, IsWinner: &responderIsWinner,
		})

		// Swap initiative to the other side.
		nextInit := pduelOpponentID(plan, latest.DeclarerID)
		state.InitiativePlayerID = &nextInit

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			winID := int64(0)
			if winnerPtr != nil {
				winID = *winnerPtr
			}
			h.BroadcastEvent(model.EventDuelBoutResolved, model.DuelBoutResolvedPayload{
				PlanID:       plan.ID,
				BoutNumber:   latest.BoutNumber,
				DeclarerID:   latest.DeclarerID,
				ResponderID:  latest.ResponderID,
				DeclarerDie:  declDie,
				ResponderDie: respStake.HiddenDie,
				WinnerID:     winID,
				IsMatch:      outcome.Match,
			})
		}

		// Check remaining stakes.
		prepLeft, err := deps.Q.CountUnresolvedDuelStakes(ctx, dbgen.CountUnresolvedDuelStakesParams{
			PlanID: plan.ID, PlayerID: plan.PreparerID,
		})
		if err != nil {
			respondInternalErr(w, r, "could not count stakes", err)
			return
		}
		var targLeft int64
		if plan.TargetPlayerID != nil {
			targLeft, err = deps.Q.CountUnresolvedDuelStakes(ctx, dbgen.CountUnresolvedDuelStakesParams{
				PlanID: plan.ID, PlayerID: *plan.TargetPlayerID,
			})
			if err != nil {
				respondInternalErr(w, r, "could not count stakes", err)
				return
			}
		}

		if prepLeft == 0 || targLeft == 0 {
			// Bouts complete — create the standard roll with accumulated dice.
			if err := pduelCreateFinalRoll(ctx, deps, plan, &resData, state); err != nil {
				respondInternalErr(w, r, "could not create final roll", err)
				return
			}
		} else {
			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondInternalErr(w, r, "could not save state", err)
				return
			}
		}

		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{
			"plan_id":        plan.ID,
			"bout":           latest.BoutNumber,
			"winner_id":      winnerPtr,
			"is_match":       outcome.Match,
			"bouts_complete": prepLeft == 0 || targLeft == 0,
		})
	}
}

// pduelCreateFinalRoll accumulates winning dice by side and creates the
// plan's standard dice roll with dice pre-assigned. Preparer's accumulated
// dice become actor dice; target's accumulated dice become interference.
//

func pduelCreateFinalRoll(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	state *gamepkg.DuelResolutionData,
) error {
	bouts, err := deps.Q.ListDuelBoutsByPlan(ctx, plan.ID)
	if err != nil {
		return fmt.Errorf("list bouts: %w", err)
	}
	// Build the domain snapshot (chronological bout views) and let the pure
	// rule accumulate winning dice, including tied-bout carry-over. See
	// game.AccumulateDuelDice for the carry-over semantics.
	views := make([]gamepkg.DuelBoutView, 0, len(bouts))
	for _, bt := range bouts {
		var winnerIsPrep *bool
		if bt.WinnerID != nil {
			v := *bt.WinnerID == plan.PreparerID
			winnerIsPrep = &v
		}
		views = append(views, gamepkg.DuelBoutView{
			DeclarerDie:      bt.DeclarerDie,
			ResponderDie:     bt.ResponderDie,
			IsMatch:          bt.IsMatch,
			WinnerIsPreparer: winnerIsPrep,
		})
	}
	prepDice, targDice := gamepkg.AccumulateDuelDice(views)

	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return fmt.Errorf("load game: %w", err)
	}
	difficulty, err := pduelHandler{}.ComputeDifficulty(ctx, deps.Q, plan, resData)
	if err != nil {
		return fmt.Errorf("compute difficulty: %w", err)
	}

	// Create the roll without the default 2 actor dice (createPlanRoll adds 2).
	// Here we create the roll row directly and add one die per accumulated face.
	rollRow, err := deps.Q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
		GameID:     game.ID,
		PlanID:     &plan.ID,
		RowNumber:  &game.CurrentRow,
		ActorID:    plan.PreparerID,
		Difficulty: difficulty,
		Stage:      "leverage",
	})
	if err != nil {
		return fmt.Errorf("create roll: %w", err)
	}

	for _, face := range prepDice {
		die, err := deps.Q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
			RollID: rollRow.ID, PlayerID: plan.PreparerID, IsInterference: false,
		})
		if err != nil {
			return err
		}
		f := face
		if err := deps.Q.SetDieFace(ctx, dbgen.SetDieFaceParams{ID: die.ID, Face: &f}); err != nil {
			return err
		}
	}
	for _, face := range targDice {
		oppID := plan.PreparerID
		if plan.TargetPlayerID != nil {
			oppID = *plan.TargetPlayerID
		}
		die, err := deps.Q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
			RollID: rollRow.ID, PlayerID: oppID, IsInterference: true,
		})
		if err != nil {
			return err
		}
		f := face
		if err := deps.Q.SetDieFace(ctx, dbgen.SetDieFaceParams{ID: die.ID, Face: &f}); err != nil {
			return err
		}
	}

	state.Phase = duelPhaseRoll
	if err := saveResolutionData(ctx, deps.Q, plan.ID, *resData); err != nil {
		return err
	}

	if h, ok := deps.Manager.Get(plan.GameID); ok {
		h.BroadcastEvent(model.EventRollCreated, model.RollCreatedPayload{Roll: rollRow})
		h.BroadcastEvent(model.EventDuelBoutsComplete, model.DuelBoutsCompletePayload{
			PlanID:       plan.ID,
			PreparerDice: prepDice,
			OpponentDice: targDice,
			RollID:       rollRow.ID,
		})
	}
	return nil
}
