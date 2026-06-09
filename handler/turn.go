package handler

// handler/turn.go — Focus player turn structure actions (Phase 2d).
//
// Per-row loop (rules §"Steps For Each Row"):
//
//  1. (War step — skipped in Phase 2)
//  2. Resolve topmost pending plan on this row (Phase 2f).
//  3. Focus player sets a scene (EndScene marks it complete).
//  4. Roleplay; dice if needed; add summary.
//  5. Focus player prepares a plan or refreshes assets (RefreshAssets).
//  6. Pass the focus marker clockwise (PassFocus).
//  7. If pending plans remain on this row, repeat from step 2 (server
//     auto-checks inside PassFocus).
//  8. Advance the current-row marker; cross engrailed lines; end if past 13
//     (PassFocus auto-advances when no plans remain after step 6).
//
// AdvanceRow is kept as a facilitator escape hatch (manual row advance without
// touching focus). PassFocus is the normal end-of-turn action and handles the
// step-7/8 logic automatically.

import (
	"encoding/json"
	"fmt"
	"net/http"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

const (
	minPlayerCount       = 2
	maxPlayerCount       = 5
	publicRecordRowCount = 13
)

// requireFocusPlayer validates that the caller is the current focus player.
// Returns the game and player, or writes an error response.
func requireFocusPlayer(w http.ResponseWriter, r *http.Request, q *dbgen.Queries) (*dbgen.Game, *dbgen.Player, bool) {
	gameID, player, ok := parseGamePlayer(w, r, q)
	if !ok {
		return nil, nil, false
	}
	game, err := q.GetGameByID(r.Context(), gameID)
	if err != nil {
		respondErr(w, http.StatusNotFound, "table not found")
		return nil, nil, false
	}
	if game.FocusPlayerID == nil || *game.FocusPlayerID != player.ID {
		respondErr(w, http.StatusForbidden, "only the focus player can do this")
		return nil, nil, false
	}
	return &game, player, true
}

// rawNextFocusPlayer returns the raw next player by join order, wrapping around.
// It does not check whether the player is eligible to hold focus (has peers).
func rawNextFocusPlayer(r *http.Request, q *dbgen.Queries, gameID, currentFocusID int64) (*dbgen.Player, error) {
	next, err := q.GetNextFocusPlayer(r.Context(), dbgen.GetNextFocusPlayerParams{
		GameID: gameID,
		ID:     currentFocusID,
	})
	if err != nil {
		// No later-joined player — wrap to the first.
		first, err2 := q.GetFirstFocusPlayer(r.Context(), gameID)
		if err2 != nil {
			return nil, err2
		}
		return &first, nil
	}
	return &next, nil
}

// nextFocusPlayer returns the next player in seat-order who has at least one
// non-destroyed peer asset. A player with no peers cannot be the focus player
// (they have no characters to act through).
//
// If the full rotation has no eligible players (everyone has lost all peers),
// it falls back to the raw next player so the game can still proceed.
func nextFocusPlayer(r *http.Request, q *dbgen.Queries, gameID, currentFocusID int64) (*dbgen.Player, error) {
	candidateID := currentFocusID
	var fallback *dbgen.Player // raw next, used if nobody has peers

	// Iterate at most once through all players (max 6 in a game).
	for range maxPlayerCount {
		next, err := rawNextFocusPlayer(r, q, gameID, candidateID)
		if err != nil {
			return nil, err
		}
		if fallback == nil {
			fallback = next
		}
		// If we've looped back to the original focus player, no one has peers.
		if next.ID == currentFocusID {
			break
		}

		hasPeers, err := playerHasPeers(r.Context(), q, gameID, next.ID)
		if err != nil || hasPeers {
			return next, err
		}

		candidateID = next.ID
	}

	// No eligible player found — return the raw next as a fallback so the
	// game can still proceed (facilitator must handle end state manually).
	return fallback, nil
}

// isEngrailedLine reports whether advancing from oldRow to newRow crosses
// an engrailed line. Engrailed lines fall after rows 4, 8, and 12.
func isEngrailedLine(oldRow, newRow int16) bool {
	for _, line := range []int16{4, 8, 12} {
		if oldRow <= line && newRow > line {
			return true
		}
	}
	return false
}

// advanceRowInner performs the shared row-advance logic used by both
// PassFocus (auto-advance) and AdvanceRow (manual). It increments
// current_row, broadcasts events, and transitions the game to ended if
// past row 13. Returns the new row number, or 0 if the game ended.
//
// h may be nil when no clients are connected — all h.BroadcastEvent calls
// are guarded by the nil check.
// Focus is NOT changed here — whoever holds it going in keeps it.
func advanceRowInner(
	r *http.Request,
	q *dbgen.Queries,
	manager *hub.Manager,
	h *hub.Hub,
	game *dbgen.Game,
) (int16, bool, error) {
	oldRow := game.CurrentRow

	newRow, err := q.AdvanceRow(r.Context(), game.ID)
	if err != nil {
		return 0, false, err
	}

	// Past row 13 — transition into the Shake-Up. From there, players spend
	// dice-rolled tokens across three categories before the game ends.
	if newRow > publicRecordRowCount {
		if err = BeginShakeUp(r.Context(), q, manager, game.ID); err != nil {
			return 0, false, err
		}
		return newRow, true, nil
	}

	crossed := isEngrailedLine(oldRow, newRow)

	// Run the ranking update when crossing an engrailed line. The update
	// resolves between the old row and the new one, so emit its chat narration
	// (and broadcast the new rankings) BEFORE the "Row N begins" boundary, so
	// the log reads in chronological order. runRankingUpdate lives in ranking.go.
	if crossed {
		updatedRankings, diff, rankErr := runRankingUpdate(r.Context(), q, game.ID)
		if rankErr == nil {
			if h != nil {
				h.BroadcastEvent(model.EventRankingsUpdated, model.RankingsUpdatedPayload{Rankings: updatedRankings})
			}
			EmitRankingUpdated(r.Context(), q, manager, game.ID, newRow, diff)
		}
	}

	if h != nil {
		h.BroadcastEvent(model.EventRowAdvanced, model.RowAdvancedPayload{
			RowNumber:        newRow,
			CrossedEngrailed: crossed,
		})
	}
	EmitSystemPost(r.Context(), q, manager, game.ID, "row.advanced",
		model.SeverityBoundary,
		fmt.Sprintf("Row %d begins", newRow), &newRow, nil, nil,
		map[string]any{"row_number": newRow, "crossed_engrailed": crossed})

	broadcastRowState(r.Context(), q, manager, game.ID)
	return newRow, false, nil
}

// EndScene handles POST /api/tables/{id}/end-scene.
//
// Validates the caller is the focus player and broadcasts scene.ended so all
// clients know the roleplay portion of this turn is complete. No DB write —
// the event is a coordination signal only.
func EndScene(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, player, ok := requireFocusPlayer(w, r, s.Q)
		if !ok {
			return
		}
		if game.Phase != model.PhaseMainEvent {
			respondErr(w, http.StatusConflict, "game is not in the main event phase")
			return
		}

		// If a scene is active, end it and write a public-record summary
		// using the same "[main character] at [holding], [time later]" text
		// as the scene-start banner. This replaces the old manual
		// "+ Add to public record" button.
		var endedSceneID int64
		ctx := r.Context()
		if active, err := loadActiveScene(ctx, s.Q, game.ID); err == nil && active != nil {
			if err := s.Q.EndScene(ctx, active.ID); err == nil {
				endedSceneID = active.ID
				summary := resolveSceneBannerText(ctx, s.Q, active, player.DisplayName)
				if entry, err := s.Q.CreateSceneEntry(ctx, dbgen.CreateSceneEntryParams{
					GameID:    game.ID,
					RowNumber: active.RowNumber,
					AuthorID:  player.ID,
					Body:      summary,
				}); err == nil {
					if h, ok := manager.Get(game.ID); ok {
						h.BroadcastEvent(model.EventSceneEntryCreated, model.SceneEntryCreatedPayload{Entry: entry})
					}
				}
			}
		}

		if h, ok := manager.Get(game.ID); ok {
			h.BroadcastEvent(model.EventSceneEnded, model.SceneEndedPayload{
				RowNumber: game.CurrentRow,
				PlayerID:  player.ID,
				SceneID:   endedSceneID,
			})
		}
		broadcastRowState(ctx, s.Q, manager, game.ID)
		row := game.CurrentRow
		var sceneIDPtr *int64
		if endedSceneID != 0 {
			sceneIDPtr = &endedSceneID
		}
		EmitSystemPost(ctx, s.Q, manager, game.ID, "scene.ended",
			model.SeverityImportant,
			fmt.Sprintf("%s ends the scene", player.DisplayName),
			&row, nil, sceneIDPtr,
			map[string]any{"row_number": row, "player_id": player.ID, "scene_id": endedSceneID})

		respond(w, http.StatusOK, map[string]any{
			"row_number": game.CurrentRow,
			"scene_id":   endedSceneID,
		})
	}
}

