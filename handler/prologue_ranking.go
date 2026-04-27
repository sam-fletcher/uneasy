package handler

// handler/prologue_ranking.go — Ranking sub-flow endpoints (Phase 4b).
//
// State machine driven by games.prologue_ranking_step:
//
//   declare_power → place_set_asides_power → declare_knowledge →
//   place_set_asides_knowledge → declare_esteem → place_set_asides_esteem →
//   extra_peers (≤3 players) → main_event
//
// place_set_asides_X is skipped automatically if a track has no set-aside
// players; finalize-ranking advances directly to the next declare step.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// trackForStep returns the ranking track ("power"/"knowledge"/"esteem")
// associated with a declare_X or place_set_asides_X step, or "" otherwise.
func trackForStep(step string) string {
	switch step {
	case gamepkg.PrologueStepDeclarePower, gamepkg.PrologueStepPlaceSetAsidesPower:
		return gamepkg.PrologueTrackPower
	case gamepkg.PrologueStepDeclareKnowledge, gamepkg.PrologueStepPlaceSetAsidesKnow:
		return gamepkg.PrologueTrackKnowledge
	case gamepkg.PrologueStepDeclareEsteem, gamepkg.PrologueStepPlaceSetAsidesEsteem:
		return gamepkg.PrologueTrackEsteem
	}
	return ""
}

// modelCategoryForTrack converts a track string to the model.RankingCategory
// used by the rankings table.
func modelCategoryForTrack(track string) model.RankingCategory {
	switch track {
	case gamepkg.PrologueTrackPower:
		return model.CategoryPower
	case gamepkg.PrologueTrackKnowledge:
		return model.CategoryKnowledge
	case gamepkg.PrologueTrackEsteem:
		return model.CategoryEsteem
	}
	return ""
}

// nextDeclareStepAfter returns the next declare step after finishing track,
// or "" if there is no next track (i.e. esteem was the last).
func nextDeclareStepAfter(track string) string {
	switch track {
	case gamepkg.PrologueTrackPower:
		return gamepkg.PrologueStepDeclareKnowledge
	case gamepkg.PrologueTrackKnowledge:
		return gamepkg.PrologueStepDeclareEsteem
	}
	return ""
}

func placeSetAsidesStepFor(track string) string {
	switch track {
	case gamepkg.PrologueTrackPower:
		return gamepkg.PrologueStepPlaceSetAsidesPower
	case gamepkg.PrologueTrackKnowledge:
		return gamepkg.PrologueStepPlaceSetAsidesKnow
	case gamepkg.PrologueTrackEsteem:
		return gamepkg.PrologueStepPlaceSetAsidesEsteem
	}
	return ""
}

// dummyRanksForPlayerCount returns the ranks (1-indexed) occupied by dummy
// tokens given the player count. Per PROLOGUE_RULES.md:
//   - 5 players: no dummies
//   - 4 players: rank 3
//   - 3 players: ranks 1 and 5
//   - 2 players: ranks 1, 3, and 5
func dummyRanksForPlayerCount(n int) []int16 {
	switch n {
	case 4:
		return []int16{3}
	case 3:
		return []int16{1, 5}
	case 2:
		return []int16{1, 3, 5}
	}
	return nil
}

// openRanks returns ranks 1..5 with the dummy positions filtered out, in
// ascending order — i.e. the slots auto-ranked + set-aside players fill.
func openRanks(playerCount int) []int16 {
	dummies := dummyRanksForPlayerCount(playerCount)
	out := make([]int16, 0, 5)
	for r := int16(1); r <= 5; r++ {
		if !slices.Contains(dummies, r) {
			out = append(out, r)
		}
	}
	return out
}

// requirePrologueStep writes 409 and returns false unless the game is in the
// prologue phase at the expected ranking step.
func requirePrologueStep(w http.ResponseWriter, game *dbgen.Game, want string) bool {
	if game.Phase != model.PhasePrologue {
		respondErr(w, http.StatusConflict, "game is not in the prologue phase")
		return false
	}
	if game.PrologueRankingStep == nil || *game.PrologueRankingStep != want {
		respondErr(w, http.StatusConflict, "wrong ranking step")
		return false
	}
	return true
}

