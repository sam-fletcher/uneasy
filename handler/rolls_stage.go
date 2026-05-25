package handler

// handler/rolls_stage.go — Stage machine internals for dice rolls.
//
// This file owns the participant/intent/ready state and the sweeps that
// keep it consistent. HTTP handlers in rolls.go call into these helpers;
// rolls_dice.go handles the math once the stage machine decides it's time
// to resolve.
//
// Stage transitions: decide_vote → voting → leverage → resolved. Clients
// never write the stage column directly — only the handlers below do, via
// advanceToLeverage or by ResolveDiceRoll (in finalizeRoll).

import (
	"context"
	"fmt"
	"net/http"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// seedRollParticipants inserts a dice_roll_participants row for every player
// in the game. Actor is created with intent='aid' (implicit); others with
// intent=NULL. Every participant who has no dice to commit (no unleveraged
// assets, no unspent banked dice) is auto-readied at seed — there is
// nothing for them to do, so blocking on their explicit Ready click would
// just be busywork.
func seedRollParticipants(
	ctx context.Context,
	q *dbgen.Queries,
	gameID, rollID, actorID int64,
) error {
	players, err := q.GetPlayersByGame(ctx, gameID)
	if err != nil {
		return fmt.Errorf("load players: %w", err)
	}
	aid := intentAid
	for _, p := range players {
		var intent *string
		if p.ID == actorID {
			intent = &aid
		}
		canCommit, err := playerCanCommit(ctx, q, gameID, p.ID)
		if err != nil {
			return err
		}
		if err := q.CreateRollParticipant(ctx, dbgen.CreateRollParticipantParams{
			RollID:   rollID,
			PlayerID: p.ID,
			Intent:   intent,
			IsReady:  !canCommit,
		}); err != nil {
			return fmt.Errorf("create participant: %w", err)
		}
	}
	return nil
}

// playerCanCommit returns true when the player has at least one
// unleveraged, non-destroyed asset OR one unspent banked die.
func playerCanCommit(ctx context.Context, q *dbgen.Queries, gameID, playerID int64) (bool, error) {
	assets, err := q.ListAssetsByOwner(ctx, playerID)
	if err != nil {
		return false, fmt.Errorf("list assets: %w", err)
	}
	for _, a := range assets {
		if !a.IsDestroyed && !a.IsLeveraged {
			return true, nil
		}
	}
	count, err := q.CountUnspentBankedDiceByPlayer(ctx, dbgen.CountUnspentBankedDiceByPlayerParams{
		GameID:   gameID,
		PlayerID: playerID,
	})
	if err != nil {
		return false, fmt.Errorf("count banked dice: %w", err)
	}
	return count > 0, nil
}

// runAutoReadySweep flips the given player to is_ready=true if they have no
// dice left to commit. Broadcasts roll.ready_changed with forced=true when
// it triggers. No-op if the player is already ready or still has dice.
func runAutoReadySweep(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	roll *dbgen.DiceRoll,
	playerID int64,
) error {
	part, err := q.GetParticipant(ctx, dbgen.GetParticipantParams{
		RollID: roll.ID, PlayerID: playerID,
	})
	if err != nil {
		return err
	}
	if part.IsReady {
		return nil
	}
	can, err := playerCanCommit(ctx, q, roll.GameID, playerID)
	if err != nil {
		return err
	}
	if can {
		return nil
	}
	if err := q.SetParticipantReady(ctx, dbgen.SetParticipantReadyParams{
		RollID: roll.ID, PlayerID: playerID, IsReady: true,
	}); err != nil {
		return err
	}
	broadcastEvent(manager, roll.GameID, model.EventRollReadyChanged, model.RollReadyChangedPayload{
		RollID:   roll.ID,
		PlayerID: playerID,
		IsReady:  true,
		Forced:   true,
		Reason:   "no_dice_remaining",
	})
	return nil
}

// runAutoUnreadySweep flips every opposing-side participant who is currently
// ready AND still has dice to commit back to is_ready=false. "Opposing" is
// determined by the just-committed die's interference flag.
func runAutoUnreadySweep(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	roll *dbgen.DiceRoll,
	committedIsInterference bool,
) error {
	participants, err := q.ListParticipantsByRoll(ctx, roll.ID)
	if err != nil {
		return err
	}
	// Aid commit (is_interference=false) un-readies interferers.
	// Interfere commit (is_interference=true) un-readies aiders (incl. actor).
	opposingIntent := intentInterfere
	if committedIsInterference {
		opposingIntent = intentAid
	}
	for _, p := range participants {
		if !p.IsReady {
			continue
		}
		// Locked-ready (no dice) participants stay ready — can't commit anyway.
		can, err := playerCanCommit(ctx, q, roll.GameID, p.PlayerID)
		if err != nil {
			return err
		}
		if !can {
			continue
		}
		// Actor counts as aid; non-actors must have explicit matching intent.
		isOpposing := false
		if opposingIntent == intentAid {
			if p.PlayerID == roll.ActorID {
				isOpposing = true
			} else if p.Intent != nil && *p.Intent == intentAid {
				isOpposing = true
			}
		} else if p.Intent != nil && *p.Intent == intentInterfere {
			isOpposing = true
		}
		if !isOpposing {
			continue
		}
		if err := q.SetParticipantReady(ctx, dbgen.SetParticipantReadyParams{
			RollID: roll.ID, PlayerID: p.PlayerID, IsReady: false,
		}); err != nil {
			return err
		}
		broadcastEvent(manager, roll.GameID, model.EventRollReadyChanged, model.RollReadyChangedPayload{
			RollID:   roll.ID,
			PlayerID: p.PlayerID,
			IsReady:  false,
			Forced:   true,
			Reason:   "opposing_leverage",
		})
	}
	return nil
}

// maybeAutoResolve rolls and resolves the roll if every participant is ready.
// A roll with zero participant rows is treated as malformed (it predates the
// stage-machine migration, or seeding failed) and is left alone — otherwise
// the unready count would be 0 vacuously and the next Ready click would
// auto-resolve a roll nobody has actually committed to.
func maybeAutoResolve(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
	manager *hub.Manager,
	roll *dbgen.DiceRoll,
) error {
	participants, err := q.ListParticipantsByRoll(ctx, roll.ID)
	if err != nil {
		return err
	}
	if len(participants) == 0 {
		return nil
	}
	for _, p := range participants {
		if !p.IsReady {
			return nil
		}
	}
	return finalizeRoll(ctx, w, r, q, manager, roll)
}

// advanceToLeverage sets stage='leverage', broadcasts the stage change,
// runs the skip-leverage short-circuit if no one can commit anything,
// then runs maybeAutoResolve.
func advanceToLeverage(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
	manager *hub.Manager,
	roll *dbgen.DiceRoll,
) error {
	if err := q.SetDiceRollStage(ctx, dbgen.SetDiceRollStageParams{
		ID: roll.ID, Stage: stageLeverage,
	}); err != nil {
		return err
	}
	roll.Stage = stageLeverage
	broadcastEvent(manager, roll.GameID, model.EventRollStageChanged, model.RollStageChangedPayload{
		RollID: roll.ID, Stage: stageLeverage,
	})

	// Skip-leverage short-circuit: if nobody has anything to commit, force
	// everyone ready, emit a Minor chat log, and resolve immediately.
	participants, err := q.ListParticipantsByRoll(ctx, roll.ID)
	if err != nil {
		return err
	}
	anyCanCommit := false
	for _, p := range participants {
		can, err := playerCanCommit(ctx, q, roll.GameID, p.PlayerID)
		if err != nil {
			return err
		}
		if can {
			anyCanCommit = true
			break
		}
	}
	if !anyCanCommit {
		if err := q.SetAllParticipantsReady(ctx, roll.ID); err != nil {
			return err
		}
		EmitRollSkipLeverage(ctx, q, manager, roll)
	}

	return maybeAutoResolve(ctx, w, r, q, manager, roll)
}

// postCommitSweeps runs the auto-unready / auto-ready sweeps and triggers
// auto-resolution if appropriate. Called by commitDie after any successful
// commit (asset leverage or banked-die spend).
func postCommitSweeps(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
	manager *hub.Manager,
	roll *dbgen.DiceRoll,
	committerID int64,
	committedIsInterference bool,
) error {
	if err := runAutoUnreadySweep(ctx, q, manager, roll, committedIsInterference); err != nil {
		return err
	}
	if err := runAutoReadySweep(ctx, q, manager, roll, committerID); err != nil {
		return err
	}
	return maybeAutoResolve(ctx, w, r, q, manager, roll)
}

// commitDie is the shared tail of LeverageRoll and UseBankedDie. The source
// has already been validated and its mark-as-used side effects applied; this
// helper creates the die row, broadcasts roll.leverage_added, writes the
// Minor chat log, and runs the post-commit sweeps (including auto-resolve).
//
// leveragedAssetID is nil for banked-die spends; assetNameForLog is nil for
// banked-die spends (the chat log substitutes "banked die" in that case).
func commitDie(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
	manager *hub.Manager,
	roll *dbgen.DiceRoll,
	player *dbgen.Player,
	isInterference bool,
	leveragedAssetID *int64,
	assetNameForLog *string,
) (dbgen.DiceRollDice, bool) {
	die, err := q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
		RollID:           roll.ID,
		PlayerID:         player.ID,
		IsInterference:   isInterference,
		LeveragedAssetID: leveragedAssetID,
	})
	if err != nil {
		respondInternalErr(w, r, "could not add die to roll", err)
		return dbgen.DiceRollDice{}, false
	}
	var assetIDForEvent int64
	if leveragedAssetID != nil {
		assetIDForEvent = *leveragedAssetID
	}
	broadcastEvent(manager, roll.GameID, model.EventRollLeverageAdded, model.RollLeverageAddedPayload{
		RollID:         roll.ID,
		PlayerID:       player.ID,
		AssetID:        assetIDForEvent,
		IsInterference: isInterference,
	})
	EmitRollCommit(ctx, q, manager, roll, player, isInterference, assetNameForLog)
	if err := postCommitSweeps(ctx, w, r, q, manager, roll, player.ID, isInterference); err != nil {
		respondInternalErr(w, r, "could not run sweeps", err)
		return dbgen.DiceRollDice{}, false
	}
	return die, true
}
