package handler

// handler/shake_up.go — Shake-Up endpoints (Phase 4c).
//
// Lifecycle:
//
//   BeginShakeUp (called from the row-13 / endgame trigger) →
//     phase=shake_up, category=esteem, step=1, all tokens zeroed.
//
//   For each category in [esteem, knowledge, power]:
//     Step 1 (rolling) — real dice rolls through the existing stage machine
//       (shakeUpOpenRollForRoller + the /api/rolls/:id/leverage + /ready
//       endpoints), one player at a time in reverse-rank turn order. Each
//       resolution grants tokens = distinct faces (finalizeShakeUpRoll) and
//       opens the next roller's roll; after the last player rolls, server
//       advances to step 2.
//     Step 2 (spending) — players take turns in reverse rank order.
//       On their turn:
//         - announce a spend (creates shake_up_spends, base_cost=1, tokens
//           charged immediately for the spender);
//         - other players may post ±1 adjustments via ShakeUpAdjust (each
//           costs 1 token from the bidder's pool);
//         - spender commits via ShakeUpCommit, which locks final_cost and
//           applies the mechanical effect.
//       The category advances once every player's pool reaches 0.
//
//   After (power, 2): phase=ended, final rankings recorded.
//
// Cost-adjustment model: synchronous, no server timer. The spender's
// initial 1-token cost is paid at announce time. Adjusters can submit
// nudges any time before the spender hits commit. This is the "play-by-
// post"-friendly version of the rulebook's adjustment window — the spirit
// of the rule ("once you commit, you must spend regardless of changes")
// is preserved because the announce step locks the spender in.

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// ── Snapshot ─────────────────────────────────────────────────────────────────

// GetShakeUp handles GET /api/tables/{id}/shake-up.
//
// Returns the current state machine, each player's token pool, and the open
// spend (if any) with its accumulated adjustments.
func GetShakeUp(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		ctx := r.Context()
		game, err := s.Q.GetGameByID(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}
		tokens, err := s.Q.ListShakeUpTokensByGame(ctx, gameID)
		if err != nil {
			respondInternalErr(w, r, "could not load tokens", err)
			return
		}

		out := map[string]any{
			"phase":             game.Phase,
			"shake_up_category": game.ShakeUpCategory,
			"shake_up_step":     game.ShakeUpStep,
			"tokens":            tokens,
			"options":           shakeUpOptionsForGame(game),
		}

		// The "Claim a new title" option (Power category) offers every title not
		// already claimed game-wide (ADR-007). Compute it server-side so the picker
		// can't offer a taken title.
		if claimable, cerr := claimableTitles(ctx, s.Q, gameID); cerr == nil {
			out["claimable_titles"] = claimable
		}

		// "Who must act" — single source of truth shared with ComputeWaitState
		// (handler/wait_state.go), so this snapshot and the notification
		// ticker never compute two different answers.
		waitees, err := computeShakeUpWaitees(ctx, s.Q, gameID, &game)
		if err != nil {
			respondInternalErr(w, r, "could not compute waitees", err)
			return
		}

		// Open spend (if any). While a spend is open no one may announce, so
		// current_actor is only meaningful when the spending step is awaiting
		// the next announce.
		open, err := s.Q.GetOpenShakeUpSpend(ctx, gameID)
		if err == nil {
			adj, _ := s.Q.ListAdjustmentsForSpend(ctx, open.ID)
			passes, _ := s.Q.ListPassesForSpend(ctx, open.ID)
			pending, perr := shakeUpPendingReactors(ctx, s.Q, gameID, open)
			if perr != nil {
				pending = []int64{}
			}
			out["open_spend"] = map[string]any{
				"spend":               open,
				"adjustments":         adj,
				"passes":              passes,
				"pending_reactor_ids": pending,
				"commit_ready":        len(pending) == 0,
			}
		} else if game.ShakeUpStep != nil && *game.ShakeUpStep == gamepkg.ShakeUpStepSpending &&
			game.ShakeUpCategory != nil && len(waitees) == 1 {
			out["current_actor"] = waitees[0]
		}

		// Step 1 (rolling): who's up, and the open roll id if the client wants to
		// fetch its full state (dice, participants) via getActiveRollForGame/getRoll.
		if game.ShakeUpStep != nil && *game.ShakeUpStep == gamepkg.ShakeUpStepRolling &&
			game.ShakeUpCategory != nil {
			if openRoll, rerr := s.Q.GetOpenShakeUpRollByGame(ctx, gameID); rerr == nil {
				out["open_roll_id"] = openRoll.ID
			}
			if len(waitees) == 1 {
				out["current_roller_id"] = waitees[0]
			}
		}
		respond(w, http.StatusOK, out)
	}
}

func shakeUpOptionsForGame(game dbgen.Game) []gamepkg.ShakeUpOptionInfo {
	if game.ShakeUpCategory == nil {
		return nil
	}
	return gamepkg.ShakeUpOptionsForCategory(*game.ShakeUpCategory)
}

// claimableTitleInfo is one title the Shake-Up "Claim a new title" picker may
// offer: a stable id, its display name + description, and whether it sits in the
// line of succession (so the UI can flag throne-line titles with a crown).
type claimableTitleInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	InSuccession bool   `json:"in_succession"`
}

