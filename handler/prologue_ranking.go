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
// players; track resolution (in prologue_committed_hearts.go) advances
// directly to the next declare step.
//
// The main_event transition is automatic: completing the last prologue
// action (final track's resolution/place-set-asides for 4–5 players, or the
// last extra peer for ≤3 players) immediately calls advanceToMainEvent —
// no facilitator button.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"uneasy/db"
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

// ── Place set-asides ─────────────────────────────────────────────────────────

// PlaceSetAsides handles POST /api/games/{id}/prologue/place-set-asides.
//
// Body: {"ordering": [player_id, ...]}.
//
// Caller must be the top-ranked player on the current track. The ordering must
// be a permutation of the set-aside players (those not yet given a
// rankings.rank for this track). Server appends them to the rankings, then
// advances to the next declare step (or extra-peers / done).
//
//nolint:gocognit,gocyclo,cyclop,funlen // place set-asides validates auth, permutation, and advances the step machine
func PlaceSetAsides(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		ctx := r.Context()
		game := loadGameForPrologue(w, ctx, s.Q, gameID)
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

		// Caller must be the player at the top of this track. Per PROLOGUE_RULES
		// step 6, "the player at the top of each track adds the set-aside players"
		// — that's the highest-status *real* player, i.e. the non-dummy ranking
		// with the lowest rank number. We can't assume rank 1: in 2–3 player games
		// dummy tokens occupy rank 1 (see gamepkg.DummyRanks), so the top
		// player there sits at rank 2.
		rankings, err := s.Q.ListRankingsByGame(ctx, gameID)
		if err != nil {
			respondInternalErr(w, r, "could not load rankings", err)
			return
		}
		cat := modelCategoryForTrack(track)
		var topPlayer *int64
		var topRank int16
		for _, rk := range rankings {
			if rk.Category != cat || rk.PlayerID == nil {
				continue
			}
			if topPlayer == nil || rk.Rank < topRank {
				topPlayer = rk.PlayerID
				topRank = rk.Rank
			}
		}
		if topPlayer == nil || *topPlayer != player.ID {
			respondErr(w, http.StatusForbidden, "only the track's top-ranked player can place set-asides")
			return
		}

		// Set-asides = players in this game with no ranking on this track.
		players, err := s.Q.GetPlayersByGame(ctx, gameID)
		if err != nil {
			respondInternalErr(w, r, "could not load players", err)
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
		for _, rank := range gamepkg.OpenRanks(len(players)) {
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
			err = s.Q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
				GameID: gameID, PlayerID: &pidPtr, Category: cat, Rank: open[i],
			})
			if err != nil {
				respondInternalErr(w, r, "could not set ranking", err)
				return
			}
		}

		// Insert dummy rows for any rank slot still empty (only happens
		// if the auto-ranked + set-aside count is still less than open
		// slots, which shouldn't, but kept here for safety).
		if err = fillDummyRanks(ctx, s.Q, gameID, cat, len(players)); err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Build the final ordering for broadcast.
		updated, _ := s.Q.ListRankingsByGame(ctx, gameID)
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
		EmitPrologueTrackRanked(ctx, s.Q, manager, gameID, track)

		// Advance step.
		nextStep := nextDeclareStepAfter(track)
		if nextStep == "" && len(players) <= 3 {
			nextStep = gamepkg.PrologueStepExtraPeers
		}
		if err = setRankingStep(ctx, s.Q, gameID, nextStep); err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		broadcastEvent(manager, gameID, model.EventPrologueRankingStepChanged,
			model.PrologueRankingStepChangedPayload{Step: nextStep})

		// 4–5 player game finishing the last track's set-asides: prologue complete.
		if nextStep == "" {
			if err := advanceToMainEvent(ctx, s.Q, manager, gameID); err != nil {
				respondInternalErr(w, r, "could not advance to main event", err)
				return
			}
		}

		respond(w, http.StatusOK, map[string]any{"track": track, "next_step": nextStep})
	}
}

// ── Extra peers (≤3 players) ─────────────────────────────────────────────────

// insertExtraPeerWithTitle creates the extra peer, records the extra-peer claim,
// and stamps the claimed title on a marginalia of the peer (ADR-007). The title
// lives on the marginalia so currentMonarch — which reads marginalia.title
// across all assets, not just main characters — can find an extra-peer
// monarch/heir in ≤3-player games. Claiming the monarch title trips the
// throne_established gate. Runs inside the caller's transaction.
func insertExtraPeerWithTitle(
	ctx context.Context,
	q *dbgen.Queries,
	gameID, playerID int64,
	titleName, peerText, titleID string,
) (dbgen.Asset, dbgen.Marginalium, error) {
	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:    gameID,
		OwnerID:   playerID,
		CreatorID: playerID,
		AssetType: model.AssetPeer,
		Name:      peerText,
	})
	if err != nil {
		return dbgen.Asset{}, dbgen.Marginalium{}, errors.New("could not create extra peer")
	}
	if _, err := q.InsertExtraPeer(ctx, dbgen.InsertExtraPeerParams{
		GameID: gameID, PlayerID: playerID, TitleName: titleName, AssetID: asset.ID,
	}); err != nil {
		return dbgen.Asset{}, dbgen.Marginalium{}, errors.New("could not record extra peer")
	}
	marg, err := q.CreateTitleMarginalia(ctx, dbgen.CreateTitleMarginaliaParams{
		AssetID:  asset.ID,
		Position: 1,
		Text:     titleName,
		Title:    &titleID,
	})
	if err != nil {
		return dbgen.Asset{}, dbgen.Marginalium{}, errors.New("could not stamp extra peer title")
	}
	if titleID == gamepkg.TitleMonarch {
		if err := q.EstablishThrone(ctx, gameID); err != nil {
			return dbgen.Asset{}, dbgen.Marginalium{}, errors.New("could not establish the throne")
		}
	}
	return asset, marg, nil
}