// loadGameForPrologue loads the game and returns nil on error (response
// already written).
func loadGameForPrologue(w http.ResponseWriter, ctx context.Context, q *dbgen.Queries, gameID int64) *dbgen.Game {
	game, err := q.GetGameByID(ctx, gameID)
	if err != nil {
		respondErr(w, http.StatusNotFound, "table not found")
		return nil
	}
	return &game
}

// ── Declare hearts ───────────────────────────────────────────────────────────

// DeclareHearts handles POST /api/games/{id}/prologue/declare-hearts.
//
// Body: {"count": N}. The current track is implied by the game's
// prologue_ranking_step. Validates that the player has enough remaining
// hearts (hearts held minus hearts already declared on other tracks). The
// per-track count overwrites any previous declaration on the same track.
func DeclareHearts(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, q)
		if !ok {
			return
		}
		ctx := r.Context()
		game := loadGameForPrologue(w, ctx, q, gameID)
		if game == nil {
			return
		}
		if game.Phase != model.PhasePrologue || game.PrologueRankingStep == nil {
			respondErr(w, http.StatusConflict, "ranking is not active")
			return
		}
		track := trackForStep(*game.PrologueRankingStep)
		if track == "" || *game.PrologueRankingStep != "declare_"+track {
			respondErr(w, http.StatusConflict, "current step is not a declare step")
			return
		}

		var body struct {
			Count int16 `json:"count"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Count < 0 {
			respondErr(w, http.StatusBadRequest, "count must be a non-negative integer")
			return
		}

		// Validate against this player's hearts held minus hearts spent on
		// other tracks.
		cards, err := q.ListPlayerCardsByPlayer(ctx, dbgen.ListPlayerCardsByPlayerParams{
			GameID: gameID, PlayerID: player.ID,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load cards")
			return
		}
		held := 0
		for _, c := range cards {
			if c.CardSuit == "H" {
				held++
			}
		}
		alreadyTotal, err := q.SumHeartDeclarationsByPlayer(ctx, dbgen.SumHeartDeclarationsByPlayerParams{
			GameID: gameID, PlayerID: player.ID,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load declarations")
			return
		}
		// Re-running for the same track replaces an existing declaration, so
		// subtract its previous value before checking.
		decls, err := q.ListHeartDeclarationsByGame(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load declarations")
			return
		}
		var prevForTrack int16
		for _, d := range decls {
			if d.PlayerID == player.ID && d.Track == track {
				prevForTrack = d.Count
				break
			}
		}
		if int(alreadyTotal)-int(prevForTrack)+int(body.Count) > held {
			respondErr(w, http.StatusBadRequest, "you do not hold that many remaining hearts")
			return
		}

		err = q.UpsertHeartDeclaration(ctx, dbgen.UpsertHeartDeclarationParams{
			GameID: gameID, PlayerID: player.ID, Track: track, Count: body.Count,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save declaration")
			return
		}
		broadcastEvent(manager, gameID, model.EventPrologueHeartsDeclared, model.PrologueHeartsDeclaredPayload{
			PlayerID: player.ID, Track: track, Count: body.Count,
		})
		respond(w, http.StatusOK, map[string]any{"track": track, "count": body.Count})
	}
}

// ── Finalize ranking ─────────────────────────────────────────────────────────

// FinalizeTrackRanking handles POST /api/games/{id}/prologue/finalize-ranking.
//
// Facilitator-only. Computes the ranking for the current track, persists the
// auto-ranked players to the rankings table, and either advances to the
// place_set_asides_X step (if any set-asides exist) or directly to the next
// declare step / extra-peers / main_event when set-asides are empty.
func FinalizeTrackRanking(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, ok := requireFacilitator(w, r, q)
		if !ok {
			return
		}
		if game.Phase != model.PhasePrologue || game.PrologueRankingStep == nil {
			respondErr(w, http.StatusConflict, "ranking is not active")
			return
		}
		track := trackForStep(*game.PrologueRankingStep)
		if track == "" || *game.PrologueRankingStep != "declare_"+track {
			respondErr(w, http.StatusConflict, "current step is not a declare step")
			return
		}

		ctx := r.Context()
		players, err := q.GetPlayersByGame(ctx, game.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load players")
			return
		}
		ids := make([]int64, len(players))
		for i, p := range players {
			ids[i] = p.ID
		}
		cards, err := loadPrologueCards(ctx, q, game.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load cards")
			return
		}
		decls, err := loadPrologueDeclarations(ctx, q, game.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load declarations")
			return
		}
		ranked, setAside, err := gamepkg.ComputeTrackRanking(track, ids, cards, decls)
		if err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}

		// Persist the auto-ranked portion now. Set-asides will be appended
		// once the rank-1 player submits an ordering (or immediately if
		// there are none).
		err = persistTrackRanks(ctx, q, game.ID, track, len(players), ranked, nil)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		broadcastEvent(manager, game.ID, model.EventPrologueTrackRanked, model.PrologueTrackRankedPayload{
			Track: track, Ranked: ranked, SetAside: setAside,
		})

		// Advance step. If no set-asides, skip place_set_asides_X.
		var nextStep string
		switch {
		case len(setAside) > 0:
			nextStep = placeSetAsidesStepFor(track)
		default:
			nextStep = nextDeclareStepAfter(track)
			if nextStep == "" {
				// Last track; either go to extra_peers (≤3 players) or
				// straight to main_event-ready (still in prologue phase
				// until facilitator calls start-main-event).
				if len(players) <= 3 {
					nextStep = gamepkg.PrologueStepExtraPeers
				}
			}
		}

		if err = setRankingStep(ctx, q, game.ID, nextStep); err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		broadcastEvent(manager, game.ID, model.EventPrologueRankingStepChanged,
			model.PrologueRankingStepChangedPayload{Step: nextStep})
		respond(w, http.StatusOK, map[string]any{
			"track":     track,
			"ranked":    ranked,
			"set_aside": setAside,
			"next_step": nextStep,
		})
	}
}

// ── Place set-asides ─────────────────────────────────────────────────────────

// PlaceSetAsides handles POST /api/games/{id}/prologue/place-set-asides.
//
// Body: {"ordering": [player_id, ...]}.
//
// Caller must be the rank-1 player on the current track. The ordering must
// be a permutation of the set-aside players (those not yet given a
// rankings.rank for this track). Server appends them to the rankings, then
// advances to the next declare step (or extra-peers / done).
//
//nolint:gocognit,gocyclo,cyclop,funlen // place set-asides validates auth, permutation, and advances the step machine
func PlaceSetAsides(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, q)
		if !ok {
			return
		}
		ctx := r.Context()
		game := loadGameForPrologue(w, ctx, q, gameID)
		if game == nil {
			return
		}
		if game.Phase != model.PhasePrologue || game.PrologueRankingStep == nil {
			respondErr(w, http.StatusConflict, "ranking is not active")
			return
		}
		track := trackForStep(*game.PrologueRankingStep)
		if track == "" || *game.PrologueRankingStep != placeSetAsidesStepFor(track) {
			respondErr(w, http.StatusConflict, "not currently placing set-asides")
			return
		}

		// Caller must be rank-1 on this track.
		rankings, err := q.ListRankingsByGame(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load rankings")
			return
		}
		cat := modelCategoryForTrack(track)
		var rank1 *int64
		for _, rk := range rankings {
			if rk.Category == cat && rk.Rank == 1 {
				rank1 = rk.PlayerID
				break
			}
		}
		if rank1 == nil || *rank1 != player.ID {
			respondErr(w, http.StatusForbidden, "only the track's rank-1 player can place set-asides")
			return
		}

		// Set-asides = players in this game with no ranking on this track.
		players, err := q.GetPlayersByGame(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load players")
			return
		}
		ranked := map[int64]bool{}
		for _, rk := range rankings {
			if rk.Category == cat && rk.PlayerID != nil {
				ranked[*rk.PlayerID] = true
			}
		}
		expected := []int64{}
		for _, p := range players {
			if !ranked[p.ID] {
				expected = append(expected, p.ID)
			}
		}

		var body struct {
			Ordering []int64 `json:"ordering"`
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !isPermutation(body.Ordering, expected) {
			respondErr(w, http.StatusBadRequest, "ordering must be a permutation of the set-aside players")
			return
		}

		// Find which open ranks remain unassigned for this track.
		taken := map[int16]bool{}
		for _, rk := range rankings {
			if rk.Category == cat {
				taken[rk.Rank] = true
			}
		}
		open := []int16{}
		for _, rank := range openRanks(len(players)) {
			if !taken[rank] {
				open = append(open, rank)
			}
		}
		if len(open) != len(body.Ordering) {
			respondErr(w, http.StatusInternalServerError, "rank slot count mismatch")
			return
		}
		for i, pid := range body.Ordering {
			pidPtr := pid
			err = q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
				GameID: gameID, PlayerID: &pidPtr, Category: cat, Rank: open[i],
			})
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not set ranking")
				return
			}
		}

		// Insert dummy rows for any rank slot still empty (only happens
		// if the auto-ranked + set-aside count is still less than open
		// slots, which shouldn't, but kept here for safety).
		if err = fillDummyRanks(ctx, q, gameID, cat, len(players)); err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Build the final ordering for broadcast.
		updated, _ := q.ListRankingsByGame(ctx, gameID)
		final := make([]int64, 0, 5)
		for r := int16(1); r <= 5; r++ {
			for _, rk := range updated {
				if rk.Category == cat && rk.Rank == r && rk.PlayerID != nil {
					final = append(final, *rk.PlayerID)
				}
			}
		}
		broadcastEvent(manager, gameID, model.EventPrologueSetAsidesPlaced, model.PrologueSetAsidesPlacedPayload{
			Track: track, Ranked: final,
		})
		broadcastEvent(manager, gameID, model.EventRankingsUpdated, model.RankingsUpdatedPayload{Rankings: updated})

		// Advance step.
		nextStep := nextDeclareStepAfter(track)
		if nextStep == "" && len(players) <= 3 {
			nextStep = gamepkg.PrologueStepExtraPeers
		}
		if err = setRankingStep(ctx, q, gameID, nextStep); err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		broadcastEvent(manager, gameID, model.EventPrologueRankingStepChanged,
			model.PrologueRankingStepChangedPayload{Step: nextStep})
		respond(w, http.StatusOK, map[string]any{"track": track, "next_step": nextStep})
	}
}

// ── Extra peers (≤3 players) ─────────────────────────────────────────────────

// CreateExtraPeer handles POST /api/games/{id}/prologue/extra-peer.
//
// Body: {"title_name": "..."}. Creates one additional peer asset for the
// caller, named after an unused title from the titles sheet. Available only
// during the extra_peers step.
func CreateExtraPeer(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, q)
		if !ok {
			return
		}
		ctx := r.Context()
		game := loadGameForPrologue(w, ctx, q, gameID)
		if game == nil {
			return
		}
		if !requirePrologueStep(w, game, gamepkg.PrologueStepExtraPeers) {
			return
		}

		var body struct {
			TitleName string `json:"title_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if gamepkg.FindPrologueChoice(gamepkg.PrologueSheetTitles, body.TitleName) == nil {
			respondErr(w, http.StatusBadRequest, "unknown title")
			return
		}
		// Title must be unused.
		claimed, err := q.PrologueChoiceClaimed(ctx, dbgen.PrologueChoiceClaimedParams{
			GameID: gameID, SheetType: gamepkg.PrologueSheetTitles, ChoiceName: body.TitleName,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not check title status")
			return
		}
		if claimed {
			respondErr(w, http.StatusBadRequest, "title was claimed during the prologue")
			return
		}

		asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID:    gameID,
			OwnerID:   player.ID,
			CreatorID: player.ID,
			AssetType: model.AssetPeer,
			Name:      body.TitleName,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create extra peer")
			return
		}
		broadcastEvent(manager, gameID, model.EventAssetCreated, model.AssetPayload{Asset: asset})
		respond(w, http.StatusOK, map[string]any{"asset": asset})
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func loadPrologueCards(ctx context.Context, q *dbgen.Queries, gameID int64) ([]gamepkg.PlayerCard, error) {
	rows, err := q.ListPlayerCardsByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	out := make([]gamepkg.PlayerCard, 0, len(rows))
	for _, r := range rows {
		if len(r.CardSuit) == 0 {
			continue
		}
		out = append(out, gamepkg.PlayerCard{
			PlayerID: r.PlayerID,
			Suit:     rune(r.CardSuit[0]),
			Value:    r.CardValue,
		})
	}
	return out, nil
}

