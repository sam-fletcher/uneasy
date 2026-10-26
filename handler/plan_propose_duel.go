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
//	               with accumulated dice pre-assigned, resolves it immediately
//	               (no leverage window — the bout dice are the whole roll), and
//	               advances to "roll". A declared bout's CALL (which stake,
//	               high/low) is public; its DICE are not until it resolves —
//	               see duelBoutView.
//	"roll"         The resolved roll shows in the standard dice panel; no dice
//	               can be added. The claim transfers assets and leverages all
//	               stakes. It applies itself when the winner's budget covers
//	               every stake the loser put up (nothing to choose — see
//	               AutoApplyChoiceOnRoll); otherwise the winner submits
//	               make-choice with their pick.
//	"done"         Stakes claimed. The plan resolves itself
//	               (AutoCompleteAfterChoice); there is no "Complete plan" step.
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
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

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

// PreparedDescriptor names the challenged player and the duel type (arms/wits)
// in the plan.prepared log, which the generic line drops.
func (pduelHandler) PreparedDescriptor(
	ctx context.Context,
	q *dbgen.Queries,
	plan dbgen.Plan,
	resData *ResolutionData,
) (string, bool) {
	if plan.TargetPlayerID == nil {
		return "", false
	}
	weapon := "a duel"
	if d := resData.Duel; d != nil && d.DuelType != "" {
		weapon = fmt.Sprintf("a duel of %s", d.DuelType)
	}
	return fmt.Sprintf("prepared Propose Duel, challenging %s to %s%s",
		playerDisplayName(ctx, q, *plan.TargetPlayerID), weapon, notesSuffix(plan)), true
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

// AutoApplyChoiceOnRoll opts the duel into recording its result the moment the
// final roll lands. The claim is only sometimes decision-free, so ApplyChoice —
// not this flag — decides: it applies the result when the winner has no real
// pick to make (see pduelForcedClaim) and no-ops when they do, leaving the
// claim to them. The duel's roll is created already-resolved at the end of the
// bouts (pduelCreateFinalRoll), so this is the first moment the outcome exists.
func (pduelHandler) AutoApplyChoiceOnRoll() bool {
	return true
}

// AutoCompleteAfterChoice resolves the duel without a separate "Complete plan"
// click. Once the stakes are claimed and leveraged the done view offers no
// decision and no information the action log doesn't already carry, and the
// click landed on the PREPARER — who, on a mar, is the player who just lost the
// duel and has nothing left to say about it (DESIGN_PRINCIPLES.md principle 3).
func (pduelHandler) AutoCompleteAfterChoice(_ *dbgen.Plan, resData *ResolutionData) bool {
	return resData.Duel != nil && resData.Duel.Phase == duelPhaseDone
}

// pduelClaimBudget returns how many of the loser's staked assets the winner may
// claim, read off the plan's resolved roll ("equal to your result" on a make,
// "equal to the difficulty" on a mar). ok=false when there is no resolved roll
// to read a budget from, in which case the caller must not synthesise a claim.
func pduelClaimBudget(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan, result string) (int, bool) {
	roll, err := q.GetDiceRollByPlanID(ctx, &plan.ID)
	if err != nil || roll.Result == nil {
		return 0, false
	}
	difficulty := roll.Difficulty
	if roll.AdjustedDifficulty != nil {
		difficulty = *roll.AdjustedDifficulty
	}
	return pduelHandler{}.MaxChoices(result, *roll.Result, difficulty), true
}

// ApplyChoice transfers assets and leverages all staked assets after the
// standard dice roll resolves. choices is the list of asset IDs the winner
// chose to take (as stringified int64s); each must be one of the loser's own
// staked assets.
//
// The winner is only *asked* which stakes to take when that is a real question.
// Their budget often covers every stake the loser put up — "take a number equal
// to your result" is mandatory, not a maximum — and then the picker has exactly
// one legal answer. In that case the server fills the claim in itself, whether
// it got here from the roll resolving (AutoApplyChoiceOnRoll, choices nil) or
// from a client posting make-choice. Conversely, when the budget is smaller than
// the pile there IS a decision, and an empty claim means it hasn't been made
// yet: ApplyChoice leaves the duel untouched rather than silently resolving it
// for nothing.
//
//nolint:funlen // one linear claim/leverage/transfer/log sequence
func (pduelHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	choices []string,
	result string,
) error {
	// Idempotent: the manual make-choice route stays mounted after the roll has
	// already auto-applied its outcome.
	if d := resData.Duel; d != nil && d.Phase == duelPhaseDone {
		return nil
	}

	stakes, err := deps.Q.ListDuelStakesByPlan(ctx, plan.ID)
	if err != nil {
		return fmt.Errorf("list stakes: %w", err)
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
	claimable := make([]int64, 0, len(stakes))
	loserStakeIDs := make(map[int64]bool)
	for _, s := range stakes {
		if s.PlayerID == loserID {
			claimable = append(claimable, s.AssetID)
			loserStakeIDs[s.AssetID] = true
		}
	}

	choices, decided := pduelResolveClaim(ctx, deps.Q, plan, result, choices, claimable)
	if !decided {
		// A genuine subset pick that the winner still owes. Nothing applies —
		// not even the leverage, which the rules hang off the same moment.
		return nil
	}
	// Record what is actually being applied, so the stored choices match the
	// transfers even when the server filled them in.
	resData.MakeMarChoices = make([]Choice, len(choices))
	for i, c := range choices {
		resData.MakeMarChoices[i] = Choice{Option: c}
	}

	// Validate the whole claim before touching anything: each choice is a stake
	// asset ID staked by the loser. Checking up front means a rejected claim
	// leaves no half-applied state (stakes leveraged, duel still in 'roll').
	claim := make([]int64, 0, len(choices))
	for _, c := range choices {
		var assetID int64
		if _, err := fmt.Sscanf(c, "%d", &assetID); err != nil || assetID == 0 {
			continue
		}
		if !loserStakeIDs[assetID] {
			return fmt.Errorf("asset %d is not one of the losing side's staked assets", assetID)
		}
		if _, err := deps.Q.GetAssetByID(ctx, assetID); err != nil {
			return fmt.Errorf("asset %d not found", assetID)
		}
		claim = append(claim, assetID)
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

	taken := 0
	for _, assetID := range claim {
		if _, err := takeAssetEffect(ctx, deps.Q, deps.Manager, plan.GameID, assetID, loserID, winnerID); err != nil {
			return fmt.Errorf("transfer asset %d: %w", assetID, err)
		}
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

// pduelResolveClaim decides which of the loser's staked assets the winner takes.
//
// It returns the claim to apply and decided=true when the answer is settled —
// either because the winner's budget covers the whole pile (so "take all" is
// the one legal answer and whatever the caller passed is beside the point), or
// because there is nothing to take at all. It returns decided=false only for
// the case worth a gate: the budget is smaller than the pile, so the winner
// genuinely chooses a subset, and hasn't yet.
//
// A claim that was explicitly posted is honoured as-is; the per-asset
// "was it staked by the loser?" and over-budget checks still run on it
// (ApplyChoice and enforceChoiceBudget respectively).
func pduelResolveClaim(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	result string,
	choices []string,
	claimable []int64,
) ([]string, bool) {
	budget, ok := pduelClaimBudget(ctx, q, plan, result)
	if !ok {
		// No resolved roll to size the claim against — fall back to whatever
		// the caller supplied rather than inventing one.
		return choices, len(choices) > 0
	}
	switch {
	case budget >= len(claimable):
		// Every stake changes hands. One legal answer, so don't ask.
		all := make([]string, len(claimable))
		for i, id := range claimable {
			all[i] = strconv.FormatInt(id, 10)
		}
		return all, true
	case budget == 0:
		// Nothing to take (a made roll with result 0); the leverage still applies.
		return nil, true
	default:
		return choices, len(choices) > 0
	}
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

// duelBoutView is the wire shape of a duel bout, and exists for the same
// reason duelStakeView does: dbgen.DuelBout carries both dice with json tags,
// and the bout row is written at DECLARE time with the declarer's die already
// in it (see pduelBoutDeclareHandler). Handing the raw row back would let the
// responder — whose whole job is to answer blind — read the die they are about
// to answer. The two dice are masked exactly as a stake's is: the owning side
// sees their own, and both become public the moment the bout resolves.
//
// Everything else about a declared bout IS public: which stake was opened
// with, and whether the declarer called high or low. That is the call the
// responder is answering, so it ships unmasked (rules: "Player with initiative
// selects a staked asset and declares 'high' or 'low.'").
type duelBoutView struct {
	ID               int64              `json:"id"`
	PlanID           int64              `json:"plan_id"`
	BoutNumber       int16              `json:"bout_number"`
	DeclarerID       int64              `json:"declarer_id"`
	DeclarerStakeID  int64              `json:"declarer_stake_id"`
	ResponderID      int64              `json:"responder_id"`
	ResponderStakeID *int64             `json:"responder_stake_id"`
	Declaration      *string            `json:"declaration"`
	DeclarerDie      *int16             `json:"declarer_die"`
	ResponderDie     *int16             `json:"responder_die"`
	WinnerID         *int64             `json:"winner_id"`
	IsMatch          bool               `json:"is_match"`
	CreatedAt        pgtype.Timestamptz `json:"created_at"`
	ResolvedAt       pgtype.Timestamptz `json:"resolved_at"`
}

// duelBoutViews masks each bout's dice for viewerID. A bout's dice are public
// once it has resolved (ResolvedAt set) — that is the reveal the whole minigame
// turns on — and before that each side sees only its own. This is the same rule
// GetDuelState applies to stakes (owner, or resolved), keyed on the bout's
// recorded combatants: the bout row names the preparer as declarer even when a
// Make Demands perform_steps winner is the one driving that seat.
func duelBoutViews(bouts []dbgen.DuelBout, viewerID int64) []duelBoutView {
	views := make([]duelBoutView, 0, len(bouts))
	for _, b := range bouts {
		resolved := b.ResolvedAt.Valid
		v := duelBoutView{
			ID:               b.ID,
			PlanID:           b.PlanID,
			BoutNumber:       b.BoutNumber,
			DeclarerID:       b.DeclarerID,
			DeclarerStakeID:  b.DeclarerStakeID,
			ResponderID:      b.ResponderID,
			ResponderStakeID: b.ResponderStakeID,
			Declaration:      b.Declaration,
			WinnerID:         b.WinnerID,
			IsMatch:          b.IsMatch,
			CreatedAt:        b.CreatedAt,
			ResolvedAt:       b.ResolvedAt,
		}
		if resolved || b.DeclarerID == viewerID {
			v.DeclarerDie = b.DeclarerDie
		}
		if resolved || b.ResponderID == viewerID {
			v.ResponderDie = b.ResponderDie
		}
		views = append(views, v)
	}
	return views
}

// GetDuelState returns the full duel state a caller is allowed to see: all
// stakes and the full bout history, both with their hidden dice masked for
// unresolved rows the caller doesn't own. Neither collection may be handed
// back raw — see duelStakeView and duelBoutView.
// GET /api/plans/:planId/duel-state.
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

		// During setup (before both duellists have committed) a player's stakes —
		// and even their count — stay hidden from the opponent, preserving the
		// rules' simultaneous blind reveal. The caller always sees their own.
		duelState := gamepkg.LoadDuelData(plan.ResolutionData)
		inSetup := duelState.Phase == gamepkg.DuelPhaseSetup || duelState.Phase == ""

		views := make([]duelStakeView, 0, len(stakes))
		for _, s := range stakes {
			if inSetup && s.PlayerID != player.ID {
				continue
			}
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
			views = append(views, v)
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID,
			"stakes":  views,
			"bouts":   duelBoutViews(bouts, player.ID),
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

// pduelEffectiveSeat returns the duellist seat playerID occupies for bout
// actions (declare/respond), and ok=false if they hold no seat.
//
// A duellist occupies their own seat. A Make Demands perform_steps winner stands
// in the PREPARER's seat — they drive the preparer's bouts and the preparer is
// locked out of that seat. The TARGET duellist's seat is never transferred:
// perform_steps replaces only the preparer's role, never a third party's. The
// returned seat (not the raw caller) keys initiative/responder/stake-ownership,
// so the bout records the preparer as the combatant regardless of who clicked.
func pduelEffectiveSeat(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan, playerID int64) (int64, bool) {
	if pduelIsParticipant(plan, playerID) {
		// The preparer is locked out of their own seat once a perform_steps
		// winner (someone else) holds it.
		if playerID == plan.PreparerID {
			if w := performStepsWinner(ctx, q, plan); w != 0 && w != plan.PreparerID {
				return 0, false
			}
		}
		return playerID, true
	}
	// A non-duellist may drive the preparer's seat iff they won perform_steps.
	if w := performStepsWinner(ctx, q, plan); w != 0 && playerID == w {
		return plan.PreparerID, true
	}
	return 0, false
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

// ResolvingWaitees returns the narrower RowState for a resolving Propose Duel
// plan. setup/staking are simultaneous-submit; ActingPlayerIDs lists whoever
// still owes a submission. 'bouts' blocks on a single player — the responder if
// a declared bout is unresolved, otherwise the declarer (InitiativePlayerID).
// 'roll' and 'done' return false and ride the generic PlanResolving case (roll
// is the standard dice flow; done is terminal).
func (pduelHandler) ResolvingWaitees(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	state := gamepkg.LoadDuelData(plan.ResolutionData)
	switch state.Phase {
	case gamepkg.DuelPhaseSetup, gamepkg.DuelPhaseStaking:
		ids := duelStakingActors(ctx, q, plan)
		if len(ids) == 0 {
			// Both duellists have submitted; the phase is about to advance.
			// Ride the generic resolution case (which names the preparer)
			// rather than naming both duellists — naming everyone when no one
			// owes anything is the over-attribution class this whole audit set
			// out to remove. Mirrors the liaise "all in → preparer" convention.
			return model.RowState{}, false
		}
		return model.RowState{Kind: model.RowStateAwaitDuelStaking, ActingPlayerIDs: ids}, true
	case gamepkg.DuelPhaseBouts:
		actor := duelBoutActor(ctx, q, plan, &state)
		if actor == 0 {
			return model.RowState{}, false
		}
		return model.RowState{Kind: model.RowStateAwaitDuelBout, ActingPlayerIDs: []int64{actor}}, true
	case gamepkg.DuelPhaseRoll:
		// A marred roll hands the asset-claim make-choice to the *target* (the
		// mar winner): they pick which of the preparer's staked assets to take.
		// Until ApplyChoice advances the phase to 'done', the bar must name the
		// target, not the preparer. A made roll is claimed by the preparer, so it
		// rides the generic case. planRollIsMar is false until the roll resolves,
		// so the pre-roll window (preparer owns the roll) also stays generic.
		if plan.TargetPlayerID != nil && planRollIsMar(ctx, q, plan) {
			return model.RowState{
				Kind:            model.RowStatePlanResolving,
				ActingPlayerIDs: []int64{*plan.TargetPlayerID},
			}, true
		}
		return model.RowState{}, false
	case gamepkg.DuelPhaseDone:
		return model.RowState{}, false
	}
	return model.RowState{}, false
}

// duelStakingActors returns the duellists who still owe a stake commit during
// setup — anyone who hasn't placed their stakes yet. A commit creates all of a
// duellist's stake rows atomically, so "has any stake row" == "has committed".
// We derive this from the stake rows rather than from a count in resolution_data
// so the opponent's commit (and count) never leaks before both are in. The
// result may be empty (both committed, phase about to advance to bouts); the
// caller then rides the generic preparer case rather than naming both — see
// ResolvingWaitees.
func duelStakingActors(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) []int64 {
	participants := []int64{plan.PreparerID}
	if plan.TargetPlayerID != nil {
		participants = append(participants, *plan.TargetPlayerID)
	}

	stakes, err := q.ListDuelStakesByPlan(ctx, plan.ID)
	if err != nil {
		return participants
	}
	committed := map[int64]bool{}
	for i := range stakes {
		committed[stakes[i].PlayerID] = true
	}

	var pending []int64
	for _, p := range participants {
		if !committed[p] {
			pending = append(pending, p)
		}
	}
	return pending
}

// duelBoutActor returns the duellist who owes the next bout action: the
// responder if a declared bout is unresolved, else the declarer (=
// InitiativePlayerID). Returns 0 if neither can be determined.
func duelBoutActor(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan, state *gamepkg.DuelResolutionData) int64 {
	latest, err := q.GetLatestDuelBout(ctx, plan.ID)
	if err == nil && !latest.ResolvedAt.Valid {
		return latest.ResponderID
	}
	if state.InitiativePlayerID != nil {
		return *state.InitiativePlayerID
	}
	return 0
}