// RefreshAssets handles POST /api/tables/{id}/refresh-assets.
//
// The focus player refreshes up to current_row of their leveraged assets.
// Request body: {"asset_ids": [id1, id2, ...]}.
func RefreshAssets(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, player, ok := requireFocusPlayer(w, r, s.Q)
		if !ok {
			return
		}
		if game.Phase != model.PhaseMainEvent {
			respondErr(w, http.StatusConflict, "game is not in the main event phase")
			return
		}

		var body struct {
			AssetIDs []int64 `json:"asset_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if len(body.AssetIDs) == 0 {
			respond(w, http.StatusOK, map[string]any{"refreshed": []int64{}})
			return
		}

		maxRefresh := int(game.CurrentRow)
		if len(body.AssetIDs) > maxRefresh {
			respondErr(w, http.StatusBadRequest,
				fmt.Sprintf("can only refresh up to %d assets on row %d", maxRefresh, game.CurrentRow))
			return
		}

		ctx := r.Context()

		// Validate: all assets must be owned by the caller and currently leveraged.
		for _, id := range body.AssetIDs {
			asset, err := s.Q.GetAssetByID(ctx, id)
			if err != nil {
				respondErr(w, http.StatusBadRequest, "asset not found")
				return
			}
			if asset.OwnerID != player.ID {
				respondErr(w, http.StatusForbidden, "you can only refresh your own assets")
				return
			}
			if !asset.IsLeveraged {
				respondErr(w, http.StatusBadRequest, fmt.Sprintf("asset %d is not leveraged", id))
				return
			}
		}

		h, hasHub := manager.Get(game.ID)

		for _, id := range body.AssetIDs {
			if err := s.Q.RefreshPlayerAssets(ctx, id); err != nil {
				respondInternalErr(w, r, "could not refresh asset", err)
				return
			}
			if hasHub {
				h.BroadcastEvent(model.EventAssetRefreshed, model.AssetIDPayload{AssetID: id})
			}
			if asset, err := s.Q.GetAssetByID(ctx, id); err == nil {
				EmitAssetRefreshed(ctx, s.Q, manager, game.ID, asset, game.CurrentRow)
			}
		}

		// Refreshing assets is the focus player's step-5 action; pass the
		// focus marker automatically so refresh is a one-click commit. The
		// primary action has already committed, so a failure here is logged
		// and recovered via the manual /pass-focus endpoint rather than
		// failing the request.
		if err := autoPassFocus(r, s, manager, game); err != nil {
			loggerFromContext(r.Context()).Error("auto pass-focus after refresh-assets", "err", err)
		}

		respond(w, http.StatusOK, map[string]any{"refreshed": body.AssetIDs})
	}
}

// AdvanceRow handles POST /api/tables/{id}/advance-row.
//
// Facilitator escape hatch: manually advances the current row without touching
// the focus player. In normal play, PassFocus handles the row advance
// automatically after the last plan on a row is resolved. Use this endpoint
// only when the automatic path cannot be taken (e.g. stuck state recovery).
//
// Event order: row.advanced → rankings.updated (engrailed only) → phase.changed (ended only).
// Focus does NOT change.
func AdvanceRow(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, _, ok := requireFocusPlayer(w, r, s.Q)
		if !ok {
			return
		}
		if game.Phase != model.PhaseMainEvent {
			respondErr(w, http.StatusConflict, "game is not in the main event phase")
			return
		}

		h, _ := manager.Get(game.ID) // nil if no clients connected — advanceRowInner handles nil

		if outstanding, err := mwOutstandingCostsForGame(r.Context(), s.Q, game.ID, game.CurrentRow); err != nil {
			respondInternalErr(w, r, "could not check battle costs", err)
			return
		} else if len(outstanding) > 0 {
			respondErr(w, http.StatusConflict, "outstanding battle costs must be paid before advancing the row")
			return
		}
		if claims, err := mwOutstandingSurrenderClaimsForGame(r.Context(), s.Q, game.ID); err != nil {
			respondInternalErr(w, r, "could not check surrender claims", err)
			return
		} else if len(claims) > 0 {
			respondErr(
				w,
				http.StatusConflict,
				"opposing players must take an asset from each surrendered player before advancing",
			)
			return
		}

		newRow, ended, err := advanceRowInner(r, s.Q, manager, h, game)
		if err != nil {
			respondInternalErr(w, r, "could not advance row", err)
			return
		}

		if ended {
			respond(w, http.StatusOK, map[string]any{"phase": model.PhaseEnded})
			return
		}
		mwBroadcastBattleCostsDue(r.Context(), s.Q, manager, game.ID, newRow)
		respond(w, http.StatusOK, map[string]any{
			"row_number":        newRow,
			"crossed_engrailed": isEngrailedLine(game.CurrentRow, newRow),
		})
	}
}

// autoPassFocus runs steps 6–8 of the per-row loop as a side effect after the
// focus player's primary action (plan prep, asset refresh) has already
// succeeded. It is the same logic as PassFocus but writes no HTTP response.
//
// Returns an error only for hard failures (focus could not be moved, or row
// advance failed after focus moved). Expected soft conditions — pending plans
// remain on the row, or war costs / surrender claims block row advance — are
// not errors; the caller's primary action still committed, and the manual
// PassFocus endpoint remains as a recovery path either way.
func autoPassFocus(r *http.Request, s *db.Store, manager *hub.Manager, game *dbgen.Game) error {
	if game.Phase != model.PhaseMainEvent || game.FocusPlayerID == nil {
		return nil
	}
	ctx := r.Context()

	next, err := nextFocusPlayer(r, s.Q, game.ID, *game.FocusPlayerID)
	if err != nil {
		return fmt.Errorf("determine next focus player: %w", err)
	}
	if err = s.Q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{
		ID:            game.ID,
		FocusPlayerID: new(next.ID),
	}); err != nil {
		return fmt.Errorf("update focus player: %w", err)
	}

	h, hasHub := manager.Get(game.ID)
	if hasHub {
		h.BroadcastEvent(model.EventFocusChanged, model.FocusChangedPayload{
			PlayerID:    next.ID,
			DisplayName: next.DisplayName,
		})
	}
	// Row state always changes when focus does (the new focus player's
	// turn-scene state is different from the previous player's). If the
	// row-advance path below fires too, advanceRowInner will re-broadcast
	// with the post-advance state — idempotent.
	broadcastRowState(ctx, s.Q, manager, game.ID)

	// Soft conditions that block row advance: pending plans, outstanding
	// war costs, or open surrender claims. Each check is conservative —
	// any DB error is treated as "skip row advance" so we never advance
	// past a state we couldn't verify is clear. Failures are logged but
	// not returned, since focus has already moved and /advance-row is the
	// recovery path.
	logger := loggerFromContext(ctx)

	pending, err := s.Q.ListPendingPlansByRow(ctx, dbgen.ListPendingPlansByRowParams{
		GameID:    game.ID,
		RowNumber: new(game.CurrentRow),
	})
	if err != nil {
		logger.Warn("auto pass-focus: could not list pending plans; skipping row advance", "err", err)
		return nil
	}
	if len(pending) > 0 {
		return nil
	}

	outstanding, err := mwOutstandingCostsForGame(ctx, s.Q, game.ID, game.CurrentRow)
	if err != nil {
		logger.Warn("auto pass-focus: could not check outstanding war costs; skipping row advance", "err", err)
		return nil
	}
	if len(outstanding) > 0 {
		return nil
	}

	claims, err := mwOutstandingSurrenderClaimsForGame(ctx, s.Q, game.ID)
	if err != nil {
		logger.Warn("auto pass-focus: could not check surrender claims; skipping row advance", "err", err)
		return nil
	}
	if len(claims) > 0 {
		return nil
	}

	newRow, ended, err := advanceRowInner(r, s.Q, manager, h, game)
	if err != nil {
		return fmt.Errorf("advance row: %w", err)
	}
	if !ended {
		mwBroadcastBattleCostsDue(ctx, s.Q, manager, game.ID, newRow)
	}
	return nil
}

// PassFocus handles POST /api/tables/{id}/pass-focus.
//
// Implements rules steps 6–8 of the per-row loop:
//
//  6. Pass the focus marker clockwise → broadcasts focus.changed.
//  7. Check if pending plans remain on this row.
//  8. If none remain, advance the row automatically → broadcasts row.advanced
//     (and rankings.updated at engrailed lines, phase.changed if the game ends).
//
// Focus does NOT change again on the row advance — whoever receives it in
// step 6 carries it into the next row.
//
//nolint:funlen // TODO: see if there's a clean way to break this up
func PassFocus(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, _, ok := requireFocusPlayer(w, r, s.Q)
		if !ok {
			return
		}
		if game.Phase != model.PhaseMainEvent {
			respondErr(w, http.StatusConflict, "game is not in the main event phase")
			return
		}
		if game.FocusPlayerID == nil {
			respondErr(w, http.StatusConflict, "no focus player set")
			return
		}

		ctx := r.Context()

		// Step 6: pass focus to the next player clockwise.
		next, err := nextFocusPlayer(r, s.Q, game.ID, *game.FocusPlayerID)
		if err != nil {
			respondInternalErr(w, r, "could not determine next focus player", err)
			return
		}

		if err = s.Q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{
			ID:            game.ID,
			FocusPlayerID: new(next.ID),
		}); err != nil {
			respondInternalErr(w, r, "could not update focus player", err)
			return
		}

		h, hasHub := manager.Get(game.ID)
		if hasHub {
			h.BroadcastEvent(model.EventFocusChanged, model.FocusChangedPayload{
				PlayerID:    next.ID,
				DisplayName: next.DisplayName,
			})
		}
		broadcastRowState(ctx, s.Q, manager, game.ID)

		// Step 7: are there pending plans still on this row?
		pending, err := s.Q.ListPendingPlansByRow(ctx, dbgen.ListPendingPlansByRowParams{
			GameID:    game.ID,
			RowNumber: new(game.CurrentRow),
		})
		if err != nil {
			// Non-fatal: pass focus succeeded; leave row advance to the
			// facilitator's manual /advance-row if needed. Log so a
			// persistent DB issue here doesn't silently stall row advance.
			loggerFromContext(ctx).Warn("pass-focus: could not list pending plans; skipping row advance", "err", err)
			respond(w, http.StatusOK, map[string]any{
				"focus_player_id":   next.ID,
				"focus_player_name": next.DisplayName,
			})
			return
		}

		if len(pending) > 0 {
			// Plans remain — new focus player will resolve the next one.
			// No row advance yet.
			respond(w, http.StatusOK, map[string]any{
				"focus_player_id":   next.ID,
				"focus_player_name": next.DisplayName,
			})
			return
		}

		// Step 8: no plans remain — advance the row automatically, unless
		// any active war still has unpaid battle costs for the current row,
		// or any surrender claim is still open. Both checks are conservative
		// on error: if we can't verify the row is clear, we skip the advance
		// and the facilitator can retry via /advance-row.
		logger := loggerFromContext(ctx)

		outstanding, costErr := mwOutstandingCostsForGame(ctx, s.Q, game.ID, game.CurrentRow)
		if costErr != nil {
			logger.Warn("pass-focus: could not check outstanding war costs; skipping row advance", "err", costErr)
			respond(w, http.StatusOK, map[string]any{
				"focus_player_id":   next.ID,
				"focus_player_name": next.DisplayName,
				"advance_blocked":   "could not verify war costs; retry via /advance-row",
			})
			return
		}
		if len(outstanding) > 0 {
			respond(w, http.StatusOK, map[string]any{
				"focus_player_id":   next.ID,
				"focus_player_name": next.DisplayName,
				"advance_blocked":   "outstanding battle costs must be paid before the row can advance",
			})
			return
		}

		claims, claimErr := mwOutstandingSurrenderClaimsForGame(ctx, s.Q, game.ID)
		if claimErr != nil {
			logger.Warn("pass-focus: could not check surrender claims; skipping row advance", "err", claimErr)
			respond(w, http.StatusOK, map[string]any{
				"focus_player_id":   next.ID,
				"focus_player_name": next.DisplayName,
				"advance_blocked":   "could not verify surrender claims; retry via /advance-row",
			})
			return
		}
		if len(claims) > 0 {
			respond(w, http.StatusOK, map[string]any{
				"focus_player_id":   next.ID,
				"focus_player_name": next.DisplayName,
				"advance_blocked":   "opposing players must take an asset from each surrendered player before the row can advance",
			})
			return
		}

		// Focus stays with `next` (they carry it into the new row).
		newRow, ended, err := advanceRowInner(r, s.Q, manager, h, game)
		if err != nil {
			// Row advance failed after focus already moved — not ideal, but
			// focus.changed was already broadcast so respond with what we have.
			respond(w, http.StatusOK, map[string]any{
				"focus_player_id":   next.ID,
				"focus_player_name": next.DisplayName,
				"advance_error":     "could not advance row; use /advance-row to retry",
			})
			return
		}

		if ended {
			respond(w, http.StatusOK, map[string]any{
				"focus_player_id":   next.ID,
				"focus_player_name": next.DisplayName,
				"phase":             model.PhaseEnded,
			})
			return
		}

		mwBroadcastBattleCostsDue(ctx, s.Q, manager, game.ID, newRow)
		respond(w, http.StatusOK, map[string]any{
			"focus_player_id":   next.ID,
			"focus_player_name": next.DisplayName,
			"row_number":        newRow,
			"crossed_engrailed": isEngrailedLine(game.CurrentRow, newRow),
		})
	}
}
