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
	"errors"
	"fmt"
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
		recipient, err := AssetRecipientForPlan(ctx, deps.Q, plan)
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
