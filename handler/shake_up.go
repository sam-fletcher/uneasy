package handler

// handler/shake_up.go — Shake-Up endpoints (Phase 4c).
//
// Lifecycle:
//
//   BeginShakeUp (called from the row-13 / endgame trigger) →
//     phase=shake_up, category=esteem, step=1, all tokens zeroed.
//
//   For each category in [esteem, knowledge, power]:
//     Step 1 (rolling) — every player calls ShakeUpRoll once. After the
//       last player rolls, server advances to step 2.
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
	"encoding/json"
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

		// Open spend (if any). While a spend is open no one may announce, so
		// current_actor is only meaningful when the spending step is awaiting
		// the next announce.
		open, err := s.Q.GetOpenShakeUpSpend(ctx, gameID)
		if err == nil {
			adj, _ := s.Q.ListAdjustmentsForSpend(ctx, open.ID)
			out["open_spend"] = map[string]any{
				"spend":       open,
				"adjustments": adj,
			}
		} else if game.ShakeUpStep != nil && *game.ShakeUpStep == gamepkg.ShakeUpStepSpending &&
			game.ShakeUpCategory != nil {
			if actor, aerr := currentShakeUpActor(ctx, s.Q, gameID, *game.ShakeUpCategory); aerr == nil && actor != 0 {
				out["current_actor"] = actor
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
	EmitShakeUpBegin(ctx, q, manager, gameID, cat)
	return nil
}

// ── Step 1: rolling ──────────────────────────────────────────────────────────

// ShakeUpRoll handles POST /api/tables/{id}/shake-up/roll.
//
// The caller rolls dice (own assets only — leverage selection lives in the
// body, mirroring plan rolls). The integer result is added to the caller's
// shake_up_tokens. After the last player rolls, the server advances to
// step 2 of the current category.
func ShakeUpRoll(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		ctx := r.Context()
		game, err := s.Q.GetGameByID(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}
		if !inShakeUpStep(w, &game, gamepkg.ShakeUpStepRolling) {
			return
		}

		var body struct {
			Result int16 `json:"result"` // sum of dice including any leverage
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil || body.Result < 1 {
			respondErr(w, http.StatusBadRequest, "result must be a positive integer")
			return
		}

		// Idempotency: if the player already has a shake-up roll for the
		// current category, reject. We use dice_rolls.is_shake_up + actor_id
		// + a per-category check derived from created_at order; simplest:
		// only one per (game, actor) per category — enforced via a quick
		// sentinel using shake_up_tokens (zero for the category at start).
		// For now, allow re-roll only when the player has 0 tokens AND the
		// step is still 1 (i.e. they haven't rolled yet *this category*).
		if player.ShakeUpTokens > 0 {
			respondErr(w, http.StatusConflict, "you have already rolled for this category")
			return
		}

		// Persist a dice_rolls row for the audit trail.
		_, err = s.Q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
			GameID:     gameID,
			ActorID:    player.ID,
			Difficulty: 0,
			Stage:      "leverage",
		})
		if err != nil {
			respondInternalErr(w, r, "could not persist roll", err)
			return
		}
		newTotal, err := s.Q.AddShakeUpTokens(ctx, dbgen.AddShakeUpTokensParams{
			ID: player.ID, ShakeUpTokens: body.Result,
		})
		if err != nil {
			respondInternalErr(w, r, "could not add tokens", err)
			return
		}
		broadcastEvent(manager, gameID, model.EventShakeUpRolled, model.ShakeUpRolledPayload{
			PlayerID: player.ID, Result: body.Result, Total: newTotal,
		})
		EmitShakeUpRolled(ctx, s.Q, manager, gameID, player.ID, body.Result, newTotal, *game.ShakeUpCategory)

		// Advance to step 2 once everyone has rolled.
		players, err := s.Q.GetPlayersByGame(ctx, gameID)
		if err != nil {
			respondInternalErr(w, r, "could not load players", err)
			return
		}
		allRolled := true
		for _, p := range players {
			fresh, err := s.Q.GetShakeUpTokens(ctx, p.ID)
			if err != nil || fresh == 0 {
				allRolled = false
				break
			}
		}
		if allRolled {
			cat := *game.ShakeUpCategory
			step := gamepkg.ShakeUpStepSpending
			err = s.Q.SetShakeUpStep(ctx, dbgen.SetShakeUpStepParams{
				ID: gameID, ShakeUpCategory: &cat, ShakeUpStep: &step,
			})
			if err != nil {
				respondInternalErr(w, r, "could not advance step", err)
				return
			}
			broadcastEvent(manager, gameID, model.EventShakeUpStepChanged,
				model.ShakeUpStepChangedPayload{Category: cat, Step: step})
		}

		respond(w, http.StatusOK, map[string]any{"tokens": newTotal})
	}
}

