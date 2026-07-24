package handler

// handler/plan_seek_answers.go — Seek Answers plan handler (Phase 3b).
//
// Seek Answers (knowledge, delay 4): The preparer investigates a topic.
// Difficulty = preparer's rank on the knowledge track.
//
// Preparing: description of research methods/topics (preparation_notes required).
//
// Pre-Roll: OnResolve opens the plan in 'resolving' with no roll. The preparer
// restates their methods and describes one thing they've learned so far via the
// cast-roll route, which posts that narration as their own scene post, computes
// difficulty, and creates the dice roll. (Mirrors Chronicle Histories' cast-roll
// pre-roll, minus the artifact invocations — Seek Answers' pre-roll is purely
// narrative and does not affect difficulty.)
//
// Make: choose N options equal to the dice result (repeatable):
//   - "break_resource"  → tear a marginalia on a target resource asset
//   - "declare_truth"   → narrative (posted as scene_post)
//   - "ask_question"    → narrative; knowledge-ranked players may veto once
//   - "reveal_secret"   → grant secret visibility on all secrets of a chosen asset
//
// Mar: choose options equal to the result from the same list. Then the server
// applies "describe a flaw" (break_resource) to the preparer's own resource
// assets equal to (difficulty - result); the preparer chooses which ones.
//
// Extra routes:
//   POST /api/plans/:planId/seek-cast-roll   {"narration": "..."}
//   POST /api/plans/:planId/break-resource   {"asset_id": N, "marginalia_id": M}
//   POST /api/plans/:planId/reveal-secret    {"asset_id": N}
import (
	"context"
	"errors"
	"fmt"
	"net/http"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanSeekAnswers, saHandler{})
}

type saHandler struct{}

func (saHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryKnowledge, Delay: 4}
}

func (saHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (*int16, string) {
	if v.Notes == "" {
		return nil, "seek_answers requires preparation_notes describing research methods and topics"
	}
	return nil, ""
}

func (saHandler) ComputeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	rank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryKnowledge)
	if err != nil {
		return 0, fmt.Errorf("could not determine preparer knowledge rank: %w", err)
	}
	return gamepkg.SeekAnswersDifficulty(rank), nil
}

// OnResolve opens the pre-roll narration step and returns nil — no roll yet.
// The plan sits in 'resolving' with PreRollDone=false until the preparer
// restates their methods, describes one thing they've learned, and casts the
// dice via the cast-roll route (which posts that narration, computes difficulty,
// and creates the roll). Mirrors Chronicle Histories. No log entry here — the
// standard plan.resolving post already marks the start, and the preparer's
// pre-roll narration is logged by cast-roll.
func (saHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	resData := loadResolutionData(plan.ResolutionData)
	sa := resData.EnsureSeekAnswers()
	sa.PreRollDone = false
	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return nil, fmt.Errorf("could not open pre-roll step: %w", err)
	}
	// Return nil — the roll is created later when the preparer calls cast-roll.
	return nil, nil
}

// ApplyChoice records the make/mar choices (kept in ResData.MakeMarChoices by
// MakeChoice) and, on a mar, fixes the self-flaw penalty. Mechanical effects
// (break_resource, reveal_secret) are performed afterwards via extra routes.
//
// Mar penalty (rules): "apply 'describe a flaw' to your own resource assets a
// number of times equal to (difficulty − result)." Because each resource may
// be flawed at most once and resources can only gain marginalia mid-resolution
// (never spawn anew), the effective requirement is capped here, once, at the
// number of the preparer's eligible own resources.
func (saHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	_ []string,
	result string,
) error {
	if result != marOutcome {
		saLog(ctx, deps, plan, model.SeverityDefault,
			"Seek Answers succeeded — the preparer presses their inquiries.")
		return nil
	}

	roll, err := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID)
	if err != nil {
		return fmt.Errorf("could not load roll for mar penalty: %w", err)
	}
	diff := roll.Difficulty
	if roll.AdjustedDifficulty != nil {
		diff = *roll.AdjustedDifficulty
	}
	var res int16
	if roll.Result != nil {
		res = *roll.Result
	}

	// Imperative shell: load the snapshot, then let the pure rule decide the
	// penalty. saEligibleOwnResources builds a game.ResourceFlawView snapshot
	// from the DB and applies the pure eligibility filter.
	eligible, err := saEligibleOwnResources(ctx, deps, plan.GameID, plan.PreparerID, nil)
	if err != nil {
		return fmt.Errorf("could not count preparer resources: %w", err)
	}
	required := gamepkg.SeekAnswersMarPenalty(diff, res, int16(len(eligible)))

	sa := resData.EnsureSeekAnswers()
	sa.MarSelfFlawsRequired = required
	saLog(ctx, deps, plan, model.SeverityImportant,
		fmt.Sprintf("Seek Answers marred — the preparer must break %d of their own resources.", required))
	return nil
}

