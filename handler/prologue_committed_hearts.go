package handler

// handler/prologue_committed_hearts.go — Endpoints for the
// "max-commitment" prologue ranking flow (replaces DeclareHearts /
// FinalizeTrackRanking once the frontend migrates).
//
// Players commit specific heart cards to the active track and toggle a
// per-track "Done" flag. When every player is Done, the server resolves
// the track: bright (necessary) hearts lock in; grey (wasted) hearts are
// refunded by deleting their committed-hearts rows so they're available
// for the next track. The set-aside flow (rank-1 player slotting in
// zero-suit players) is unchanged and still uses PlaceSetAsides.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// CommittedHeartView is the JSON shape returned by ranking-state.
type CommittedHeartView struct {
	PlayerID int64  `json:"player_id"`
	Track    string `json:"track"`
	CardID   int64  `json:"card_id"`
	Value    string `json:"value"`
	Suit     string `json:"suit"`
}

type TrackDoneView struct {
	PlayerID int64  `json:"player_id"`
	Track    string `json:"track"`
	Done     bool   `json:"done"`
}

type ExtraPeerView struct {
	PlayerID  int64  `json:"player_id"`
	TitleName string `json:"title_name"`
	AssetID   int64  `json:"asset_id"`
}

type PrologueRankingState struct {
	Committed  []CommittedHeartView `json:"committed"`
	Done       []TrackDoneView      `json:"done"`
	ExtraPeers []ExtraPeerView      `json:"extra_peers"`
}

// GetPrologueRankingState handles GET /api/tables/{id}/prologue/ranking-state.
// Returns the full per-player commitment + Done state for the game.
func GetPrologueRankingState(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, q)
		if !ok {
			return
		}
		ctx := r.Context()
		committed, err := q.ListCommittedHeartsByGame(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load committed hearts")
			return
		}
		done, err := q.ListTrackDoneByGame(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load done flags")
			return
		}
		extras, err := q.ListExtraPeersByGame(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load extra peers")
			return
		}
		commit := make([]CommittedHeartView, 0, len(committed))
		for _, c := range committed {
			commit = append(commit, CommittedHeartView{
				PlayerID: c.PlayerID, Track: c.Track, CardID: c.CardID,
				Value: c.CardValue, Suit: c.CardSuit,
			})
		}
		doneViews := make([]TrackDoneView, 0, len(done))
		for _, d := range done {
			doneViews = append(doneViews, TrackDoneView{
				PlayerID: d.PlayerID, Track: d.Track, Done: d.Done,
			})
		}
		extraViews := make([]ExtraPeerView, 0, len(extras))
		for _, e := range extras {
			extraViews = append(extraViews, ExtraPeerView{
				PlayerID: e.PlayerID, TitleName: e.TitleName, AssetID: e.AssetID,
			})
		}
		respond(w, http.StatusOK, PrologueRankingState{
			Committed: commit, Done: doneViews, ExtraPeers: extraViews,
		})
	}
}