// ── Step 2: announce / adjust / commit ───────────────────────────────────────

// ShakeUpAnnounce handles POST /api/tables/{id}/shake-up/spend.
//
// Body: {"option_key", "target_asset_id"?, "target_player_id"?}. Creates an
// open spend, deducts 1 token from the spender. No effect is applied yet —
// other players may submit adjustments until the spender commits.
func ShakeUpAnnounce(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		ctx := r.Context()
		game, err := s.Q.GetGameByID(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}
		if !inShakeUpStep(w, &game, gamepkg.ShakeUpStepSpending) {
			return
		}

		var body struct {
			OptionKey      string `json:"option_key"`
			TargetAssetID  *int64 `json:"target_asset_id"`
			TargetPlayerID *int64 `json:"target_player_id"`
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		info, err := gamepkg.ShakeUpOption(body.OptionKey)
		if err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if info.Category != *game.ShakeUpCategory {
			respondErr(w, http.StatusBadRequest, "option does not belong to the current category")
			return
		}
		if info.NeedsAsset && body.TargetAssetID == nil {
			respondErr(w, http.StatusBadRequest, "target_asset_id required for "+info.Key)
			return
		}

		// Refuse a second open spend.
		if open, err := s.Q.GetOpenShakeUpSpend(ctx, gameID); err == nil {
			respondErr(w, http.StatusConflict, fmt.Sprintf("an open spend exists (#%d) — commit it first", open.ID))
			return
		}

		// Enforce reverse-rank turn order (lowest status first, looping past
		// players who are out of tokens). Per SHAKEUP_RULES.md.
		actor, err := currentShakeUpActor(ctx, s.Q, gameID, *game.ShakeUpCategory)
		if err != nil {
			respondInternalErr(w, r, "could not determine turn", err)
			return
		}
		if actor != player.ID {
			respondErr(w, http.StatusConflict, "it is not your turn to spend")
			return
		}

		// Spender pays the 1-token base cost immediately.
		if player.ShakeUpTokens < 1 {
			respondErr(w, http.StatusConflict, "you have no tokens to spend")
			return
		}
		_, err = s.Q.SubShakeUpTokens(ctx, dbgen.SubShakeUpTokensParams{
			ID: player.ID, ShakeUpTokens: 1,
		})
		if err != nil {
			respondInternalErr(w, r, "could not deduct token", err)
			return
		}

		spend, err := s.Q.CreateShakeUpSpend(ctx, dbgen.CreateShakeUpSpendParams{
			GameID:         gameID,
			PlayerID:       player.ID,
			Category:       info.Category,
			OptionKey:      info.Key,
			TargetAssetID:  body.TargetAssetID,
			TargetPlayerID: body.TargetPlayerID,
			BaseCost:       1,
		})
		if err != nil {
			respondInternalErr(w, r, "could not create spend", err)
			return
		}
		broadcastEvent(manager, gameID, model.EventShakeUpSpendOpened, model.ShakeUpSpendOpenedPayload{
			Spend: spend,
		})
		var targetName string
		if body.TargetAssetID != nil {
			if a, aErr := s.Q.GetAssetByID(ctx, *body.TargetAssetID); aErr == nil {
				targetName = a.Name
			}
		}
		EmitShakeUpAnnounced(ctx, s.Q, manager, gameID, spend, shakeUpOptionPhrase(info.Description), targetName)
		respond(w, http.StatusOK, map[string]any{"spend": spend})
	}
}

