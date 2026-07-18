package handler

// handler/wait_state.go — ComputeWaitState (Notifications Plan Session 2).
//
// One server-side answer to "who must act right now?", dispatched per game
// phase — the same question ComputeRowState already answers for main_event,
// generalized to lobby/prologue/shake_up/ended so the Session 3 notification
// ticker (and, later, a "your turn" badge) have a single source across the
// whole game lifecycle. Read-only: this file makes no mutations and sends no
// broadcasts.

import (
	"context"
	"strings"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// ComputeWaitState returns the single authoritative WaitState for a game,
// dispatched by phase:
//
//   - lobby      → the facilitator, once at least two players have joined
//     (otherwise nobody — there's no one else to wait on yet).
//   - prologue   → computePrologueWaitees, one of the four modes the
//     prologue view itself derives from prologue_ranking_step.
//   - main_event → ComputeRowState verbatim; RowState stays the richer,
//     authoritative source for this phase (PlanID, SceneID, etc.).
//   - shake_up   → computeShakeUpWaitees, the same function GetShakeUp uses
//     for its own current-actor/current-roller fields.
//   - ended      → nobody.
func ComputeWaitState(ctx context.Context, q *dbgen.Queries, gameID int64) (model.WaitState, error) {
	game, err := q.GetGameByID(ctx, gameID)
	if err != nil {
		return model.WaitState{}, err
	}

	switch game.Phase {
	case model.PhaseLobby:
		ids, lErr := computeLobbyWaitees(ctx, q, gameID)
		if lErr != nil {
			return model.WaitState{}, lErr
		}
		kind := model.WaitKindNobody
		if len(ids) > 0 {
			kind = model.WaitKindLobbyFacilitator
		}
		return model.WaitState{Phase: game.Phase, Kind: kind, ActingPlayerIDs: ids}, nil

	case model.PhasePrologue:
		kind, ids, pErr := computePrologueWaitees(ctx, q, gameID, &game)
		if pErr != nil {
			return model.WaitState{}, pErr
		}
		return model.WaitState{Phase: game.Phase, Kind: kind, ActingPlayerIDs: ids}, nil

	case model.PhaseMainEvent:
		rs, rErr := ComputeRowState(ctx, q, gameID)
		if rErr != nil {
			return model.WaitState{}, rErr
		}
		return model.WaitState{
			Phase:           game.Phase,
			Kind:            model.WaitStateKind(rs.Kind),
			ActingPlayerIDs: rs.ActingPlayerIDs,
		}, nil

	case model.PhaseShakeUp:
		ids, sErr := computeShakeUpWaitees(ctx, q, gameID, &game)
		if sErr != nil {
			return model.WaitState{}, sErr
		}
		kind := model.WaitKindNobody
		if game.ShakeUpStep != nil {
			switch *game.ShakeUpStep {
			case gamepkg.ShakeUpStepRolling:
				kind = model.WaitKindShakeUpRolling
			case gamepkg.ShakeUpStepSpending:
				kind = model.WaitKindShakeUpSpending
			}
		}
		return model.WaitState{Phase: game.Phase, Kind: kind, ActingPlayerIDs: ids}, nil

	case model.PhaseEnded:
		return model.WaitState{Phase: game.Phase, Kind: model.WaitKindNobody}, nil
	}
	return model.WaitState{Phase: game.Phase, Kind: model.WaitKindNobody}, nil
}

// computeLobbyWaitees names the facilitator once the lobby has its minimum
// two players (per the lobby's own start-game gate) — before that, there's
// no one else to wait on, so nobody is named.
func computeLobbyWaitees(ctx context.Context, q *dbgen.Queries, gameID int64) ([]int64, error) {
	players, err := q.GetPlayersByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	if len(players) < 2 {
		return nil, nil
	}
	for _, p := range players {
		if p.IsFacilitator {
			return []int64{p.ID}, nil
		}
	}
	return nil, nil
}

// computePrologueWaitees ports the four modes PrologueView.svelte derives
// from game.prologue_ranking_step onto server facts:
//
//   - nil step               → choosing: the on-turn player (prologueTurnState).
//   - declare_X               → players who haven't marked themselves done
//     spending hearts on track X.
//   - place_set_asides_X      → track X's top-ranked real player.
//   - anything else (closing) → players who haven't marked themselves ready.
func computePrologueWaitees(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
	game *dbgen.Game,
) (model.WaitStateKind, []int64, error) {
	step := game.PrologueRankingStep
	switch {
	case step == nil:
		active, _, err := prologueTurnState(ctx, q, gameID)
		if err != nil {
			return "", nil, err
		}
		if active == nil {
			return model.WaitKindPrologueChoosing, nil, nil
		}
		return model.WaitKindPrologueChoosing, []int64{active.ID}, nil

	case strings.HasPrefix(*step, "declare_"):
		ids, err := prologueDeclareWaitees(ctx, q, gameID, trackForStep(*step))
		if err != nil {
			return "", nil, err
		}
		return model.WaitKindPrologueDeclare, ids, nil

	case strings.HasPrefix(*step, "place_set_asides_"):
		ids, err := prologuePlaceWaitees(ctx, q, gameID, trackForStep(*step))
		if err != nil {
			return "", nil, err
		}
		return model.WaitKindProloguePlaceSetAsides, ids, nil

	default: // closing
		ids, err := prologueClosingWaitees(ctx, q, gameID)
		if err != nil {
			return "", nil, err
		}
		return model.WaitKindPrologueClosing, ids, nil
	}
}

// prologueDeclareWaitees names every player who hasn't yet marked themselves
// done spending hearts on track, mirroring PrologueView.svelte's declare-mode
// `notDone` derivation.
func prologueDeclareWaitees(ctx context.Context, q *dbgen.Queries, gameID int64, track string) ([]int64, error) {
	players, err := q.GetPlayersByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	doneFlags, err := q.ListTrackDoneByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	done := make(map[int64]bool, len(doneFlags))
	for _, d := range doneFlags {
		if d.Track == track && d.Done {
			done[d.PlayerID] = true
		}
	}
	var ids []int64
	for _, p := range players {
		if !done[p.ID] {
			ids = append(ids, p.ID)
		}
	}
	return ids, nil
}

// prologuePlaceWaitees names track's top-ranked real player — the same
// dummy-rank-aware lookup PlaceSetAsides's auth check uses, via the shared
// gamepkg.TopOfTrackPlayer.
func prologuePlaceWaitees(ctx context.Context, q *dbgen.Queries, gameID int64, track string) ([]int64, error) {
	rankings, err := q.ListRankingsByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	top := gamepkg.TopOfTrackPlayer(rankingRows(rankings), track)
	if top == nil {
		return nil, nil
	}
	return []int64{*top}, nil
}

// prologueClosingWaitees names every player who hasn't yet marked themselves
// ready for the closing step, mirroring ClosingStage's not-ready derivation.
func prologueClosingWaitees(ctx context.Context, q *dbgen.Queries, gameID int64) ([]int64, error) {
	players, err := q.GetPlayersByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	readyRows, err := q.ListClosingReadyByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	ready := make(map[int64]bool, len(readyRows))
	for _, r := range readyRows {
		if r.Ready {
			ready[r.PlayerID] = true
		}
	}
	var ids []int64
	for _, p := range players {
		if !ready[p.ID] {
			ids = append(ids, p.ID)
		}
	}
	return ids, nil
}

// computeShakeUpWaitees names whoever the current shake-up step blocks on:
//
//   - Step 1 (rolling): the open roll's actor, checked FIRST — shakeUpNextRoller
//     can't be used alone here, since it treats "has a dice_rolls row" as
//     "already rolled" (correct once a roll has resolved, wrong while one is
//     still open) and would name the NEXT roller instead of the current one.
//     Only falls back to it in the brief gap before the next roll has opened.
//   - Step 2 (spending): an open spend's pending reactors (ruling 5) if any
//     are still outstanding; once every reactor has adjusted or passed, the
//     spender who must commit; with no open spend yet, whoever's turn it is
//     to announce (currentShakeUpActor).
//
// Extracted from GetShakeUp's inline composition so the snapshot endpoint and
// ComputeWaitState share one answer instead of two copies that could drift.
func computeShakeUpWaitees(ctx context.Context, q *dbgen.Queries, gameID int64, game *dbgen.Game) ([]int64, error) {
	if game.ShakeUpStep == nil || game.ShakeUpCategory == nil {
		return nil, nil
	}
	switch *game.ShakeUpStep {
	case gamepkg.ShakeUpStepRolling:
		if openRoll, err := q.GetOpenShakeUpRollByGame(ctx, gameID); err == nil {
			return []int64{openRoll.ActorID}, nil
		}
		roller, err := shakeUpNextRoller(ctx, q, gameID, *game.ShakeUpCategory)
		if err != nil {
			return nil, err
		}
		if roller == 0 {
			return nil, nil
		}
		return []int64{roller}, nil

	case gamepkg.ShakeUpStepSpending:
		if open, err := q.GetOpenShakeUpSpend(ctx, gameID); err == nil {
			pending, pErr := shakeUpPendingReactors(ctx, q, gameID, open)
			if pErr != nil {
				return nil, pErr
			}
			if len(pending) > 0 {
				return pending, nil
			}
			return []int64{open.PlayerID}, nil
		}
		actor, err := currentShakeUpActor(ctx, q, gameID, *game.ShakeUpCategory)
		if err != nil {
			return nil, err
		}
		if actor == 0 {
			return nil, nil
		}
		return []int64{actor}, nil
	}
	return nil, nil
}
