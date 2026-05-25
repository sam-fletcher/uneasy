package handler

// handler/rolls_dice.go — Pure dice math + resolution finalization.
//
// This file owns the parts of a dice roll that don't care about HTTP,
// participants, or the stage machine: rolling random faces, the
// interference cancellation algorithm, computing the result and outcome,
// and writing the resolved-roll state to the DB.
//
// finalizeRoll is the one entry point that does mutate DB state; the
// rest are pure functions covered by unit tests in rolls_test.go.

import (
	"context"
	"math/rand/v2"
	"net/http"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// dieEntry is a lightweight representation of a die used in roll processing.
type dieEntry struct {
	id   int64
	face int16
}

// cancelInterference groups dice by face and returns, per cancelled actor
// die, the interference die that cancelled it.
func cancelInterference(actorDice, interfereDice []dieEntry) map[int64]int64 {
	actorByFace := make(map[int16][]dieEntry)
	for _, e := range actorDice {
		actorByFace[e.face] = append(actorByFace[e.face], e)
	}
	intByFace := make(map[int16][]dieEntry)
	for _, e := range interfereDice {
		intByFace[e.face] = append(intByFace[e.face], e)
	}
	pairs := make(map[int64]int64)
	for face, intGroup := range intByFace {
		actorGroup := actorByFace[face]
		n := min(len(intGroup), len(actorGroup))
		for i := range n {
			pairs[actorGroup[i].id] = intGroup[i].id
		}
	}
	return pairs
}

// rollAndCancelDice rolls all dice and applies interference cancellation,
// writing the cancelling die's id back to each cancelled die. Dice that
// already have a face (banked dice no longer do, but Propose Duel's
// pre-set faces survive) keep it; everything else gets a fresh roll.
func rollAndCancelDice(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
	dice []dbgen.DiceRollDice,
) ([]dieEntry, map[int64]struct{}, error) {
	var actorDice, interfereDice []dieEntry
	for _, d := range dice {
		var f int16
		if d.Face != nil && *d.Face >= 1 && *d.Face <= diceSides {
			f = *d.Face
		} else {
			f = int16(rand.IntN(diceSides) + 1)
			if err := q.SetDieFace(ctx, dbgen.SetDieFaceParams{ID: d.ID, Face: &f}); err != nil {
				respondInternalErr(w, r, "could not set die face", err)
				return nil, nil, err
			}
		}
		e := dieEntry{id: d.ID, face: f}
		if d.IsInterference {
			interfereDice = append(interfereDice, e)
		} else {
			actorDice = append(actorDice, e)
		}
	}
	pairs := cancelInterference(actorDice, interfereDice)
	cancelledIDs := make(map[int64]struct{}, len(pairs))
	for cancelledID, byID := range pairs {
		byVal := byID
		if err := q.SetDieCancelledBy(ctx, dbgen.SetDieCancelledByParams{
			ID: cancelledID, CancelledByDieID: &byVal,
		}); err != nil {
			respondInternalErr(w, r, "could not cancel die", err)
			return nil, nil, err
		}
		cancelledIDs[cancelledID] = struct{}{}
	}
	return actorDice, cancelledIDs, nil
}

// calculateRollResult computes the result and outcome of a resolved roll.
// result = number of distinct faces in the actor's uncancelled dice;
// outcome = "make" when result ≥ effective difficulty, else "mar".
func calculateRollResult(
	actorDice []dieEntry,
	cancelledIDs map[int64]struct{},
	roll *dbgen.DiceRoll,
) (int16, string) {
	distinctFaces := make(map[int16]struct{})
	for _, e := range actorDice {
		if _, exists := cancelledIDs[e.id]; !exists {
			distinctFaces[e.face] = struct{}{}
		}
	}
	result := int16(len(distinctFaces))

	effectiveDifficulty := roll.Difficulty
	if roll.AdjustedDifficulty != nil {
		effectiveDifficulty = *roll.AdjustedDifficulty
	}
	outcome := marOutcome
	if result >= effectiveDifficulty {
		outcome = makeOutcome
	}
	return result, outcome
}

// finalizeRoll rolls every die, applies interference cancellation, writes
// the result/outcome to the DB, and broadcasts roll.resolved. Called from
// maybeAutoResolve and (legacy) CloseLeverage.
func finalizeRoll(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
	manager *hub.Manager,
	roll *dbgen.DiceRoll,
) error {
	dice, err := q.ListDiceByRoll(ctx, roll.ID)
	if err != nil {
		return err
	}
	actorDice, cancelledIDs, err := rollAndCancelDice(ctx, w, r, q, dice)
	if err != nil {
		return err
	}
	result, outcome := calculateRollResult(actorDice, cancelledIDs, roll)
	if err := q.ResolveDiceRoll(ctx, dbgen.ResolveDiceRollParams{
		ID: roll.ID, Result: &result, Outcome: &outcome,
	}); err != nil {
		return err
	}
	resolved, err := q.GetDiceRollByID(ctx, roll.ID)
	if err != nil {
		return err
	}
	finalDice, err := q.ListDiceByRoll(ctx, roll.ID)
	if err != nil {
		finalDice = []dbgen.DiceRollDice{}
	}
	cancelledDice := []dbgen.DiceRollDice{}
	for _, d := range finalDice {
		if d.IsCancelled {
			cancelledDice = append(cancelledDice, d)
		}
	}
	broadcastEvent(manager, roll.GameID, model.EventRollResolved, model.RollResolvedPayload{
		Roll:          resolved,
		Dice:          finalDice,
		CancelledDice: cancelledDice,
	})
	// Roll outcome can change the RowState — e.g. a marred Make Demands
	// roll transitions plan_resolving → await_demand_counter. Recompute
	// and broadcast unconditionally; ComputeRowState is cheap and the
	// no-op case is harmless on the client.
	broadcastRowState(ctx, q, manager, roll.GameID)
	return nil
}