// ShakeUpAdjust handles POST /api/tables/{id}/shake-up/adjust.
//
// Body: {"spend_id", "adjustment": ±1}. Anyone with tokens may bid; each
// adjustment costs the bidder 1 token, deducted at insert.
func ShakeUpAdjust(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		var body struct {
			SpendID    int64 `json:"spend_id"`
			Adjustment int16 `json:"adjustment"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Adjustment != 1 && body.Adjustment != -1 {
			respondErr(w, http.StatusBadRequest, "adjustment must be +1 or -1")
			return
		}
		ctx := r.Context()
		spend, err := s.Q.GetShakeUpSpend(ctx, body.SpendID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "spend not found")
			return
		}
		if spend.GameID != gameID || spend.CommittedAt.Valid {
			respondErr(w, http.StatusConflict, "spend is not open in this table")
			return
		}
		if spend.PlayerID == player.ID {
			respondErr(w, http.StatusForbidden, "the spender cannot adjust their own spend")
			return
		}
		if player.ShakeUpTokens < 1 {
			respondErr(w, http.StatusConflict, "you have no tokens to adjust with")
			return
		}
		_, err = s.Q.SubShakeUpTokens(ctx, dbgen.SubShakeUpTokensParams{
			ID: player.ID, ShakeUpTokens: 1,
		})
		if err != nil {
			respondInternalErr(w, r, "could not deduct token", err)
			return
		}
		adj, err := s.Q.CreateShakeUpAdjustment(ctx, dbgen.CreateShakeUpAdjustmentParams{
			SpendID: spend.ID, PlayerID: player.ID, Adjustment: body.Adjustment,
		})
		if err != nil {
			respondInternalErr(w, r, "could not record adjustment", err)
			return
		}
		broadcastEvent(manager, gameID, model.EventShakeUpAdjusted, model.ShakeUpAdjustedPayload{
			SpendID: spend.ID, PlayerID: player.ID, Adjustment: body.Adjustment, AdjustmentID: adj.ID,
		})
		var optionPhrase string
		if info, oErr := gamepkg.ShakeUpOption(spend.OptionKey); oErr == nil {
			optionPhrase = shakeUpOptionPhrase(info.Description)
		}
		EmitShakeUpAdjusted(ctx, s.Q, manager, gameID, spend, player.ID, body.Adjustment, optionPhrase)
		respond(w, http.StatusOK, map[string]any{"adjustment": adj})
	}
}

// ShakeUpCommit handles POST /api/tables/{id}/shake-up/commit.
//
// Body: {"spend_id"}. Caller must be the spender. Locks final_cost,
// applies the mechanical effect, advances the category if everyone is now
// at zero tokens.
func ShakeUpCommit(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		var body struct {
			SpendID int64 `json:"spend_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		ctx := r.Context()
		spend, err := s.Q.GetShakeUpSpend(ctx, body.SpendID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "spend not found")
			return
		}
		if spend.GameID != gameID || spend.CommittedAt.Valid {
			respondErr(w, http.StatusConflict, "spend is not open in this table")
			return
		}
		if spend.PlayerID != player.ID {
			respondErr(w, http.StatusForbidden, "only the spender can commit")
			return
		}

		adjTotal, err := s.Q.SumAdjustmentsForSpend(ctx, spend.ID)
		if err != nil {
			respondInternalErr(w, r, "could not total adjustments", err)
			return
		}
		// Final cost = base + adjustments, floored at 0. The spender already
		// paid base_cost at announce; charge the *delta* now (adjTotal can be
		// negative or positive). If the delta would put them negative, only
		// take what they have — the rule is "must spend regardless", so the
		// spend goes through, but the spender can't go below 0 tokens.
		extra := min(int(adjTotal), int(player.ShakeUpTokens))
		if extra > 0 {
			if _, err = s.Q.SubShakeUpTokens(ctx, dbgen.SubShakeUpTokensParams{
				ID: player.ID, ShakeUpTokens: int16(extra),
			}); err != nil {
				respondInternalErr(w, r, "could not deduct adjusted cost", err)
				return
			}
		} else if extra < 0 {
			// Negative adjustment refunds — never below 0 in practice
			// because base_cost = 1 and -1 brings it to 0.
			refund := int16(-extra)
			if _, err = s.Q.AddShakeUpTokens(ctx, dbgen.AddShakeUpTokensParams{
				ID: player.ID, ShakeUpTokens: refund,
			}); err != nil {
				respondInternalErr(w, r, "could not refund adjustment", err)
				return
			}
		}
		finalCost := max(int16(int(spend.BaseCost)+int(adjTotal)), 0)
		if err = s.Q.CommitShakeUpSpend(ctx, dbgen.CommitShakeUpSpendParams{
			ID: spend.ID, FinalCost: &finalCost,
		}); err != nil {
			respondInternalErr(w, r, "could not commit spend", err)
			return
		}

		// Apply the mechanical effect (which also emits the committed log post).
		if err = applyShakeUpEffect(ctx, s.Q, manager, gameID, &spend, finalCost); err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		broadcastEvent(manager, gameID, model.EventShakeUpSpendCommitted, model.ShakeUpSpendCommittedPayload{
			SpendID: spend.ID, FinalCost: finalCost,
		})

		// Maybe advance the category.
		if err = maybeAdvanceShakeUpCategory(ctx, s.Q, manager, gameID); err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		respond(w, http.StatusOK, map[string]any{"spend_id": spend.ID, "final_cost": finalCost})
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// currentShakeUpActor returns the player whose turn it is to announce a spend
// in the current spending step, per reverse-rank order (lowest status first,
// looping, skipping token-less players). Returns 0 if no one holds tokens.
func currentShakeUpActor(ctx context.Context, q *dbgen.Queries, gameID int64, category string) (int64, error) {
	rankings, err := q.ListRankingsByGame(ctx, gameID)
	if err != nil {
		return 0, fmt.Errorf("load rankings: %w", err)
	}
	rows := make([]gamepkg.RankingRow, 0, len(rankings))
	for _, rk := range rankings {
		rows = append(rows, gamepkg.RankingRow{
			PlayerID: rk.PlayerID, Category: string(rk.Category), Rank: rk.Rank,
		})
	}
	order := gamepkg.ShakeUpTurnOrder(category, rows)

	tokens, err := q.ListShakeUpTokensByGame(ctx, gameID)
	if err != nil {
		return 0, fmt.Errorf("load tokens: %w", err)
	}
	hasTokens := make(map[int64]bool, len(tokens))
	for _, t := range tokens {
		hasTokens[t.ID] = t.ShakeUpTokens > 0
	}

	var lastActor *int64
	if last, lerr := q.GetLastCommittedShakeUpSpend(ctx, dbgen.GetLastCommittedShakeUpSpendParams{
		GameID: gameID, Category: category,
	}); lerr == nil {
		pid := last.PlayerID
		lastActor = &pid
	}
	return gamepkg.NextShakeUpActor(order, hasTokens, lastActor), nil
}

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
	return nil
}

