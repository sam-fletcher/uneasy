package handler

// handler/shake_up_pay_abandon.go — Shake-Up step-2 spending: the
// announce/adjust/pass/commit flow and the ADR-008 "pay or abandon" rulings
// for a spend whose cost got raised. See shake_up.go for the phase's full
// lifecycle.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

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
		if body.TitleFlavor != nil {
			flavor, ok := textField(w, "title_flavor", *body.TitleFlavor, maxAssetNameLen)
			if !ok {
				return
			}
			body.TitleFlavor = &flavor
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
		if spend.GameID != gameID || spend.CommittedAt.Valid || spend.AbandonedAt.Valid {
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
		// ADR-008 §1: the running cost has a hard floor of 1 — a −1 adjustment
		// while the cost already sits at 1 would drop it below that floor, so
		// it's rejected server-side (the UI mirrors this by disabling the
		// reduce button at that point, aria-disabled with the reason shown).
		if body.Adjustment == -1 {
			adjTotal, sErr := s.Q.SumAdjustmentsForSpend(ctx, spend.ID)
			if sErr != nil {
				respondInternalErr(w, r, "could not total adjustments", sErr)
				return
			}
			runningCost := spend.BaseCost + adjTotal
			if runningCost <= 1 {
				respondErr(w, http.StatusConflict, "the cost can't go below 1 token")
				return
			}
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
		EmitShakeUpAdjusted(ctx, s.Q, manager, gameID, spend, player.ID, body.Adjustment)
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
		if spend.GameID != gameID || spend.CommittedAt.Valid || spend.AbandonedAt.Valid {
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
		}
		respond(w, http.StatusOK, map[string]any{"pass": pass})
	}
}

// ShakeUpCommit handles POST /api/tables/{id}/shake-up/commit.
//
// Body: {"spend_id", "intent"?}. Caller must be the spender. ADR-008 "pay or
// abandon": if the running cost was never raised (extra == 0), the spend
// auto-resolves exactly as before and `intent` is ignored. If it was raised,
// the spender must name an intent — "pay" (charge the extra tokens, apply
// the effect) or "abandon" (no further charge, no effect, terminal abandoned
// state) — and the server, not the client, is authoritative on affordability:
// a "pay" that the spender can't actually afford is rejected (409), never
// capped. Every committed or adjuster token stays burned either way (no
// refunds). Advances the category if everyone is now at zero tokens.
func ShakeUpCommit(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		var body struct {
			SpendID int64  `json:"spend_id"`
			Intent  string `json:"intent"` // "pay" or "abandon"; required only when the cost was raised
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
		if spend.GameID != gameID || spend.CommittedAt.Valid || spend.AbandonedAt.Valid {
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
		// The adjust-time floor guard (§1) keeps base_cost + adjTotal ≥ 1
		// always, and base_cost is always 1, so extra == adjTotal is always
		// ≥ 0 here — never a refund.
		extra := adjTotal
		finalCost := spend.BaseCost + extra
		outcome := "committed"
		forced := false

		switch {
		case extra == 0:
			// Cost unchanged (or net-lowered back to the floor): the purchase
			// completes automatically, exactly as before. The chooser may not
			// back out.
			if err = shakeUpCommitAndApply(ctx, s.Q, manager, gameID, &spend, finalCost); err != nil {
				respondErr(w, http.StatusInternalServerError, err.Error())
				return
			}
		case body.Intent == "pay":
			if player.ShakeUpTokens < extra {
				respondErr(w, http.StatusConflict, fmt.Sprintf(
					"you cannot afford the raised cost (need %d more token(s), you have %d) — you must abandon",
					extra, player.ShakeUpTokens))
				return
			}
			if _, err = s.Q.SubShakeUpTokens(ctx, dbgen.SubShakeUpTokensParams{
				ID: player.ID, ShakeUpTokens: extra,
			}); err != nil {
				respondInternalErr(w, r, "could not deduct raised cost", err)
				return
			}
			if err = shakeUpCommitAndApply(ctx, s.Q, manager, gameID, &spend, finalCost); err != nil {
				respondErr(w, http.StatusInternalServerError, err.Error())
				return
			}
		case body.Intent == "abandon":
			outcome = "abandoned"
			forced = player.ShakeUpTokens < extra
			if err = shakeUpAbandonAndLog(ctx, s.Q, manager, gameID, &spend, finalCost, forced); err != nil {
				respondErr(w, http.StatusInternalServerError, err.Error())
				return
			}
		default:
			respondErr(w, http.StatusBadRequest,
				`the cost was raised — intent must be "pay" or "abandon"`)
			return
		}

		// Maybe advance the category. The announce consumed the spender's turn
		// whether the spend committed or was abandoned (ADR-008).
		if err = maybeAdvanceShakeUpCategory(ctx, s.Q, manager, gameID); err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"spend_id": spend.ID, "final_cost": finalCost, "outcome": outcome, "forced": forced,
		})
	}
}