func loadPrologueDeclarations(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
) ([]gamepkg.HeartDeclaration, error) {
	rows, err := q.ListHeartDeclarationsByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	out := make([]gamepkg.HeartDeclaration, 0, len(rows))
	for _, r := range rows {
		out = append(out, gamepkg.HeartDeclaration{
			PlayerID: r.PlayerID, Track: r.Track, Count: int(r.Count),
		})
	}
	return out, nil
}

// persistTrackRanks writes auto-ranked players to the open slots (skipping
// dummy positions). If setAside is non-nil, it's appended after the
// auto-ranked portion. Existing rankings for this track are cleared first.
func persistTrackRanks(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
	track string,
	playerCount int,
	autoRanked, setAside []int64,
) error {
	cat := modelCategoryForTrack(track)
	// Clear existing track rankings before writing — easier than reasoning
	// about partial upserts and the only "track-only" cleanup we need.
	if err := q.DeleteRankingsByCategory(ctx, dbgen.DeleteRankingsByCategoryParams{
		GameID: gameID, Category: cat,
	}); err != nil {
		return fmt.Errorf("clear ranking: %w", err)
	}

	open := openRanks(playerCount)
	all := append([]int64(nil), autoRanked...)
	all = append(all, setAside...)
	if len(all) > len(open) {
		return errors.New("too many players for open ranks")
	}
	for i, pid := range all {
		pidPtr := pid
		if err := q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
			GameID: gameID, PlayerID: &pidPtr, Category: cat, Rank: open[i],
		}); err != nil {
			return fmt.Errorf("set ranking: %w", err)
		}
	}
	if err := fillDummyRanks(ctx, q, gameID, cat, playerCount); err != nil {
		return err
	}
	return nil
}

