package handler

// handler/shake_up_rolls.go — Shake-Up step-1 dice rolls: determining the
// next roller in reverse-rank turn order, opening their roll, and applying
// the token gain once it resolves. See shake_up.go for the phase's full
// lifecycle.

import (
	"context"
	"errors"
	"fmt"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// ── Real dice rolls (step 1) ─────────────────────────────────────────────────

// shakeUpNextRoller returns the player whose turn it is to roll in the
// current category's step 1: the first player in reverse-rank turn order
// (gamepkg.ShakeUpTurnOrder — lowest status first, dummies already skipped)
// who has no dice_rolls row for this category yet. The partial unique index
// uq_one_shake_up_roll_per_category guarantees at most one row per
// (game, actor, category) ever exists, so "has a row" is the durable
// "already rolled" check — unlike the old tokens>0 proxy, this survives
// tokens being spent before every roll of the category resolves. Returns 0
// once every player has rolled.
func shakeUpNextRoller(ctx context.Context, q *dbgen.Queries, gameID int64, category string) (int64, error) {
	rankings, err := q.ListRankingsByGame(ctx, gameID)
	if err != nil {
		return 0, fmt.Errorf("load rankings: %w", err)
	}
	order := gamepkg.ShakeUpTurnOrder(category, rankingRows(rankings))

	rolls, err := q.ListDiceRollsByGame(ctx, gameID)
	if err != nil {
		return 0, fmt.Errorf("load rolls: %w", err)
	}
	rolled := make(map[int64]bool, len(rolls))
	for _, roll := range rolls {
		if roll.IsShakeUp && roll.ShakeUpCategory != nil && *roll.ShakeUpCategory == category {
			rolled[roll.ActorID] = true
		}
	}
	for _, pid := range order {
		if !rolled[pid] {
			return pid, nil
		}
	}
	return 0, nil
}

// shakeUpOpenRollForRoller creates rollerID's step-1 roll for category: 2
// base dice, and a single participant row (the roller, intent aid, not
// ready) — shake-up rolls are actor-only, per SHAKEUP_RULES.md ("may not
// help or interfere with others"). The row is created directly in
// stage='leverage' (no difficulty vote — difficulty is the shake-up
// sentinel, 0), so no separate stage-transition call is needed here.
//
// This never needs to force an immediate auto-resolve: the roller is always
// the roll's sole participant, so whenever they call the existing
// /api/rolls/:id/ready endpoint (Session 3's "Roll the dice" button),
// maybeAutoResolve resolves it on that same request — regardless of whether
// they had anything to leverage. That happens through the ordinary HTTP
// roll endpoints, which already have a real ResponseWriter to report
// failures on; this helper runs from contexts that don't (e.g. BeginShakeUp,
// reached via the row-advance side effect chain), so it deliberately doesn't
// attempt to resolve anything itself.
func shakeUpOpenRollForRoller(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	category string,
	rollerID int64,
) error {
	cat := category
	roll, err := q.CreateShakeUpDiceRoll(ctx, dbgen.CreateShakeUpDiceRollParams{
		GameID: gameID, ActorID: rollerID, ShakeUpCategory: &cat,
	})
	if err != nil {
		return fmt.Errorf("create shake-up roll: %w", err)
	}
	for range 2 {
		if _, err := q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
			RollID: roll.ID, PlayerID: rollerID, IsInterference: false,
		}); err != nil {
			return fmt.Errorf("create die: %w", err)
		}
	}
	aid := intentAid
	if err := q.CreateRollParticipant(ctx, dbgen.CreateRollParticipantParams{
		RollID: roll.ID, PlayerID: rollerID, Intent: &aid, IsReady: false,
	}); err != nil {
		return fmt.Errorf("create participant: %w", err)
	}
	broadcastEvent(manager, gameID, model.EventRollCreated, model.RollCreatedPayload{Roll: roll})
	return nil
}

// finalizeShakeUpRoll applies a just-resolved shake-up roll's token gain
// (result = distinct faces, per ruling 1) and advances the rolling step:
// creates the next reverse-rank roller's roll, or — once every player has
// rolled this category — flips the step to spending. Called from
// finalizeRoll (rolls_dice.go) once the roll itself is resolved and
// broadcast.
func finalizeShakeUpRoll(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	resolved *dbgen.DiceRoll,
	diceCount, result int16,
) error {
	if resolved.ShakeUpCategory == nil {
		return errors.New("shake-up roll missing category")
	}
	category := *resolved.ShakeUpCategory
	newTotal, err := q.AddShakeUpTokens(ctx, dbgen.AddShakeUpTokensParams{
		ID: resolved.ActorID, ShakeUpTokens: result,
	})
	if err != nil {
		return fmt.Errorf("add tokens: %w", err)
	}
	broadcastEvent(manager, resolved.GameID, model.EventShakeUpRolled, model.ShakeUpRolledPayload{
		PlayerID: resolved.ActorID, Result: result, Total: newTotal,
	})
	EmitShakeUpRolled(ctx, q, manager, resolved.GameID, resolved.ActorID, diceCount, result, newTotal, category)

	next, err := shakeUpNextRoller(ctx, q, resolved.GameID, category)
	if err != nil {
		return fmt.Errorf("determine next roller: %w", err)
	}
	if next != 0 {
		if err := shakeUpOpenRollForRoller(ctx, q, manager, resolved.GameID, category, next); err != nil {
			return fmt.Errorf("create next roll: %w", err)
		}
		return nil
	}

	step := gamepkg.ShakeUpStepSpending
	if err := q.SetShakeUpStep(ctx, dbgen.SetShakeUpStepParams{
		ID: resolved.GameID, ShakeUpCategory: &category, ShakeUpStep: &step,
	}); err != nil {
		return fmt.Errorf("advance to spending: %w", err)
	}
	broadcastEvent(manager, resolved.GameID, model.EventShakeUpStepChanged,
		model.ShakeUpStepChangedPayload{Category: category, Step: step})
	return nil
}
