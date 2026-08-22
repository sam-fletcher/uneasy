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
// PassFocus handles steps 6–8 and is the only way a row advances in play. It
// runs automatically (autoPassFocus) after the focus player's step-5 action, so
// the endpoint itself is a manual fallback for the rare case where that
// post-commit side effect is dropped.
//
// There is deliberately NO manual row-advance route. One existed
// (POST /tables/{id}/advance-row) and was removed — see
// adr/FACILITATOR_POWERS_AUDIT.md. It skipped the pending-plan check PassFocus
// performs below, so it could advance past another player's unresolved plan and
// strand it permanently (topPendingPlanOnRow matches row_number exactly, and
// nothing re-homes a plan left behind). Pushing a stalled table forward is a
// social matter, not a mechanical one; a genuinely dropped advance is retried by
// the next player's autoPassFocus. Tests that need to fast-forward the record
// use POST /api/dev/advance-row (dev-gated, see handler/dev.go).

import (
	"context"
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

// advanceRowInner performs the shared row-advance logic behind every path that
// ends a row. It increments current_row, broadcasts events, and transitions the
// game into the Shake-Up if past row 13. Returns the new row number, or 0 if the
// game ended.
//
// It takes a ctx rather than the *http.Request it only ever pulled a context
// out of, because not every caller has a request: the Explosive Finale's row 13
// has no turns, so the row that ends it is ended by broadcastRowState
// (finaleTerminalAdvance in finale.go), not by an HTTP handler.
//
// h may be nil when no clients are connected — all h.BroadcastEvent calls
// are guarded by the nil check.
// Focus is NOT changed here — whoever holds it going in keeps it.
func advanceRowInner(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	h *hub.Hub,
	game *dbgen.Game,
) (int16, bool, error) {
	oldRow := game.CurrentRow

	newRow, err := q.AdvanceRow(ctx, game.ID)
	if err != nil {
		return 0, false, err
	}

	// Past row 13 — transition into the Shake-Up. From there, players spend
	// dice-rolled tokens across three categories before the game ends.
	if newRow > publicRecordRowCount {
		if err = BeginShakeUp(ctx, q, manager, game.ID); err != nil {
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
		updatedRankings, diff, rankErr := runRankingUpdate(ctx, q, game.ID)
		if rankErr == nil {
			if h != nil {
				h.BroadcastEvent(model.EventRankingsUpdated, model.RankingsUpdatedPayload{Rankings: updatedRankings})
			}
			EmitRankingUpdated(ctx, q, manager, game.ID, newRow, diff)
		}
	}

	if h != nil {
		h.BroadcastEvent(model.EventRowAdvanced, model.RowAdvancedPayload{
			RowNumber:        newRow,
			CrossedEngrailed: crossed,
		})
	}
	EmitSystemPost(ctx, q, manager, game.ID, "row.advanced",
		model.SeverityBoundary,
		fmt.Sprintf("Row %d begins", newRow), &newRow, nil, nil,
		map[string]any{"row_number": newRow, "crossed_engrailed": crossed})

	broadcastRowState(ctx, q, manager, game.ID)
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
		// "+ Add to public record" button. Only kind='turn' scenes are ended
		// by the focus player this way — a plan-scene closes on its own when
		// the plan resolves (closePlanSceneIfAny).
		ctx := r.Context()
		activeScene, activeErr := loadActiveScene(ctx, s.Q, game.ID)
		if activeErr == nil && activeScene != nil && activeScene.Kind == model.SceneKindPlan {
			respondErr(w, http.StatusConflict,
				"the active scene belongs to a resolving plan and can only end when that plan resolves")
			return
		}

		var endedSceneID int64
		if active := activeScene; activeErr == nil && active != nil {
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
			fmt.Sprintf("%s ends the scene", playerMark(player.ID, player.DisplayName)),
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

		// Refreshing is the other half of step 5, so it ends the turn — which a
		// player whose declaration is still waiting on its delay reveal has not
		// finished taking. Refuse until it settles, or the paused turn buys both
		// a plan and a refresh (and the deferred pass in passFocusAfterDelayReveal
		// would move a marker this call already moved).
		if plans, pErr := s.Q.ListPlansByGame(r.Context(), game.ID); pErr == nil {
			if dr := openDelayRevealPlanFor(plans, player.ID); dr != nil {
				respondErr(w, http.StatusConflict,
					"your "+planLabel(dr.PlanType)+" is still waiting on its delay reveal — "+
						"you cannot refresh assets until it settles")
				return
			}
		}

		var body struct {
			AssetIDs []int64 `json:"asset_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		// An empty list is a valid "refresh nothing" action, and deliberately so:
		// it is the focus player's guaranteed way to end a turn in which they can
		// do nothing else (every plan tile greyed out, no leveraged assets), so
		// no board state can strand the table. The work is all in autoPassFocus
		// below. The UI surfaces it as the "Refresh 0 Assets" button.
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

		refreshed := make([]dbgen.Asset, 0, len(body.AssetIDs))
		for _, id := range body.AssetIDs {
			if err := s.Q.RefreshPlayerAssets(ctx, id); err != nil {
				respondInternalErr(w, r, "could not refresh asset", err)
				return
			}
			if hasHub {
				h.BroadcastEvent(model.EventAssetRefreshed, model.AssetIDPayload{AssetID: id})
			}
			if asset, err := s.Q.GetAssetByID(ctx, id); err == nil {
				refreshed = append(refreshed, asset)
			}
		}
		if len(refreshed) > 0 {
			EmitAssetsRefreshedBatch(ctx, s.Q, manager, game.ID, player.ID, refreshed, game.CurrentRow)
		}

		// Refreshing assets is the focus player's step-5 action; pass the
		// focus marker automatically so refresh is a one-click commit. The
		// primary action has already committed, so a failure here is logged
		// and recovered via the manual /pass-focus endpoint rather than
		// failing the request.
		if err := autoPassFocus(r, s, manager, game); err != nil {
			loggerFromContext(ctx).ErrorContext(ctx, "auto pass-focus after refresh-assets", "err", err)
		}

		respond(w, http.StatusOK, map[string]any{"refreshed": body.AssetIDs})
	}
}

// currentGamePhase re-reads a game's phase from the DB. Used after a row
// advance that crossed row 13, so the response reports what the game
// actually transitioned into rather than assuming; falls back to
// model.PhaseShakeUp (the only transition advanceRowInner performs today) if
// the reload fails.
func currentGamePhase(ctx context.Context, q *dbgen.Queries, gameID int64) model.GamePhase {
	game, err := q.GetGameByID(ctx, gameID)
	if err != nil {
		return model.PhaseShakeUp
	}
	return game.Phase
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

	// Soft conditions that block row advance: pending plans, outstanding
	// war costs, or open surrender claims. Each check is conservative —
	// any DB error is treated as "skip row advance" so we never advance
	// past a state we couldn't verify is clear. Failures are logged but
	// not returned: focus has already moved, and the next player's own
	// autoPassFocus re-evaluates the advance from scratch.
	logger := loggerFromContext(ctx)

	// Decide row advance from the PRE-kickoff plan state. broadcastRowState
	// auto-kicks-off a pending plan (pending → resolving) as a side effect; if
	// we broadcast before this check, a freshly-kicked-off plan would no longer
	// count as "pending" and the row could advance past an unresolved plan.
	pending, err := s.Q.ListPendingPlansByRow(ctx, dbgen.ListPendingPlansByRowParams{
		GameID:    game.ID,
		RowNumber: new(game.CurrentRow),
	})
	if err != nil {
		logger.WarnContext(ctx, "auto pass-focus: could not list pending plans; skipping row advance", "err", err)
		broadcastRowState(ctx, s.Q, manager, game.ID)
		return nil
	}
	if len(pending) > 0 {
		// Plans remain — the new focus player resolves the next one. The
		// broadcast auto-kicks it off now that focus has moved on from the
		// just-resolved plan's follow-scene turn.
		broadcastRowState(ctx, s.Q, manager, game.ID)
		return nil
	}

	if rowAdvanceBlockReason(ctx, s.Q, manager, game) != "" {
		// Unpaid battle costs, open surrender claims, or the endgame vote just
		// opened — the row holds. The caller's primary action still committed;
		// PassFocus recovers (and the vote's own resolution performs the advance).
		broadcastRowState(ctx, s.Q, manager, game.ID)
		return nil
	}

	newRow, ended, err := advanceRowInner(ctx, s.Q, manager, h, game)
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
func PassFocus(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, player, ok := requireFocusPlayer(w, r, s.Q)
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

		// A caller whose own delay reveal is still open must not pass focus. The
		// reveal decides whether their declaration lands on a row at all, and if
		// it falls through (adr/ENDGAME_VOTE_AND_FINALE_PLAN.md §6) the preparer
		// is owed the chance to prepare something else — which a pass would have
		// spent. The row-advance gate already holds the ROW for an open reveal;
		// this holds the TURN.
		if plans, pErr := s.Q.ListPlansByGame(ctx, game.ID); pErr == nil {
			if dr := openDelayRevealPlanFor(plans, player.ID); dr != nil {
				respondErr(w, http.StatusConflict,
					"your "+planLabel(dr.PlanType)+" is still waiting on its delay reveal — "+
						"you cannot pass focus until it settles")
				return
			}
		}

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

		// Step 7: are there pending plans still on this row? Check BEFORE
		// broadcasting row state: broadcastRowState auto-kicks-off a pending
		// plan (pending → resolving) as a side effect, which would otherwise
		// hide it from this check and let the row advance past an unresolved
		// plan.
		pending, err := s.Q.ListPendingPlansByRow(ctx, dbgen.ListPendingPlansByRowParams{
			GameID:    game.ID,
			RowNumber: new(game.CurrentRow),
		})
		if err != nil {
			// Non-fatal: pass focus succeeded; the row simply holds and the
			// next player's pass re-evaluates it. Log so a persistent DB
			// issue here doesn't silently stall row advance forever.
			loggerFromContext(
				ctx,
			).WarnContext(ctx, "pass-focus: could not list pending plans; skipping row advance", "err", err)
			broadcastRowState(ctx, s.Q, manager, game.ID)
			respond(w, http.StatusOK, map[string]any{
				"focus_player_id":   next.ID,
				"focus_player_name": next.DisplayName,
			})
			return
		}

		if len(pending) > 0 {
			// Plans remain — new focus player will resolve the next one. The
			// broadcast auto-kicks it off now that focus has moved on from the
			// just-resolved plan's follow-scene turn. No row advance yet.
			broadcastRowState(ctx, s.Q, manager, game.ID)
			respond(w, http.StatusOK, map[string]any{
				"focus_player_id":   next.ID,
				"focus_player_name": next.DisplayName,
			})
			return
		}

		// Step 8: no plans remain — advance the row automatically, unless an
		// active war still has unpaid battle costs for the current row, an
		// open surrender claim, or (at the row 7 → 8 boundary) the endgame vote
		// this pass opens. rowAdvanceBlockReason is conservative on a DB error:
		// it reports the row as blocked rather than advancing it on state we
		// could not verify.
		if reason := rowAdvanceBlockReason(ctx, s.Q, manager, game); reason != "" {
			broadcastRowState(ctx, s.Q, manager, game.ID)
			respond(w, http.StatusOK, map[string]any{
				"focus_player_id":   next.ID,
				"focus_player_name": next.DisplayName,
				"advance_blocked":   reason,
			})
			return
		}

		// Focus stays with `next` (they carry it into the new row).
		newRow, ended, err := advanceRowInner(ctx, s.Q, manager, h, game)
		if err != nil {
			// Row advance failed after focus already moved — not ideal, but
			// focus.changed was already broadcast so respond with what we have.
			respond(w, http.StatusOK, map[string]any{
				"focus_player_id":   next.ID,
				"focus_player_name": next.DisplayName,
				"advance_error":     "could not advance the row; it will retry on the next pass",
			})
			return
		}

		if ended {
			// "ended" means the row advance crossed row 13 — today that
			// always lands in Shake-Up (see advanceRowInner), never a true
			// game-over state, so report the game's actual current phase
			// rather than assuming.
			respond(w, http.StatusOK, map[string]any{
				"focus_player_id":   next.ID,
				"focus_player_name": next.DisplayName,
				"phase":             currentGamePhase(ctx, s.Q, game.ID),
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

// rowAdvanceBlockReason reports why the current row cannot advance yet, or ""
// when it is clear to advance. A non-empty result is a player-facing message
// for the "advance_blocked" response field. A war must have all battle costs
// for the current row paid and all surrender claims taken before the row may
// advance. DB-check failures are logged and treated conservatively (the row is
// reported as blocked) rather than advancing the row on unverified state; the
// next player's pass re-runs these checks.
//
// The endgame-vote check comes LAST and is the one branch here with a side
// effect: it OPENS the vote (see endingVoteBlockReason). Running it after every
// other check is what makes ComputeRowState's near-top vote gate safe — the flag
// can only be set on a row with nothing else in flight. game is passed by
// pointer because that branch updates it in place.
//
// Both callers — autoPassFocus and PassFocus — pick this up automatically; there
// is no third row-advance path in production. (POST /tables/{id}/advance-row was
// removed per adr/FACILITATOR_POWERS_AUDIT.md; the dev-only DevAdvanceRow
// deliberately skips these gates as a test fast-forward, so a dev-seeded game
// parked past row 8 simply has ending_mode null and the finale rules inert.)
func rowAdvanceBlockReason(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	game *dbgen.Game,
) string {
	logger := loggerFromContext(ctx)
	gameID, currentRow := game.ID, game.CurrentRow

	// An open delay reveal (Make War / Clandestinely Liaise) holds the row
	// until every participant has submitted their die. Such a plan has a NULL
	// row_number, so it is invisible to the ListPendingPlansByRow(currentRow)
	// check the callers run before this gate — without this branch the row
	// advances straight past the vote, dropping the declaration's delay roll.
	plans, err := q.ListPlansByGame(ctx, gameID)
	if err != nil {
		logger.WarnContext(
			ctx,
			"row-advance gate: could not list plans for delay reveal check; skipping advance",
			"err",
			err,
		)
		return "could not verify delay reveals; the row holds until the next pass"
	}
	if openDelayRevealPlan(plans) != nil {
		return "a pending war declaration or liaison is still awaiting its delay reveal"
	}

	outstanding, err := mwOutstandingCostsForGame(ctx, q, gameID, currentRow)
	if err != nil {
		logger.WarnContext(ctx, "row-advance gate: could not check outstanding war costs; skipping advance", "err", err)
		return "could not verify war costs; the row holds until the next pass"
	}
	if len(outstanding) > 0 {
		return "outstanding battle costs must be paid before the row can advance"
	}

	claims, err := mwOutstandingSurrenderClaimsForGame(ctx, q, gameID)
	if err != nil {
		logger.WarnContext(ctx, "row-advance gate: could not check surrender claims; skipping advance", "err", err)
		return "could not verify surrender claims; the row holds until the next pass"
	}
	if len(claims) > 0 {
		return "opposing players must take an asset from each surrendered player before the row can advance"
	}

	return endingVoteBlockReason(ctx, q, manager, game)
}