// applyShakeUpEffect dispatches the option's mechanical effect.
func applyShakeUpEffect(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	spend *dbgen.ShakeUpSpend,
	finalCost int16,
) error {
	info, err := gamepkg.ShakeUpOption(spend.OptionKey)
	if err != nil {
		return err
	}
	switch spend.OptionKey {
	case gamepkg.ShakeUpOptTakePeer, gamepkg.ShakeUpOptTakeArtifact,
		gamepkg.ShakeUpOptTakeResource, gamepkg.ShakeUpOptTakeHolding:
		return shakeUpTakeAsset(ctx, q, manager, gameID, spend, expectedTakeType(spend.OptionKey), finalCost)
	case gamepkg.ShakeUpOptBreakResource, gamepkg.ShakeUpOptBreakHolding,
		gamepkg.ShakeUpOptBreakPeer, gamepkg.ShakeUpOptBreakArtifact:
		return shakeUpBreakAsset(ctx, q, manager, gameID, spend, expectedBreakType(spend.OptionKey), finalCost)
	case gamepkg.ShakeUpOptBumpEsteem, gamepkg.ShakeUpOptBumpKnowledge, gamepkg.ShakeUpOptBumpPower:
		return shakeUpBumpRank(ctx, q, manager, gameID, spend, info.BumpsTrack, finalCost)
	case gamepkg.ShakeUpOptClaimTitle:
		return shakeUpClaimTitle(ctx, q, manager, gameID, spend, finalCost)
	}
	return errors.New("no applier for option")
}

