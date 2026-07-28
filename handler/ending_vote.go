package handler

// handler/ending_vote.go — the endgame-mode table vote
// (adr/ENDGAME_VOTE_AND_FINALE_PLAN.md).
//
// The rulebook triggers the ending choice the first time a plan would overflow
// row 13. That interrupts whoever thought it was their turn and fights the async
// model, so the choice is pinned instead to the row 7 → 8 boundary: row 7 is the
// last row from which every plan still fits (max fixed delay is Host Festivity's
// 6, and 7 + 6 = 13), and at row 8 the only plan that overflows is Host
// Festivity — so committing at that boundary costs the table almost no
// information.
//
// The vote occupies the same kind of slot as the engrailed-line ranking update:
// the gap *inside* a row advance, between one row ending and the next beginning.
// The ranking update runs there synchronously; the vote can't, because it needs
// input, so it PAUSES the advance, occupies the gap, and lets the advance
// complete once it resolves. Nothing about row 8 has started while the vote is
// up; row 7 is over. Hence every vote-related log post lands on row 7, and the
// "Row 8 begins" post follows the resolution in its normal place.
//
// Two routes, no third. An earlier draft proposed a facilitator-only
// /ending-vote/close "escape hatch for an absent player";
// adr/FACILITATOR_POWERS_AUDIT.md rejected it. Every player must vote and there
// is no way to proceed without them — no timeout, no skip, no force-close. A
// table where someone has gone quiet has a social problem, and the software's
// job is to name it clearly (the Waiting On bar and the notification system),
// not to route around it. The tie-break is an automatic rule applied to a set of
// fully cast votes, not a power anyone exercises.

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// endgameVoteRow is the row whose advance is gated on the ending vote. The
// vote opens when this row's advance is otherwise clear and no ending mode has
// been settled; current_row stays at this value for the vote's whole duration.
const endgameVoteRow = 7

// endingModePreferenceOrder is clause 3 of the tie-break: among tied leaders
// that the facilitator did not vote for, the one that asks LEAST of the table
// wins, in this fixed order.
//
// The ordering is principled rather than arbitrary, which matters because it has
// to be explainable to a table that just lost a vote to it. Smooth Landing asks
// least — you simply cannot prepare a plan that doesn't fit; it is also the
// status quo, since it is what every game silently plays today. Explosive Finale
// asks more — everyone gets a bonus plan and row 13 gets busy. Long Campaign
// asks most by a wide margin: another whole session, "more than double the
// length of your game" per GAME_END_RULES.md. A deadlocked table should not be
// committed to the largest obligation by a coin flip.
var endingModePreferenceOrder = []string{
	EndingModeSmoothLanding,
	EndingModeExplosiveFinale,
	EndingModeLongCampaign,
}

// endingVote is the pure-domain shape the tally reads: one player, one mode.
// Kept separate from dbgen.EndingVote so tallyEndingVote stays a pure function
// that unit tests can drive with any ballot — including a three-option one that
// the DB's own CHECK constraint would refuse.
type endingVote struct {
	PlayerID int64
	Mode     string
}

// tallyEndingVote resolves a fully cast ballot to a single ending mode. It is a
// rule, not a power: applied automatically the moment every seated player has
// voted, taking no action by anyone.
//
//  1. Highest count wins.
//  2. On a tie, the tied option the facilitator voted for wins.
//  3. If the facilitator's option is not among the tied leaders, the tied leader
//     that asks least of the table wins, in endingModePreferenceOrder.
//
// Clause 3 is unreachable with today's two modes: a tie then requires an even
// roster split down the middle, so the facilitator is necessarily on one side
// and clause 2 always fires. It exists — and is unit-tested against a
// hypothetical three-option ballot — so Long Campaign inherits a tested
// tie-break rather than an untested one. With three options the facilitator can
// be on neither side: five players splitting 2 smooth / 2 explosive / 1 long,
// where the facilitator cast the Long Campaign vote.
//
// Returns "" only for an empty ballot; callers never tally one (resolution is
// gated on every seated player having voted).
func tallyEndingVote(votes []endingVote, facilitatorID int64) string {
	if len(votes) == 0 {
		return ""
	}

	counts := map[string]int{}
	facilitatorMode := ""
	for _, v := range votes {
		counts[v.Mode]++
		if v.PlayerID == facilitatorID {
			facilitatorMode = v.Mode
		}
	}

	// Clause 1: highest count.
	best := 0
	for _, c := range counts {
		if c > best {
			best = c
		}
	}
	leaders := map[string]bool{}
	for mode, c := range counts {
		if c == best {
			leaders[mode] = true
		}
	}
	if len(leaders) == 1 {
		for mode := range leaders {
			return mode
		}
	}

	// Clause 2: the facilitator's side of the tie. A facilitator who did not
	// vote (facilitatorMode "") never matches, and falls through to clause 3.
	if leaders[facilitatorMode] {
		return facilitatorMode
	}

	// Clause 3: the tied leader that asks least of the table.
	for _, mode := range endingModePreferenceOrder {
		if leaders[mode] {
			return mode
		}
	}

	// Unreachable: the route validates the mode and the ending_votes CHECK
	// bounds the column, so every leader is in the order above. Kept total
	// anyway — returning the lexicographically first tied leader is at least
	// deterministic, where falling out of a map range would not be.
	rest := make([]string, 0, len(leaders))
	for mode := range leaders {
		rest = append(rest, mode)
	}
	sort.Strings(rest)
	return rest[0]
}