// CreateExtraPeer handles POST /api/games/{id}/prologue/extra-peer.
//
// Body: {"title_name": "..."}. Creates one additional peer asset for the
// caller, named after an unused title from the titles sheet. Each player
// may create exactly one extra peer; each title may only be claimed once
// (across both the choosing-phase and extra-peer flows). Available only
// during the extra_peers step.
func CreateExtraPeer(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		ctx := r.Context()
		game := loadGameForPrologue(w, ctx, s.Q, gameID)
		if game == nil {
			return
		}
		if !requirePrologueStep(w, game, gamepkg.PrologueStepExtraPeers) {
			return
		}

		var body struct {
			TitleName string `json:"title_name"`
			PeerText  string `json:"peer_text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		peerText, ok := textField(w, "peer_text", body.PeerText, maxAssetNameLen)
		if !ok {
			return
		}
		body.PeerText = peerText
		if body.PeerText == "" {
			respondErr(w, http.StatusBadRequest, "peer_text is required")
			return
		}
		choice := gamepkg.FindPrologueChoice(gamepkg.PrologueSheetTitles, body.TitleName)
		if choice == nil {
			respondErr(w, http.StatusBadRequest, "unknown title")
			return
		}
		alreadyDone, err := s.Q.ExtraPeerExistsForPlayer(ctx, dbgen.ExtraPeerExistsForPlayerParams{
			GameID: gameID, PlayerID: player.ID,
		})
		if err != nil {
			respondInternalErr(w, r, "could not check player status", err)
			return
		}
		if alreadyDone {
			respondErr(w, http.StatusConflict, "you have already created your extra peer")
			return
		}
		choosingClaimed, err := s.Q.PrologueChoiceClaimed(ctx, dbgen.PrologueChoiceClaimedParams{
			GameID: gameID, SheetType: gamepkg.PrologueSheetTitles, ChoiceName: body.TitleName,
		})
		if err != nil {
			respondInternalErr(w, r, "could not check title status", err)
			return
		}
		extraClaimed, err := s.Q.ExtraPeerTitleClaimed(ctx, dbgen.ExtraPeerTitleClaimedParams{
			GameID: gameID, TitleName: body.TitleName,
		})
		if err != nil {
			respondInternalErr(w, r, "could not check title status", err)
			return
		}
		if choosingClaimed || extraClaimed {
			respondErr(w, http.StatusConflict, "title is already claimed")
			return
		}

		var asset dbgen.Asset
		var titleMarg dbgen.Marginalium
		err = s.InTx(ctx, func(q *dbgen.Queries) error {
			var iErr error
			asset, titleMarg, iErr = insertExtraPeerWithTitle(
				ctx,
				q,
				gameID,
				player.ID,
				body.TitleName,
				body.PeerText,
				choice.ID,
			)
			return iErr
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		broadcastEvent(
			manager,
			gameID,
			model.EventAssetCreated,
			model.AssetPayload{Asset: assetWithMarginalia{Asset: asset, Marginalia: []dbgen.Marginalium{titleMarg}}},
		)
		EmitAssetCreated(ctx, s.Q, manager, gameID, asset, []dbgen.Marginalium{titleMarg}, nil)
		broadcastEvent(manager, gameID, model.EventPrologueExtraPeerCreated,
			model.PrologueExtraPeerCreatedPayload{
				PlayerID: player.ID, TitleName: body.TitleName, AssetID: asset.ID,
			})

		if err := maybeAdvanceAfterExtraPeer(ctx, s.Q, manager, gameID); err != nil {
			respondInternalErr(w, r, "could not advance to main event", err)
			return
		}

		respond(w, http.StatusOK, map[string]any{"asset": asset})
	}
}

// maybeAdvanceAfterExtraPeer transitions to main_event once every player
// in the game has created their extra peer. No-op if peers are still
// missing.
func maybeAdvanceAfterExtraPeer(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
) error {
	players, err := q.GetPlayersByGame(ctx, gameID)
	if err != nil {
		return fmt.Errorf("load players: %w", err)
	}
	extras, err := q.ListExtraPeersByGame(ctx, gameID)
	if err != nil {
		return fmt.Errorf("load extra peers: %w", err)
	}
	if len(extras) < len(players) {
		return nil
	}
	return advanceToMainEvent(ctx, q, manager, gameID)
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

	open := gamepkg.OpenRanks(playerCount)
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
	dummies := gamepkg.DummyRanks(playerCount)
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