func expectedTakeType(opt string) model.AssetType {
	switch opt {
	case gamepkg.ShakeUpOptTakePeer:
		return model.AssetPeer
	case gamepkg.ShakeUpOptTakeArtifact:
		return model.AssetArtifact
	case gamepkg.ShakeUpOptTakeResource:
		return model.AssetResource
	case gamepkg.ShakeUpOptTakeHolding:
		return model.AssetHolding
	}
	return ""
}

func expectedBreakType(opt string) model.AssetType {
	switch opt {
	case gamepkg.ShakeUpOptBreakResource:
		return model.AssetResource
	case gamepkg.ShakeUpOptBreakHolding:
		return model.AssetHolding
	case gamepkg.ShakeUpOptBreakPeer:
		return model.AssetPeer
	case gamepkg.ShakeUpOptBreakArtifact:
		return model.AssetArtifact
	}
	return ""
}

func shakeUpTakeAsset(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	spend *dbgen.ShakeUpSpend,
	want model.AssetType,
	finalCost int16,
) error {
	if spend.TargetAssetID == nil {
		return errors.New("target_asset_id required")
	}
	asset, err := q.GetAssetByID(ctx, *spend.TargetAssetID)
	if err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}
	if asset.AssetType != want {
		return fmt.Errorf("target must be a %s asset", want)
	}
	if asset.IsDestroyed {
		return errors.New("asset is destroyed")
	}
	oldOwner := asset.OwnerID
	if _, err = takeAssetEffect(ctx, q, manager, gameID, asset.ID, oldOwner, spend.PlayerID); err != nil {
		return fmt.Errorf("transfer: %w", err)
	}
	spender := playerDisplayName(ctx, q, spend.PlayerID)
	from := playerDisplayName(ctx, q, oldOwner)
	EmitShakeUpCommitted(
		ctx,
		q,
		manager,
		gameID,
		spend,
		finalCost,
		fmt.Sprintf(
			"%s spent %d token(s) to take %s (%s) from %s",
			spender,
			finalCost,
			assetMark(asset.Name),
			want,
			from,
		),
		map[string]any{"effect": "take", "asset_id": asset.ID, "old_owner_id": oldOwner},
	)
	return nil
}

func shakeUpBreakAsset(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	spend *dbgen.ShakeUpSpend,
	want model.AssetType,
	finalCost int16,
) error {
	if spend.TargetAssetID == nil {
		return errors.New("target_asset_id required")
	}
	asset, err := q.GetAssetByID(ctx, *spend.TargetAssetID)
	if err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}
	if asset.AssetType != want {
		return fmt.Errorf("target must be a %s asset", want)
	}
	if asset.IsDestroyed {
		return errors.New("asset is already destroyed")
	}
	if err = q.DestroyAsset(ctx, asset.ID); err != nil {
		return fmt.Errorf("destroy: %w", err)
	}
	broadcastEvent(manager, gameID, model.EventAssetDestroyed, model.AssetIDPayload{AssetID: asset.ID})
	spender := playerDisplayName(ctx, q, spend.PlayerID)
	owner := playerDisplayName(ctx, q, asset.OwnerID)
	EmitShakeUpCommitted(
		ctx,
		q,
		manager,
		gameID,
		spend,
		finalCost,
		fmt.Sprintf(
			"%s spent %d token(s) to break %s's %s (%s)",
			spender,
			finalCost,
			owner,
			assetMark(asset.Name),
			want,
		),
		map[string]any{"effect": "break", "asset_id": asset.ID, "owner_id": asset.OwnerID},
	)
	return nil
}