// endingModeLabel renders a mode for player-facing text. Falls back to the raw
// value so an unknown mode still reads as something rather than vanishing.
func endingModeLabel(mode string) string {
	switch mode {
	case EndingModeSmoothLanding:
		return "a Smooth Landing"
	case EndingModeExplosiveFinale:
		return "an Explosive Finale"
	case EndingModeLongCampaign:
		return "a Long Campaign"
	}
	return mode
}

// endingModeConsequence spells out what the settled mode means in play, so the
// resolution log post is self-explanatory to a table seeing this once.
func endingModeConsequence(mode string) string {
	switch mode {
	case EndingModeSmoothLanding:
		return "no plan may be prepared that would land past row 13."
	case EndingModeExplosiveFinale:
		return "each player may prepare one plan that lands on row 13 instead of its own row, " +
			"and row 13 resolves plan after plan with no scenes between."
	case EndingModeLongCampaign:
		return "play continues onto a second public-record sheet."
	}
	return "the game will end under this mode."
}

// endingVotePendingVoters names every seated player who has not yet voted —
// RowStateAwaitEndgameVote's ActingPlayerIDs, and the "who hasn't" half the
// Waiting On bar needs.
func endingVotePendingVoters(ctx context.Context, q *dbgen.Queries, gameID int64) ([]int64, error) {
	players, err := q.GetPlayersByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	votes, err := q.ListEndingVotesByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	voted := make(map[int64]bool, len(votes))
	for _, v := range votes {
		voted[v.PlayerID] = true
	}
	var ids []int64
	for _, p := range players {
		if !voted[p.ID] {
			ids = append(ids, p.ID)
		}
	}
	return ids, nil
}

// endingVoteBlockReason is the row-advance gate's vote check: when the row
// endgameVoteRow → endgameVoteRow+1 advance is otherwise clear and no ending
// mode has been settled, it opens the vote (idempotently), emits the boundary
// log post, and blocks the advance.
//
// Called LAST inside rowAdvanceBlockReason, and that ordering is load-bearing:
// ComputeRowState puts the vote gate near the top of its precedence chain, which
// is only safe if the flag cannot be set while something else is still in
// flight. Running after the delay-reveal, battle-cost and surrender-claim checks
// guarantees exactly that. (No new battle cost can fall due during the vote
// either — costs become due at the start of a row, and the row is what's paused.)
//
// game is updated in place so a caller that keeps reading it sees the open vote.
func endingVoteBlockReason(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	game *dbgen.Game,
) string {
	if game.CurrentRow != endgameVoteRow || game.EndingMode != nil {
		return ""
	}
	if game.EndingVoteOpen {
		return "the table is voting on how the game ends"
	}

	if err := q.SetEndingVoteOpen(ctx, dbgen.SetEndingVoteOpenParams{
		ID: game.ID, EndingVoteOpen: true,
	}); err != nil {
		loggerFromContext(ctx).WarnContext(ctx,
			"row-advance gate: could not open the ending vote; skipping advance", "err", err)
		return "could not open the ending vote; the row holds until the next pass"
	}
	game.EndingVoteOpen = true

	EmitSystemPost(ctx, q, manager, game.ID, "endgame.vote_opened",
		model.SeverityBoundary,
		"Row 7 is complete, and the public record is running out of rows. "+
			"Before row 8 begins, everyone at the table votes on how the game ends.",
		logRow(*game), nil, nil,
		map[string]any{"row_number": game.CurrentRow})

	return "the table is voting on how the game ends"
}