// claimableTitles returns every title not yet claimed anywhere in the game, in
// the titles-sheet order, for the claim-title picker.
func claimableTitles(ctx context.Context, q *dbgen.Queries, gameID int64) ([]claimableTitleInfo, error) {
	claimedIDs, err := q.ListClaimedTitleIDsByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	claimed := make(map[string]bool, len(claimedIDs))
	for _, c := range claimedIDs {
		if c != nil {
			claimed[*c] = true
		}
	}
	out := make([]claimableTitleInfo, 0, len(gamepkg.TitlesSheet()))
	for _, t := range gamepkg.TitlesSheet() {
		if claimed[t.ID] {
			continue
		}
		_, inLine := gamepkg.SuccessionRank(t.ID)
		out = append(out, claimableTitleInfo{
			ID:           t.ID,
			Name:         t.Name,
			Description:  t.Description,
			InSuccession: inLine,
		})
	}
	return out, nil
}

// ── Begin trigger ────────────────────────────────────────────────────────────

// BeginShakeUp transitions a game from main_event into shake_up. Idempotent
// for callers that double-fire on the same trigger.
func BeginShakeUp(ctx context.Context, q *dbgen.Queries, manager *hub.Manager, gameID int64) error {
	game, err := q.GetGameByID(ctx, gameID)
	if err != nil {
		return fmt.Errorf("load game: %w", err)
	}
	if game.Phase == model.PhaseShakeUp || game.Phase == model.PhaseEnded {
		return nil
	}
	if game.Phase != model.PhaseMainEvent {
		return errors.New("shake-up can only begin from main_event")
	}
	if err = q.RefreshAllAssets(ctx, gameID); err != nil {
		return fmt.Errorf("refresh assets: %w", err)
	}
	if err = q.ZeroShakeUpTokens(ctx, gameID); err != nil {
		return fmt.Errorf("zero tokens: %w", err)
	}
	cat := gamepkg.ShakeUpCategoryEsteem
	step := gamepkg.ShakeUpStepRolling
	err = q.SetShakeUpStep(ctx, dbgen.SetShakeUpStepParams{
		ID: gameID, ShakeUpCategory: &cat, ShakeUpStep: &step,
	})
	if err != nil {
		return fmt.Errorf("set initial step: %w", err)
	}
	err = q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: gameID, Phase: model.PhaseShakeUp,
	})
	if err != nil {
		return fmt.Errorf("set phase: %w", err)
	}
	broadcastPhaseChange(ctx, q, manager, gameID, model.PhaseShakeUp)
	broadcastEvent(manager, gameID, model.EventShakeUpStepChanged, model.ShakeUpStepChangedPayload{
		Category: cat, Step: step,
	})

	roller, err := shakeUpNextRoller(ctx, q, gameID, cat)
	if err != nil {
		return fmt.Errorf("determine first roller: %w", err)
	}
	if roller != 0 {
		if err := shakeUpOpenRollForRoller(ctx, q, manager, gameID, cat, roller); err != nil {
			return fmt.Errorf("create first roll: %w", err)
		}
	}
	return nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func inShakeUpStep(w http.ResponseWriter, game *dbgen.Game, want int16) bool {
	if game.Phase != model.PhaseShakeUp {
		respondErr(w, http.StatusConflict, "game is not in the shake-up phase")
		return false
	}
	if game.ShakeUpStep == nil || *game.ShakeUpStep != want {
		respondErr(w, http.StatusConflict, "wrong shake-up step")
		return false
	}
	return true
}

// maybeAdvanceShakeUpCategory checks whether every player has 0 tokens. If
// so, advances to the next category's step 1 (or ends the game if power
// just finished).
func maybeAdvanceShakeUpCategory(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
) error {
	tokens, err := q.ListShakeUpTokensByGame(ctx, gameID)
	if err != nil {
		return fmt.Errorf("load tokens: %w", err)
	}
	for _, t := range tokens {
		if t.ShakeUpTokens > 0 {
			return nil // not all empty yet
		}
	}
	game, err := q.GetGameByID(ctx, gameID)
	if err != nil {
		return fmt.Errorf("load game: %w", err)
	}
	if game.ShakeUpCategory == nil {
		return nil
	}
	next := gamepkg.NextShakeUpCategory(*game.ShakeUpCategory)
	if next == "" {
		// Power just finished — end the game.
		if err = q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
			ID: gameID, Phase: model.PhaseEnded,
		}); err != nil {
			return fmt.Errorf("end game: %w", err)
		}
		broadcastEvent(manager, gameID, model.EventShakeUpEnded, model.ShakeUpEndedPayload{})
		broadcastPhaseChange(ctx, q, manager, gameID, model.PhaseEnded)
		EmitShakeUpEnded(ctx, q, manager, gameID)
		return nil
	}
	step := gamepkg.ShakeUpStepRolling
	if err = q.SetShakeUpStep(ctx, dbgen.SetShakeUpStepParams{
		ID: gameID, ShakeUpCategory: &next, ShakeUpStep: &step,
	}); err != nil {
		return fmt.Errorf("advance category: %w", err)
	}
	broadcastEvent(manager, gameID, model.EventShakeUpStepChanged,
		model.ShakeUpStepChangedPayload{Category: next, Step: step})
	EmitShakeUpCategory(ctx, q, manager, gameID, next)

	roller, err := shakeUpNextRoller(ctx, q, gameID, next)
	if err != nil {
		return fmt.Errorf("determine first roller: %w", err)
	}
	if roller != 0 {
		if err := shakeUpOpenRollForRoller(ctx, q, manager, gameID, next, roller); err != nil {
			return fmt.Errorf("create first roll: %w", err)
		}
	}
	return nil
}