// CommitTrackHearts handles POST /api/tables/{id}/prologue/committed-hearts.
//
// Body: {"track": "power", "card_ids": [101, 102]}.
//
// Replaces the caller's committed hearts for the active track. Each
// card must be a heart owned by the caller and not already locked into
// a previously-resolved track. Adjusting commitments un-readies the
// caller (Done → false).
//
//nolint:gocognit // Legitimate validation: track state, ownership, card suit, locking constraints
func CommitTrackHearts(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
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
		var body struct {
			Track   string  `json:"track"`
			CardIDs []int64 `json:"card_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !isDeclareableTrack(body.Track) {
			respondErr(w, http.StatusBadRequest, "invalid track")
			return
		}
		if *game.PrologueRankingStep != "declare_"+body.Track {
			respondErr(w, http.StatusConflict, "track is not currently being declared")
			return
		}
		// Validate every card.
		for _, cid := range body.CardIDs {
			card, err := q.GetPlayerCardByID(ctx, cid)
			if err != nil || card.GameID != gameID {
				respondErr(w, http.StatusBadRequest, "unknown card")
				return
			}
			if card.PlayerID != player.ID {
				respondErr(w, http.StatusForbidden, "card does not belong to you")
				return
			}
			if card.CardSuit != "H" {
				respondErr(w, http.StatusBadRequest, "only hearts may be committed")
				return
			}
		}
		// Reject re-committing cards already locked into a previously-
		// resolved track (these stay in the table after resolution).
		existing, err := q.ListCommittedHeartsByGame(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load existing commitments")
			return
		}
		for _, ch := range existing {
			if !inInt64s(body.CardIDs, ch.CardID) {
				continue
			}
			if ch.Track != body.Track && trackResolved(ch.Track, *game.PrologueRankingStep) {
				respondErr(w, http.StatusConflict, "card is locked into a previously-resolved track")
				return
			}
		}

		// Replace this player's commitments on this track.
		keep := map[int64]bool{}
		for _, cid := range body.CardIDs {
			keep[cid] = true
		}
		for _, ch := range existing {
			if ch.PlayerID == player.ID && ch.Track == body.Track && !keep[ch.CardID] {
				if err := q.UncommitHeart(ctx, dbgen.UncommitHeartParams{
					GameID: gameID, CardID: ch.CardID,
				}); err != nil {
					respondErr(w, http.StatusInternalServerError, "could not uncommit heart")
					return
				}
			}
		}
		for _, cid := range body.CardIDs {
			if err := q.CommitHeart(ctx, dbgen.CommitHeartParams{
				GameID: gameID, PlayerID: player.ID, Track: body.Track, CardID: cid,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not commit heart")
				return
			}
		}
		// Adjusting commitments un-readies the player.
		if err := q.SetTrackDone(ctx, dbgen.SetTrackDoneParams{
			GameID: gameID, PlayerID: player.ID, Track: body.Track, Done: false,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not reset done")
			return
		}

		broadcastEvent(manager, gameID, model.EventPrologueCommittedHeartsChg,
			model.PrologueCommittedHeartsChangedPayload{
				PlayerID: player.ID, Track: body.Track, CardIDs: body.CardIDs,
			})
		broadcastEvent(manager, gameID, model.EventPrologueDoneChanged,
			model.PrologueDoneChangedPayload{
				PlayerID: player.ID, Track: body.Track, Done: false,
			})
		respond(w, http.StatusOK, map[string]any{"track": body.Track, "card_ids": body.CardIDs})
	}
}

// SetPrologueDone handles POST /api/tables/{id}/prologue/done.
//
// Body: {"track": "power", "done": true}. If setting done=true causes
// every player to be Done for the active track, the server resolves it
// (computes bright/grey, persists rankings, refunds grey, advances step).
func SetPrologueDone(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
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
		var body struct {
			Track string `json:"track"`
			Done  bool   `json:"done"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !isDeclareableTrack(body.Track) {
			respondErr(w, http.StatusBadRequest, "invalid track")
			return
		}
		if *game.PrologueRankingStep != "declare_"+body.Track {
			respondErr(w, http.StatusConflict, "track is not currently being declared")
			return
		}
		if err := q.SetTrackDone(ctx, dbgen.SetTrackDoneParams{
			GameID: gameID, PlayerID: player.ID, Track: body.Track, Done: body.Done,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save done")
			return
		}
		broadcastEvent(manager, gameID, model.EventPrologueDoneChanged,
			model.PrologueDoneChangedPayload{
				PlayerID: player.ID, Track: body.Track, Done: body.Done,
			})

		if body.Done {
			allDone, err := allPlayersDoneForTrack(ctx, q, gameID, body.Track)
			if err != nil {
				respondErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			if allDone {
				if err := resolveTrack(ctx, q, manager, game, body.Track); err != nil {
					respondErr(w, http.StatusInternalServerError, err.Error())
					return
				}
			}
		}
		respond(w, http.StatusOK, map[string]any{"track": body.Track, "done": body.Done})
	}
}

// resolveTrack runs the bright/grey computation on the given track,
// persists the ranking, deletes grey commitments (refund), advances the
// step, and broadcasts events.
//
// Re-reads the game's prologue_ranking_step before doing any work so a
// concurrent SetPrologueDone request that already resolved this track
// is a no-op rather than a duplicate broadcast / persistence pass.
//
//nolint:funlen // Cohesive unit: recheck state, compute bright/grey, persist rankings, refund, advance step
func resolveTrack(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	game *dbgen.Game,
	track string,
) error {
	fresh, err := q.GetGameByID(ctx, game.ID)
	if err != nil {
		return err
	}
	if fresh.PrologueRankingStep == nil || *fresh.PrologueRankingStep != "declare_"+track {
		// Another request already resolved this track. Nothing to do.
		return nil
	}
	players, err := q.GetPlayersByGame(ctx, game.ID)
	if err != nil {
		return err
	}
	ids := make([]int64, len(players))
	for i, p := range players {
		ids[i] = p.ID
	}
	cards, err := loadPrologueCards(ctx, q, game.ID)
	if err != nil {
		return err
	}
	committed, err := loadCommittedHearts(ctx, q, game.ID)
	if err != nil {
		return err
	}

	bright, err := gamepkg.ComputeBrightHearts(track, ids, cards, committed)
	if err != nil {
		return err
	}

	brightCommitted := make([]gamepkg.CommittedHeart, 0, len(committed))
	greyCardIDs := []int64{}
	for _, h := range committed {
		if h.Track != track {
			brightCommitted = append(brightCommitted, h)
			continue
		}
		if bright[h.PlayerID][h.CardID] {
			brightCommitted = append(brightCommitted, h)
		} else {
			greyCardIDs = append(greyCardIDs, h.CardID)
		}
	}
	ranked, setAside, err := gamepkg.ComputeTrackRankingFromCommitments(track, ids, cards, brightCommitted)
	if err != nil {
		return err
	}

	// Single set-aside has no decision to make — auto-place so the
	// rank-1 player isn't prompted for a trivial choice.
	autoPlaced := len(setAside) == 1
	persistedSetAside := []int64(nil)
	if autoPlaced {
		persistedSetAside = setAside
	}
	if err := persistTrackRanks(ctx, q, game.ID, track, len(players), ranked, persistedSetAside); err != nil {
		return err
	}
	if len(greyCardIDs) > 0 {
		if err := q.DeleteCommittedHeartsByCardIDs(ctx, dbgen.DeleteCommittedHeartsByCardIDsParams{
			GameID: game.ID, CardIds: greyCardIDs,
		}); err != nil {
			return err
		}
	}
	if err := q.ResetTrackDone(ctx, dbgen.ResetTrackDoneParams{
		GameID: game.ID, Track: track,
	}); err != nil {
		return err
	}

	broadcastEvent(manager, game.ID, model.EventPrologueTrackRanked, model.PrologueTrackRankedPayload{
		Track: track, Ranked: ranked, SetAside: setAside,
	})
	if updated, lerr := q.ListRankingsByGame(ctx, game.ID); lerr == nil {
		broadcastEvent(manager, game.ID, model.EventRankingsUpdated,
			model.RankingsUpdatedPayload{Rankings: updated})
	}

	var nextStep string
	switch {
	case len(setAside) > 1:
		nextStep = placeSetAsidesStepFor(track)
	default:
		// Either no set-asides or exactly one (auto-placed above).
		nextStep = nextDeclareStepAfter(track)
		if nextStep == "" && len(players) <= 3 {
			nextStep = gamepkg.PrologueStepExtraPeers
		}
	}
	if err := setRankingStep(ctx, q, game.ID, nextStep); err != nil {
		return err
	}
	broadcastEvent(manager, game.ID, model.EventPrologueRankingStepChanged,
		model.PrologueRankingStepChangedPayload{Step: nextStep})
	return nil
}

func loadCommittedHearts(ctx context.Context, q *dbgen.Queries, gameID int64) ([]gamepkg.CommittedHeart, error) {
	rows, err := q.ListCommittedHeartsByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	out := make([]gamepkg.CommittedHeart, 0, len(rows))
	for _, r := range rows {
		out = append(out, gamepkg.CommittedHeart{
			PlayerID: r.PlayerID, Track: r.Track, CardID: r.CardID, Value: r.CardValue,
		})
	}
	return out, nil
}

func allPlayersDoneForTrack(ctx context.Context, q *dbgen.Queries, gameID int64, track string) (bool, error) {
	players, err := q.GetPlayersByGame(ctx, gameID)
	if err != nil {
		return false, err
	}
	if len(players) == 0 {
		return false, errors.New("no players")
	}
	rows, err := q.ListTrackDoneByGame(ctx, gameID)
	if err != nil {
		return false, err
	}
	doneSet := map[int64]bool{}
	for _, r := range rows {
		if r.Track == track && r.Done {
			doneSet[r.PlayerID] = true
		}
	}
	for _, p := range players {
		if !doneSet[p.ID] {
			return false, nil
		}
	}
	return true, nil
}

func isDeclareableTrack(t string) bool {
	return t == gamepkg.PrologueTrackPower ||
		t == gamepkg.PrologueTrackKnowledge ||
		t == gamepkg.PrologueTrackEsteem
}

// trackResolved reports whether `track` has already been finalized
// given the current ranking step.
func trackResolved(track, currentStep string) bool {
	seq := []string{
		gamepkg.PrologueTrackPower,
		gamepkg.PrologueTrackKnowledge,
		gamepkg.PrologueTrackEsteem,
	}
	currentIdx := -1
	for i, t := range seq {
		if currentStep == "declare_"+t || currentStep == "place_set_asides_"+t {
			currentIdx = i
			break
		}
	}
	if currentIdx == -1 {
		return true // past all tracks (extra_peers / done)
	}
	for i, t := range seq {
		if t == track {
			return i < currentIdx
		}
	}
	return false
}

func inInt64s(xs []int64, v int64) bool {
	return slices.Contains(xs, v)
}