// endingVoteResponse is the shape both routes return: the window state, every
// vote cast so far (votes are public — this is a table conversation, not a
// secret ballot), and the settled mode once there is one.
type endingVoteResponse struct {
	Open  bool              `json:"open"`
	Votes []endingVoteEntry `json:"votes"`
	Mode  *string           `json:"mode"`
	// PendingPlayerIDs is the same set RowStateAwaitEndgameVote carries,
	// restated here so a client polling this endpoint alone can render "still
	// waiting on" without a second request. Always an array, never null — like
	// Votes, and unlike RowState.ActingPlayerIDs, which is omitempty.
	PendingPlayerIDs []int64 `json:"pending_player_ids"`
}

type endingVoteEntry struct {
	PlayerID int64  `json:"player_id"`
	Mode     string `json:"mode"`
}

// loadEndingVoteState assembles the response shape from a freshly-read game.
func loadEndingVoteState(ctx context.Context, q *dbgen.Queries, game *dbgen.Game) (endingVoteResponse, error) {
	votes, err := q.ListEndingVotesByGame(ctx, game.ID)
	if err != nil {
		return endingVoteResponse{}, err
	}
	pending, err := endingVotePendingVoters(ctx, q, game.ID)
	if err != nil {
		return endingVoteResponse{}, err
	}
	if pending == nil {
		pending = []int64{}
	}
	entries := make([]endingVoteEntry, len(votes))
	for i, v := range votes {
		entries[i] = endingVoteEntry{PlayerID: v.PlayerID, Mode: v.Mode}
	}
	return endingVoteResponse{
		Open:             game.EndingVoteOpen,
		Votes:            entries,
		Mode:             game.EndingMode,
		PendingPlayerIDs: pending,
	}, nil
}

// GetEndingVote handles GET /api/tables/{id}/ending-vote.
//
// Any seated player may read it. Votes are public — who voted and for what,
// both — because it is a table conversation, and the Waiting On bar needs the
// "who hasn't" half regardless.
func GetEndingVote(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		game, err := s.Q.GetGameByID(r.Context(), gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}
		state, err := loadEndingVoteState(r.Context(), s.Q, &game)
		if err != nil {
			respondInternalErr(w, r, "could not load the ending vote", err)
			return
		}
		respond(w, http.StatusOK, state)
	}
}

// CastEndingVote handles POST /api/tables/{id}/ending-vote.
//
// Body: {"mode": "smooth_landing" | "explosive_finale"}. Any seated player;
// upsert, so a player may change their vote while the window is open. 409 unless
// games.ending_vote_open — that flag is the single window authority, so there is
// no early voting and no late voting.
//
// After every cast vote, if every seated player has voted, the tally runs
// immediately: it sets ending_mode, closes the window, and performs the deferred
// row advance inline.
func CastEndingVote(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		game, err := s.Q.GetGameByID(r.Context(), gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}
		if !game.EndingVoteOpen {
			respondErr(w, http.StatusConflict,
				"the table is not voting on how the game ends right now")
			return
		}

		var body struct {
			Mode string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		switch body.Mode {
		case EndingModeSmoothLanding, EndingModeExplosiveFinale:
			// allowed
		case EndingModeLongCampaign:
			respondErr(w, http.StatusBadRequest, "long_campaign is not yet implemented")
			return
		default:
			respondErr(w, http.StatusBadRequest, "mode must be smooth_landing or explosive_finale")
			return
		}

		ctx := r.Context()

		// What did this player have on record? Read before the upsert, so the log
		// post can distinguish a first vote from a change of mind — and stay silent
		// on a re-cast of the same mode, which is a no-op an impatient double-tap
		// should not spam the log with.
		var priorMode *string
		priorVotes, err := s.Q.ListEndingVotesByGame(ctx, gameID)
		if err != nil {
			respondInternalErr(w, r, "could not load the ending vote", err)
			return
		}
		for _, v := range priorVotes {
			if v.PlayerID == player.ID {
				priorMode = &v.Mode
				break
			}
		}

		if err := s.Q.CastEndingVote(ctx, dbgen.CastEndingVoteParams{
			GameID: gameID, PlayerID: player.ID, Mode: body.Mode,
		}); err != nil {
			respondInternalErr(w, r, "could not record your vote", err)
			return
		}

		broadcastEvent(manager, gameID, model.EventEndgameVoteCast,
			model.EndgameVoteCastPayload{PlayerID: player.ID, Mode: body.Mode})

		if priorMode == nil || *priorMode != body.Mode {
			verb := "votes for"
			if priorMode != nil {
				verb = "changes their vote to"
			}
			EmitSystemPost(ctx, s.Q, manager, gameID, "endgame.vote_cast",
				model.SeverityDefault,
				playerDisplayName(ctx, s.Q, player.ID)+" "+verb+" "+endingModeLabel(body.Mode)+".",
				logRow(game), nil, nil,
				map[string]any{"player_id": player.ID, "mode": body.Mode})
		}

		// Resolve if the ballot is now complete. This is best-effort past the point
		// where the vote itself committed: the caller's vote stands either way, and
		// the response reports the state as it actually is. It also re-runs on a
		// no-op re-cast, which is the recovery path if an earlier resolution failed.
		if err := resolveEndingVoteIfComplete(r, s, manager, &game); err != nil {
			loggerFromContext(ctx).ErrorContext(ctx, "could not resolve the ending vote",
				"game_id", gameID, "err", err)
		}

		broadcastRowState(ctx, s.Q, manager, gameID)

		fresh, err := s.Q.GetGameByID(ctx, gameID)
		if err != nil {
			respondInternalErr(w, r, "could not reload the table", err)
			return
		}
		state, err := loadEndingVoteState(ctx, s.Q, &fresh)
		if err != nil {
			respondInternalErr(w, r, "could not load the ending vote", err)
			return
		}
		respond(w, http.StatusOK, state)
	}
}