// CanComplete blocks completion of a marred Seek Answers until the preparer has
// applied the full self-flaw penalty, and blocks any Seek Answers while a
// question is still awaiting its target's answer or veto.
func (saHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	sa := resData.SeekAnswers
	if sa == nil {
		return nil
	}
	if sa.PendingQuestion != nil {
		return errors.New("a question is awaiting an answer before you can complete")
	}
	if sa.MarSelfFlawsApplied < sa.MarSelfFlawsRequired {
		return fmt.Errorf("you must break %d more of your own resources before completing",
			sa.MarSelfFlawsRequired-sa.MarSelfFlawsApplied)
	}
	// Make-list completeness is server-authoritative: every committed pick must be
	// performed (or forfeited via seek-forfeit-step when no target remains) before
	// the plan can resolve. break_resource/reveal_secret are depletable and have a
	// forfeit path; declare_truth/ask_question always have a valid target in a live
	// game, so they're simply performed.
	return subflowPicksRemaining(resData,
		subflowProgress{"break_resource", "break-resource", int(sa.BreakResourceDone)},
		subflowProgress{"reveal_secret", "reveal-secret", int(sa.RevealSecretDone)},
		subflowProgress{"declare_truth", "declare-truth", int(sa.DeclareTruthDone)},
		subflowProgress{"ask_question", "ask-question", int(sa.AskQuestionDone)},
	)
}

// saEligibleOwnResources returns the IDs of the player's resource assets that
// can still be flawed this resolution. It is the imperative shell for the pure
// game.EligibleSelfFlawResourceIDs rule: it builds the domain snapshot from the
// DB (the owner's in-game assets plus each one's intact-marginalia count), then
// hands the decision to the pure filter.
func saEligibleOwnResources(
	ctx context.Context,
	deps *PlanDeps,
	gameID, ownerID int64,
	flawed []int64,
) ([]int64, error) {
	assets, err := deps.Q.ListAssetsByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	views := make([]gamepkg.ResourceFlawView, 0, len(assets))
	for _, a := range assets {
		if a.GameID != gameID {
			continue
		}
		intact, total := 0, 0
		// Only resources can be flawed; skip the marginalia queries otherwise.
		if a.AssetType == model.AssetResource && !a.IsDestroyed {
			marg, err := deps.Q.ListMarginaliaByAsset(ctx, a.ID)
			if err != nil {
				return nil, err
			}
			total = len(marg)
			for i := range marg {
				if !marg[i].IsTorn {
					intact++
				}
			}
		}
		views = append(views, gamepkg.ResourceFlawView{
			AssetID:               a.ID,
			AssetType:             a.AssetType,
			IsDestroyed:           a.IsDestroyed,
			IntactMarginaliaCount: intact,
			TotalMarginaliaCount:  total,
		})
	}
	return gamepkg.EligibleSelfFlawResourceIDs(views, flawed), nil
}

// saLog emits a Seek Answers action-log entry anchored to the plan's row.
func saLog(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, severity int32, body string) {
	planID := plan.ID
	EmitSystemPost(ctx, deps.Q, deps.Manager, plan.GameID, "plan.seek_answers",
		severity, body, plan.RowNumber, &planID, nil,
		map[string]any{"plan_id": plan.ID})
}

// MaxChoices: both make and mar choose options from the make list equal to the
// result. (On a mar the preparer additionally flaws their own resources
// difficulty − result times, applied via the break-resource route.)
func (saHandler) MaxChoices(_ string, rollResult, _ int16) int {
	return int(rollResult)
}

func (saHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"seek-cast-roll":    saCastRollHandler(deps),
		"break-resource":    saBreakResourceHandler(deps),
		"reveal-secret":     saRevealSecretHandler(deps),
		"declare-truth":     saDeclareTruthHandler(deps),
		"ask-question":      saAskQuestionHandler(deps),
		"veto-question":     saVetoQuestionHandler(deps),
		"answer-question":   saAnswerQuestionHandler(deps),
		"seek-forfeit-step": saForfeitStepHandler(deps),
	}
}

// ResolvingWaitees returns AwaitQuestionAnswer while a Seek Answers "ask a player
// a question" pick is waiting on the target's answer or veto. ActingPlayerIDs
// names the target, so the table blocks on them rather than the resolving plan's
// focus player. No pending question → ride the generic PlanResolving case.
func (saHandler) ResolvingWaitees(_ context.Context, _ *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	resData := loadResolutionData(plan.ResolutionData)
	if sa := resData.SeekAnswers; sa != nil && sa.PendingQuestion != nil {
		target := sa.PendingQuestion.TargetID
		return model.RowState{Kind: model.RowStateAwaitQuestionAnswer, ActingPlayerIDs: []int64{target}}, true
	}
	return model.RowState{}, false
}
