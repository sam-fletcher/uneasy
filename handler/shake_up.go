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
	"strings"

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
			game.ShakeUpCategory != nil {
			if actor, aerr := currentShakeUpActor(ctx, s.Q, gameID, *game.ShakeUpCategory); aerr == nil && actor != 0 {
				out["current_actor"] = actor
			}
		}

		// Step 1 (rolling): who's up, and the open roll id if the client wants to
		// fetch its full state (dice, participants) via getActiveRollForGame/getRoll.
		if game.ShakeUpStep != nil && *game.ShakeUpStep == gamepkg.ShakeUpStepRolling &&
			game.ShakeUpCategory != nil {
			if roller, rerr := shakeUpNextRoller(ctx, s.Q, gameID, *game.ShakeUpCategory); rerr == nil && roller != 0 {
				out["current_roller_id"] = roller
			}
			if openRoll, rerr := s.Q.GetOpenShakeUpRollByGame(ctx, gameID); rerr == nil {
				out["open_roll_id"] = openRoll.ID
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
	EmitShakeUpBegin(ctx, q, manager, gameID, cat)

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
	rows := make([]gamepkg.RankingRow, 0, len(rankings))
	for _, rk := range rankings {
		rows = append(rows, gamepkg.RankingRow{
			PlayerID: rk.PlayerID, Category: string(rk.Category), Rank: rk.Rank,
		})
	}
	order := gamepkg.ShakeUpTurnOrder(category, rows)

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
		// Superseded by finalizeShakeUpRoll (real dice rolls); this handler is
		// unreachable in practice (CreateDiceRoll's Difficulty:0 always trips the
		// CHECK constraint) and slated for deletion once ShakeUpView.svelte stops
		// calling it. diceCount has no real dice to report here, so reuse Result.
		EmitShakeUpRolled(
			ctx,
			s.Q,
			manager,
			gameID,
			player.ID,
			body.Result,
			body.Result,
			newTotal,
			*game.ShakeUpCategory,
		)

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

// shakeUpAnnounceBody is the request body for ShakeUpAnnounce.
type shakeUpAnnounceBody struct {
	OptionKey          string  `json:"option_key"`
	TargetAssetID      *int64  `json:"target_asset_id"`
	TargetMarginaliaID *int64  `json:"target_marginalia_id"`
	TargetPlayerID     *int64  `json:"target_player_id"`
	TargetTitleID      *string `json:"target_title_id"`
	TitleFlavor        *string `json:"title_flavor"`
}

// validateShakeUpAnnounceTarget runs every announce-time target-shape check
// for the chosen option — the required asset, a break's marginalia, a take's
// asset ownership, and claim-title's title/peer — each re-checked
// authoritatively at commit since the board can change while the spend is
// open. Extracted from ShakeUpAnnounce to keep it flat: this only fans out to
// the per-option validators, it doesn't duplicate their logic. Writes the
// error response and returns false on the first failure.
func validateShakeUpAnnounceTarget(
	ctx context.Context,
	w http.ResponseWriter,
	q *dbgen.Queries,
	gameID, playerID int64,
	info gamepkg.ShakeUpOptionInfo,
	body shakeUpAnnounceBody,
) bool {
	if info.NeedsAsset && body.TargetAssetID == nil {
		respondErr(w, http.StatusBadRequest, "target_asset_id required for "+info.Key)
		return false
	}
	// Break options tear one marginalia (the canonical break), so the breaker
	// must name which one.
	if info.NeedsMarginalia &&
		!validateShakeUpBreakTarget(ctx, w, q, info, body.TargetAssetID, body.TargetMarginaliaID) {
		return false
	}
	// Take options must target another player's asset of the right type.
	if want := expectedTakeType(info.Key); want != "" &&
		!validateShakeUpTakeTarget(ctx, w, q, gameID, playerID, want, body.TargetAssetID) {
		return false
	}
	// Claim-a-new-title stamps a marginalia onto one of the claimer's own peers
	// (ADR-007). Validate the chosen title + receiving peer up front.
	if info.Key == gamepkg.ShakeUpOptClaimTitle &&
		!validateShakeUpClaimTitle(ctx, w, q, gameID, playerID, body.TargetTitleID, body.TargetAssetID) {
		return false
	}
	return true
}

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

		var body shakeUpAnnounceBody
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
		if !validateShakeUpAnnounceTarget(ctx, w, s.Q, gameID, player.ID, info, body) {
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
			GameID:             gameID,
			PlayerID:           player.ID,
			Category:           info.Category,
			OptionKey:          info.Key,
			TargetAssetID:      body.TargetAssetID,
			TargetMarginaliaID: body.TargetMarginaliaID,
			TargetPlayerID:     body.TargetPlayerID,
			TargetTitleID:      body.TargetTitleID,
			TitleFlavor:        body.TitleFlavor,
			BaseCost:           1,
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
		// A cost change reopens the reaction window for everyone (ruling 5) —
		// clear every existing pass so the commit gate re-checks from scratch.
		if err = s.Q.DeletePassesForSpend(ctx, spend.ID); err != nil {
			respondInternalErr(w, r, "could not reset passes", err)
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

// ShakeUpPass handles POST /api/tables/{id}/shake-up/pass.
//
// Body: {"spend_id"}. Records that the caller has reviewed the open spend and
// declines to adjust it further ("lets it stand") — one of the two ways
// (adjust or pass) a reactor can clear themselves from the commit gate
// (ruling 5). Idempotent: passing again on a spend you already passed on is a
// no-op success, not an error.
func ShakeUpPass(s *db.Store, manager *hub.Manager) http.HandlerFunc {
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
		if spend.PlayerID == player.ID {
			respondErr(w, http.StatusForbidden, "the spender cannot pass on their own spend")
			return
		}

		existing, err := s.Q.ListPassesForSpend(ctx, spend.ID)
		if err != nil {
			respondInternalErr(w, r, "could not check existing passes", err)
			return
		}
		alreadyPassed := false
		for _, p := range existing {
			if p.PlayerID == player.ID {
				alreadyPassed = true
				break
			}
		}

		pass, err := s.Q.CreateShakeUpPass(ctx, dbgen.CreateShakeUpPassParams{
			SpendID: spend.ID, PlayerID: player.ID,
		})
		if err != nil {
			respondInternalErr(w, r, "could not record pass", err)
			return
		}
		if !alreadyPassed {
			broadcastEvent(manager, gameID, model.EventShakeUpPassed, model.ShakeUpPassedPayload{
				SpendID: spend.ID, PlayerID: player.ID,
			})
			EmitShakeUpPassed(ctx, s.Q, manager, gameID, spend, player.ID)
		}
		respond(w, http.StatusOK, map[string]any{"pass": pass})
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

		// The spender cannot rush the commit (ruling 5): every other player who
		// still holds a token must react — adjust or explicitly pass — first.
		pending, err := shakeUpPendingReactors(ctx, s.Q, gameID, spend)
		if err != nil {
			respondInternalErr(w, r, "could not check reactors", err)
			return
		}
		if len(pending) > 0 {
			names := make([]string, len(pending))
			for i, pid := range pending {
				names[i] = playerDisplayName(ctx, s.Q, pid)
			}
			respondErr(w, http.StatusConflict,
				fmt.Sprintf("waiting on %s to react", strings.Join(names, ", ")))
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

// shakeUpPendingReactors returns the ids of every player other than the
// spender who still holds ≥1 token and has not yet passed on this spend
// (ruling 5: the reaction gate). Players at 0 tokens are exempt — they
// cannot adjust anyway — so they never block a commit. Returns an empty
// (non-nil) slice, never nil, so callers can serialize it directly.
func shakeUpPendingReactors(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
	spend dbgen.ShakeUpSpend,
) ([]int64, error) {
	tokens, err := q.ListShakeUpTokensByGame(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("load tokens: %w", err)
	}
	passes, err := q.ListPassesForSpend(ctx, spend.ID)
	if err != nil {
		return nil, fmt.Errorf("load passes: %w", err)
	}
	passed := make(map[int64]bool, len(passes))
	for _, p := range passes {
		passed[p.PlayerID] = true
	}
	pending := []int64{}
	for _, t := range tokens {
		if t.ID == spend.PlayerID || t.ShakeUpTokens < 1 || passed[t.ID] {
			continue
		}
		pending = append(pending, t.ID)
	}
	return pending, nil
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
	if asset.GameID != gameID {
		return errors.New("target asset belongs to another game")
	}
	if asset.AssetType != want {
		return fmt.Errorf("target must be a %s asset", want)
	}
	if asset.IsDestroyed {
		return errors.New("asset is destroyed")
	}
	if asset.OwnerID == spend.PlayerID {
		return errors.New("cannot take your own asset")
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

// validateShakeUpBreakTarget checks an announce-time break target: a marginalia
// must be named, exist, belong to the named asset, and still be intact. It
// writes the error response and returns false on any failure. The apply step
// (shakeUpBreakAsset) re-checks authoritatively at commit, since the marginalia
// could be torn by another action while the spend is open.
func validateShakeUpBreakTarget(
	ctx context.Context,
	w http.ResponseWriter,
	q *dbgen.Queries,
	info gamepkg.ShakeUpOptionInfo,
	targetAssetID, targetMarginaliaID *int64,
) bool {
	if targetMarginaliaID == nil {
		respondErr(w, http.StatusBadRequest, "target_marginalia_id required for "+info.Key)
		return false
	}
	m, err := q.GetMarginaliaByID(ctx, *targetMarginaliaID)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "target marginalia not found")
		return false
	}
	if targetAssetID == nil || m.AssetID != *targetAssetID {
		respondErr(w, http.StatusBadRequest, "marginalia does not belong to the target asset")
		return false
	}
	if m.IsTorn {
		respondErr(w, http.StatusConflict, "marginalia is already torn")
		return false
	}
	return true
}

// validateShakeUpTakeTarget checks an announce-time take target: the asset
// must exist, belong to this game, be the option's asset type, be intact, and
// NOT be owned by the spender (taking your own asset is a meaningless no-op —
// ruling 8). It writes the error response and returns false on any failure.
// shakeUpTakeAsset re-checks ownership authoritatively at commit, since the
// asset could change hands while the spend is open. Caller guarantees
// targetAssetID is non-nil (the option's NeedsAsset check runs first).
func validateShakeUpTakeTarget(
	ctx context.Context,
	w http.ResponseWriter,
	q *dbgen.Queries,
	gameID, spenderID int64,
	want model.AssetType,
	targetAssetID *int64,
) bool {
	asset, err := q.GetAssetByID(ctx, *targetAssetID)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "target asset not found")
		return false
	}
	if asset.GameID != gameID {
		respondErr(w, http.StatusBadRequest, "target asset belongs to another game")
		return false
	}
	if asset.AssetType != want {
		respondErr(w, http.StatusBadRequest, fmt.Sprintf("target must be a %s asset", want))
		return false
	}
	if asset.IsDestroyed {
		respondErr(w, http.StatusConflict, "asset is destroyed")
		return false
	}
	if asset.OwnerID == spenderID {
		respondErr(w, http.StatusForbidden, "you cannot take your own asset")
		return false
	}
	return true
}

// shakeUpBreakAsset applies a "break a … asset" spend by tearing the single
// marginalia the breaker chose — the canonical break (see breakMarginalia):
// "tear off one marginalia = breaking an asset; all 4 gone → destroyed". This
// also grants the breaker visibility on the asset's secrets and, via
// EmitMarginaliaTorn, writes the standard marginalia.torn log with its
// "how has it changed?" prompt — none of which the old whole-asset DestroyAsset
// did. The shake_up.committed post still records the token spend for the ledger.
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
	if spend.TargetMarginaliaID == nil {
		return errors.New("target_marginalia_id required")
	}
	asset, err := q.GetAssetByID(ctx, *spend.TargetAssetID)
	if err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}
	if asset.GameID != gameID {
		return errors.New("target asset belongs to another game")
	}
	if asset.AssetType != want {
		return fmt.Errorf("target must be a %s asset", want)
	}
	if asset.IsDestroyed {
		return errors.New("asset is already destroyed")
	}
	m, err := q.GetMarginaliaByID(ctx, *spend.TargetMarginaliaID)
	if err != nil {
		return fmt.Errorf("marginalia not found: %w", err)
	}
	if m.AssetID != asset.ID {
		return errors.New("marginalia does not belong to the target asset")
	}
	if m.IsTorn {
		return errors.New("marginalia is already torn")
	}

	// Canonical tear + destroy-if-last + secret-visibility grant to the breaker.
	destroyed, err := breakMarginalia(ctx, q, manager, &asset, &m, spend.PlayerID)
	if err != nil {
		return fmt.Errorf("tear marginalia: %w", err)
	}
	// breakMarginalia doesn't log the tear; emit the canonical marginalia.torn
	// post (with the owner re-describe prompt) like every other break.
	if g, gErr := q.GetGameByID(ctx, gameID); gErr == nil {
		EmitMarginaliaTorn(ctx, q, manager, gameID, asset, m, spend.PlayerID, destroyed, g.CurrentRow)
	}

	spender := playerDisplayName(ctx, q, spend.PlayerID)
	owner := playerDisplayName(ctx, q, asset.OwnerID)
	body := fmt.Sprintf(
		"%s spent %d token(s) to break %s's %s (%s)",
		spender,
		finalCost,
		owner,
		assetMark(asset.Name),
		want,
	)
	if destroyed {
		body += ", destroying it"
	}
	EmitShakeUpCommitted(
		ctx,
		q,
		manager,
		gameID,
		spend,
		finalCost,
		body,
		map[string]any{
			"effect":        "break",
			"asset_id":      asset.ID,
			"owner_id":      asset.OwnerID,
			"marginalia_id": m.ID,
			"destroyed":     destroyed,
		},
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
	// Build the category's rank→occupant map (nil = dummy/empty slot) so we can
	// climb past dummies. Dummy tokens occupy real rank slots (e.g. rank 1 in
	// 2–3p games), so "rank N-1" is not necessarily a real player — bumping must
	// pass over dummies and swap with the first *real* player above, matching the
	// engrailed ranking update (swapTokenPlayerWithAbove). Swapping into a dummy's
	// slot would corrupt the track and let a top-real player illegitimately "rise".
	occupant := map[int16]*int64{}
	var current int16
	for _, rk := range rankings {
		if rk.Category != cat {
			continue
		}
		occupant[rk.Rank] = rk.PlayerID
		if rk.PlayerID != nil && *rk.PlayerID == playerID {
			current = rk.Rank
		}
	}
	// Search upward from current-1 for the first real player to overtake; skip
	// dummy (nil) slots. No real player above → the spender is effectively at the
	// top, so the bump is a (logged) no-op.
	var target int16
	var displaced *int64
	for r := current - 1; r >= 1; r-- {
		if occupant[r] != nil {
			target = r
			displaced = occupant[r]
			break
		}
	}
	if target == 0 {
		// Already at the top (only dummies / nothing above) — the token is still
		// spent, nothing moves. The rules dwell on spends that change nothing, so
		// log it anyway.
		EmitShakeUpCommitted(ctx, q, manager, gameID, spend, finalCost,
			fmt.Sprintf("%s spent %d token(s) to rise on %s, but is already at the top — no change",
				spender, finalCost, shakeUpCategoryTitle(track)),
			map[string]any{"effect": "bump", "track": track, "changed": false})
		return nil
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

// shakeUpTitleAlreadyClaimed reports whether titleID has been claimed anywhere in
// the game (ADR-007 game-wide uniqueness). It counts every claim site — Prologue
// choosing-phase claims, the ≤3-player extra-peer claim, and prior Shake-Up
// claims — and counts torn / destroyed claims too: a deposed monarch's title is
// still "already claimed", so it can't be re-minted onto a fresh peer.
func shakeUpTitleAlreadyClaimed(ctx context.Context, q *dbgen.Queries, gameID int64, titleID string) (bool, error) {
	claimed, err := q.ListClaimedTitleIDsByGame(ctx, gameID)
	if err != nil {
		return false, fmt.Errorf("load claimed titles: %w", err)
	}
	for _, c := range claimed {
		if c != nil && *c == titleID {
			return true, nil
		}
	}
	return false, nil
}

// validateShakeUpClaimTitle checks an announce-time claim-title spend: the chosen
// title must be a real, game-wide-unclaimed title, and the receiving asset must
// be one of the claimer's own peers with a free marginalia slot. It writes the
// error response and returns false on any failure. shakeUpClaimTitle re-checks
// authoritatively at commit (another claim could land while this spend is open).
func validateShakeUpClaimTitle(
	ctx context.Context,
	w http.ResponseWriter,
	q *dbgen.Queries,
	gameID, playerID int64,
	titleID *string,
	targetAssetID *int64,
) bool {
	if titleID == nil || gamepkg.TitleChoiceByID(*titleID) == nil {
		respondErr(w, http.StatusBadRequest, "target_title_id must be a valid title")
		return false
	}
	claimed, err := shakeUpTitleAlreadyClaimed(ctx, q, gameID, *titleID)
	if err != nil {
		respondInternalErr(w, nil, "could not check claimed titles", err)
		return false
	}
	if claimed {
		respondErr(w, http.StatusConflict, "that title has already been claimed")
		return false
	}
	if targetAssetID == nil {
		respondErr(w, http.StatusBadRequest, "target_asset_id (the peer to title) required for claim_title")
		return false
	}
	asset, err := q.GetAssetByID(ctx, *targetAssetID)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "target asset not found")
		return false
	}
	if asset.OwnerID != playerID {
		respondErr(w, http.StatusForbidden, "you can only title your own peer")
		return false
	}
	if asset.AssetType != model.AssetPeer || asset.IsDestroyed {
		respondErr(w, http.StatusBadRequest, "the title must be stamped on one of your peers")
		return false
	}
	pos, err := findOpenMarginaliaPosition(ctx, q, asset.ID)
	if err != nil {
		respondInternalErr(w, nil, "could not inspect marginalia", err)
		return false
	}
	if pos == 0 {
		respondErr(w, http.StatusConflict, "that peer has no free marginalia slot for a title")
		return false
	}
	return true
}

// shakeUpClaimTitle stamps a freshly claimed title as a marginalia on one of the
// claimer's peers (ADR-007). Unlike the pre-ADR stub — which minted a generic
// "New Title" artifact invisible to the line of succession — this routes through
// the same CreateTitleMarginalia + EstablishThrone path the Prologue uses, so a
// monarchy or heir claimed here is a real, contestable title: it trips the throne
// gate when it's the monarch, and currentMonarch / Propose Decree / the crown UI
// all pick it up. No artifact and no playing cards are created — the role lives
// on the marginalia.
func shakeUpClaimTitle(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	spend *dbgen.ShakeUpSpend,
	finalCost int16,
) error {
	if spend.TargetTitleID == nil {
		return errors.New("target_title_id required")
	}
	titleID := *spend.TargetTitleID
	choice := gamepkg.TitleChoiceByID(titleID)
	if choice == nil {
		return errors.New("unknown title")
	}
	// Re-check uniqueness at commit: another player may have claimed this title
	// while the spend was open.
	claimed, err := shakeUpTitleAlreadyClaimed(ctx, q, gameID, titleID)
	if err != nil {
		return err
	}
	if claimed {
		return errors.New("that title has already been claimed")
	}
	if spend.TargetAssetID == nil {
		return errors.New("target_asset_id required")
	}
	asset, err := q.GetAssetByID(ctx, *spend.TargetAssetID)
	if err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}
	if asset.OwnerID != spend.PlayerID || asset.AssetType != model.AssetPeer || asset.IsDestroyed {
		return errors.New("the title must be stamped on one of your own peers")
	}
	pos, err := findOpenMarginaliaPosition(ctx, q, asset.ID)
	if err != nil {
		return fmt.Errorf("inspect marginalia: %w", err)
	}
	if pos == 0 {
		return errors.New("that peer has no free marginalia slot for a title")
	}

	// The marginalia text is the player's freeform flavor; the title id is the
	// immutable role. Default to the title's display name when no flavor is given.
	text := choice.Name
	if spend.TitleFlavor != nil && strings.TrimSpace(*spend.TitleFlavor) != "" {
		text = strings.TrimSpace(*spend.TitleFlavor)
	}
	m, err := q.CreateTitleMarginalia(ctx, dbgen.CreateTitleMarginaliaParams{
		AssetID:  asset.ID,
		Position: pos,
		Text:     text,
		Title:    &titleID,
	})
	if err != nil {
		return fmt.Errorf("create title marginalia: %w", err)
	}

	// Claiming the monarch title trips the one-way throne gate the succession
	// hinges on — exactly as the Prologue claim does.
	if titleID == gamepkg.TitleMonarch {
		if err = q.EstablishThrone(ctx, gameID); err != nil {
			return fmt.Errorf("establish throne: %w", err)
		}
	}

	// Broadcast the new marginalia so connected clients update the peer's card and
	// flip throne_established live (ws-handlers' establishThroneIfMonarch) — that's
	// what lights up the Phase D crown UI without a refresh.
	broadcastEvent(manager, gameID, model.EventMarginaliaAdded, model.MarginaliaPayload{
		AssetID:    asset.ID,
		Marginalia: m,
	})

	spender := playerDisplayName(ctx, q, spend.PlayerID)
	EmitShakeUpCommitted(ctx, q, manager, gameID, spend, finalCost,
		fmt.Sprintf("%s spent %d token(s) to claim the title %s on %s",
			spender, finalCost, assetMark(choice.Name), assetMark(asset.Name)),
		map[string]any{
			"effect":        "claim_title",
			"title":         titleID,
			"asset_id":      asset.ID,
			"marginalia_id": m.ID,
		})
	return nil
}