// resolveEndingVoteIfComplete tallies the ballot and closes the vote once every
// seated player has voted, then performs the row advance the blocked PassFocus
// would have made.
//
// No-op while any player still owes a vote — that wait is the whole point, and
// nothing shortens it.
func resolveEndingVoteIfComplete(
	r *http.Request,
	s *db.Store,
	manager *hub.Manager,
	game *dbgen.Game,
) error {
	ctx := r.Context()

	players, err := s.Q.GetPlayersByGame(ctx, game.ID)
	if err != nil {
		return err
	}
	votes, err := s.Q.ListEndingVotesByGame(ctx, game.ID)
	if err != nil {
		return err
	}
	cast := make(map[int64]string, len(votes))
	for _, v := range votes {
		cast[v.PlayerID] = v.Mode
	}
	ballot := make([]endingVote, 0, len(players))
	var facilitatorID int64
	for _, p := range players {
		mode, voted := cast[p.ID]
		if !voted {
			return nil // still waiting on someone; nothing to do
		}
		if p.IsFacilitator {
			facilitatorID = p.ID
		}
		ballot = append(ballot, endingVote{PlayerID: p.ID, Mode: mode})
	}
	if len(ballot) == 0 {
		return nil
	}

	mode := tallyEndingVote(ballot, facilitatorID)
	if mode == "" {
		return nil
	}

	if err := s.Q.SetEndingMode(ctx, dbgen.SetEndingModeParams{
		ID: game.ID, EndingMode: &mode,
	}); err != nil {
		return err
	}
	if err := s.Q.SetEndingVoteOpen(ctx, dbgen.SetEndingVoteOpenParams{
		ID: game.ID, EndingVoteOpen: false,
	}); err != nil {
		return err
	}
	game.EndingMode = &mode
	game.EndingVoteOpen = false

	broadcastEvent(manager, game.ID, model.EventEndgameModeSet,
		model.EndgameModeSetPayload{Mode: mode})
	EmitSystemPost(ctx, s.Q, manager, game.ID, "endgame.mode_set",
		model.SeverityBoundary,
		"The table settles on "+endingModeLabel(mode)+": "+endingModeConsequence(mode),
		logRow(*game), nil, nil,
		map[string]any{"mode": mode})

	// Perform the deferred advance — the same advanceRowInner call the blocked
	// PassFocus would have made — so the "Row 8 begins" post and every
	// downstream event fire in their normal order, just later.
	//
	// No re-run of rowAdvanceBlockReason first: the vote only opened once every
	// other block was clear, and nothing that could re-block has been possible
	// since (no turns exist during the vote, preparation is refused, and battle
	// costs fall due at the start of a row, which is exactly what is paused).
	h, _ := manager.Get(game.ID)
	newRow, ended, err := advanceRowInner(r, s.Q, manager, h, game)
	if err != nil {
		return err
	}
	if !ended {
		mwBroadcastBattleCostsDue(ctx, s.Q, manager, game.ID, newRow)
	}
	return nil
}