// shakeUpBumpRank moves spender up one rank on the target track and pushes
// whoever was above them down one slot. Dummies are passed over.
func shakeUpBumpRank(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	spend *dbgen.ShakeUpSpend,
	track string,
	finalCost int16,
) error {
	playerID := spend.PlayerID
	cat := model.RankingCategory(track)
	spender := playerDisplayName(ctx, q, playerID)
	rankings, err := q.ListRankingsByGame(ctx, gameID)
	if err != nil {
		return fmt.Errorf("load rankings: %w", err)
	}
	var current, target int16
	var displaced *int64
	for _, rk := range rankings {
		if rk.Category == cat && rk.PlayerID != nil && *rk.PlayerID == playerID {
			current = rk.Rank
			break
		}
	}
	if current <= 1 {
		// Already at the top — the token is still spent, nothing moves. The
		// rules dwell on spends that change nothing, so log it anyway.
		EmitShakeUpCommitted(ctx, q, manager, gameID, spend, finalCost,
			fmt.Sprintf("%s spent %d token(s) to rise on %s, but is already at the top — no change",
				spender, finalCost, shakeUpCategoryTitle(track)),
			map[string]any{"effect": "bump", "track": track, "changed": false})
		return nil
	}
	target = current - 1
	for _, rk := range rankings {
		if rk.Category == cat && rk.Rank == target {
			displaced = rk.PlayerID
			break
		}
	}
	pid := playerID
	if err = q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
		GameID: gameID, PlayerID: &pid, Category: cat, Rank: target,
	}); err != nil {
		return fmt.Errorf("set bumped rank: %w", err)
	}
	if err = q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
		GameID: gameID, PlayerID: displaced, Category: cat, Rank: current,
	}); err != nil {
		return fmt.Errorf("set displaced rank: %w", err)
	}
	updated, _ := q.ListRankingsByGame(ctx, gameID)
	broadcastEvent(manager, gameID, model.EventRankingsUpdated, model.RankingsUpdatedPayload{Rankings: updated})
	body := fmt.Sprintf("%s spent %d token(s) to rise to rank %d on %s",
		spender, finalCost, target, shakeUpCategoryTitle(track))
	if displaced != nil {
		body += fmt.Sprintf(" (displacing %s)", playerDisplayName(ctx, q, *displaced))
	}
	EmitShakeUpCommitted(ctx, q, manager, gameID, spend, finalCost, body,
		map[string]any{"effect": "bump", "track": track, "changed": true, "new_rank": target})
	return nil
}

// shakeUpClaimTitle creates an artifact asset representing a freshly
// claimed title. The title text comes from the spend body's
// (currently target_player_id is repurposed as a title-text carrier — see
// note on payload). For Phase 4c, we keep this minimal: a generic artifact
// named "Title" that the player renames via the asset-edit endpoint.
func shakeUpClaimTitle(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	spend *dbgen.ShakeUpSpend,
	finalCost int16,
) error {
	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:    gameID,
		OwnerID:   spend.PlayerID,
		CreatorID: spend.PlayerID,
		AssetType: model.AssetArtifact,
		Name:      "New Title",
	})
	if err != nil {
		return fmt.Errorf("create title artifact: %w", err)
	}
	broadcastEvent(
		manager,
		gameID,
		model.EventAssetCreated,
		model.AssetPayload{Asset: assetWithMarginalia{Asset: asset, Marginalia: []dbgen.Marginalium{}}},
	)
	spender := playerDisplayName(ctx, q, spend.PlayerID)
	EmitShakeUpCommitted(ctx, q, manager, gameID, spend, finalCost,
		fmt.Sprintf("%s spent %d token(s) to claim a new title %s", spender, finalCost, assetMark(asset.Name)),
		map[string]any{"effect": "claim_title", "asset_id": asset.ID})
	return nil
}