// shakeUpCommitAndApply locks in finalCost, applies the mechanical effect (which
// also emits the shake_up.committed log post), and broadcasts the committed
// event. Shared by ShakeUpCommit's extra == 0 and "pay" branches — the only
// difference between them is whether the extra tokens were charged first.
func shakeUpCommitAndApply(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	spend *dbgen.ShakeUpSpend,
	finalCost int16,
) error {
	if err := q.CommitShakeUpSpend(ctx, dbgen.CommitShakeUpSpendParams{
		ID: spend.ID, FinalCost: &finalCost,
	}); err != nil {
		return fmt.Errorf("could not commit spend: %w", err)
	}
	if err := applyShakeUpEffect(ctx, q, manager, gameID, spend, finalCost); err != nil {
		return err
	}
	broadcastEvent(manager, gameID, model.EventShakeUpSpendCommitted, model.ShakeUpSpendCommittedPayload{
		SpendID: spend.ID, FinalCost: finalCost,
	})
	return nil
}

// shakeUpAbandonAndLog closes spend in the terminal abandoned state (no effect
// applied, ADR-008), then emits its log post and WS event.
func shakeUpAbandonAndLog(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	spend *dbgen.ShakeUpSpend,
	finalCost int16,
	forced bool,
) error {
	if err := q.AbandonShakeUpSpend(ctx, dbgen.AbandonShakeUpSpendParams{
		ID: spend.ID, FinalCost: &finalCost,
	}); err != nil {
		return fmt.Errorf("could not abandon spend: %w", err)
	}
	info, err := gamepkg.ShakeUpOption(spend.OptionKey)
	if err != nil {
		return err
	}
	EmitShakeUpAbandoned(ctx, q, manager, gameID, spend, finalCost, shakeUpOptionPhrase(info.Description), forced)
	broadcastEvent(manager, gameID, model.EventShakeUpSpendAbandoned, model.ShakeUpSpendAbandonedPayload{
		SpendID: spend.ID, FinalCost: finalCost, Forced: forced,
	})
	return nil
}

// currentShakeUpActor returns the player whose turn it is to announce a spend
// in the current spending step, per reverse-rank order (lowest status first,
// looping, skipping token-less players). Returns 0 if no one holds tokens.
func currentShakeUpActor(ctx context.Context, q *dbgen.Queries, gameID int64, category string) (int64, error) {
	rankings, err := q.ListRankingsByGame(ctx, gameID)
	if err != nil {
		return 0, fmt.Errorf("load rankings: %w", err)
	}
	order := gamepkg.ShakeUpTurnOrder(category, rankingRows(rankings))

	tokens, err := q.ListShakeUpTokensByGame(ctx, gameID)
	if err != nil {
		return 0, fmt.Errorf("load tokens: %w", err)
	}
	hasTokens := make(map[int64]bool, len(tokens))
	for _, t := range tokens {
		hasTokens[t.ID] = t.ShakeUpTokens > 0
	}

	// The last RESOLVED spend — committed or abandoned — consumed its
	// announcer's turn either way (ADR-008), so turn order must advance past
	// an abandon exactly like it does past a commit.
	var lastActor *int64
	if last, lerr := q.GetLastResolvedShakeUpSpend(ctx, dbgen.GetLastResolvedShakeUpSpendParams{
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