// fillDummyRanks writes nil-player_id rows for every rank slot that's not
// already occupied — covers dummy positions for <5-player games. Called
// after the auto-ranked + set-aside writes are done.
func fillDummyRanks(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
	cat model.RankingCategory,
	playerCount int,
) error {
	rankings, err := q.ListRankingsByGame(ctx, gameID)
	if err != nil {
		return fmt.Errorf("load rankings: %w", err)
	}
	taken := map[int16]bool{}
	for _, rk := range rankings {
		if rk.Category == cat {
			taken[rk.Rank] = true
		}
	}
	dummies := dummyRanksForPlayerCount(playerCount)
	for r := int16(1); r <= 5; r++ {
		if taken[r] {
			continue
		}
		if !slices.Contains(dummies, r) {
			continue // not a dummy slot, leave empty (caller fills with set-asides)
		}
		err = q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
			GameID: gameID, PlayerID: nil, Category: cat, Rank: r,
		})
		if err != nil {
			return fmt.Errorf("set dummy ranking: %w", err)
		}
	}
	return nil
}

// setRankingStep writes step (or NULL if step == "") to
// games.prologue_ranking_step.
func setRankingStep(ctx context.Context, q *dbgen.Queries, gameID int64, step string) error {
	var ptr *string
	if step != "" {
		ptr = &step
	}
	err := q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
		ID: gameID, PrologueRankingStep: ptr,
	})
	if err != nil {
		return fmt.Errorf("set ranking step: %w", err)
	}
	return nil
}

// isPermutation returns true when xs is a reordering of ys (same multiset).
func isPermutation(xs, ys []int64) bool {
	if len(xs) != len(ys) {
		return false
	}
	count := map[int64]int{}
	for _, v := range ys {
		count[v]++
	}
	for _, v := range xs {
		count[v]--
		if count[v] < 0 {
			return false
		}
	}
	return true
}
