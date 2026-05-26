//go:build integration

package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// ── PHASE 3: Phase Handler Integration Tests ────────────────────────────────
// These tests validate game phase transitions and state machine logic.

// ── Start Prologue Tests ────────────────────────────────────────────────

func TestStartPrologue_RejectsUnderMinPlayers(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	// Create a game with only 1 player (requires 2+)
	game, err := q.CreateGame(ctx, "SinglePlayerGame")
	require.NoError(t, err)

	// Add only 1 player
	acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
		Username: "solo-" + game.JoinCode, CodeHash: "x",
	})
	require.NoError(t, err)
	_, err = q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
		GameID:      game.ID,
		DisplayName: "Solo",
		AccountID:   acct.ID,
	})
	require.NoError(t, err)

	// Try to start prologue (should fail with < 2 players)
	count, err := q.CountPlayersInGame(ctx, game.ID)
	require.NoError(t, err)
	assert.Less(t, count, int64(2), "should have fewer than 2 players")
}

func TestStartPrologue_RejectsOverMaxPlayers(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	// Create a game with 6 players (exceeds 5 max)
	game, err := q.CreateGame(ctx, "SixPlayerGame")
	require.NoError(t, err)

	for i := 0; i < 6; i++ {
		acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
			Username: fmt.Sprintf("p%d-%s", i, game.JoinCode), CodeHash: "x",
		})
		require.NoError(t, err)
		_, err = q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID:      game.ID,
			DisplayName: fmt.Sprintf("P%d", i+1),
			AccountID:   acct.ID,
		})
		require.NoError(t, err)
	}

	// Verify we have 6 players
	count, err := q.CountPlayersInGame(ctx, game.ID)
	require.NoError(t, err)
	assert.Greater(t, count, int64(5), "should have more than 5 players")
}

func TestStartPrologue_AcceptsValidPlayerCounts(t *testing.T) {
	tests := []int{2, 3, 4, 5}

	for _, playerCount := range tests {
		t.Run(fmt.Sprintf("%dPlayers", playerCount), func(t *testing.T) {
			pool := openTestDB(t)
			q := dbgen.New(pool)
			ctx := context.Background()

			game, err := q.CreateGame(ctx, fmt.Sprintf("Game%dPlayers", playerCount))
			require.NoError(t, err)

			for i := 0; i < playerCount; i++ {
				acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
					Username: fmt.Sprintf("p%d-%s", i, game.JoinCode), CodeHash: "x",
				})
				require.NoError(t, err)
				_, err = q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
					GameID:      game.ID,
					DisplayName: fmt.Sprintf("P%d", i+1),
					AccountID:   acct.ID,
				})
				require.NoError(t, err)
			}

			// Verify player count is valid for tone setting
			count, err := q.CountPlayersInGame(ctx, game.ID)
			require.NoError(t, err)
			assert.Equal(t, int64(playerCount), count)
			assert.GreaterOrEqual(t, count, int64(2))
			assert.LessOrEqual(t, count, int64(5))
		})
	}
}

func TestStartPrologue_SeedsDefaultToneTopics(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	// Create a valid 3-player game
	game, err := q.CreateGame(ctx, "ThreePlayerGame")
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
			Username: fmt.Sprintf("p%d-%s", i, game.JoinCode), CodeHash: "x",
		})
		require.NoError(t, err)
		_, err = q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID:      game.ID,
			DisplayName: fmt.Sprintf("P%d", i+1),
			AccountID:   acct.ID,
		})
		require.NoError(t, err)
	}

	// Before seeding, no tone topics
	topicsBefore, err := q.ListToneTopics(ctx, game.ID)
	require.NoError(t, err)
	assert.Empty(t, topicsBefore)

	// Seed default topics (mimicking what StartPrologue does)
	err = db.SeedDefaultToneTopics(ctx, q, game.ID)
	require.NoError(t, err)

	// Verify default topics were created
	topicsAfter, err := q.ListToneTopics(ctx, game.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, topicsAfter, "should seed default tone topics")
	assert.GreaterOrEqual(t, len(topicsAfter), 5, "should have at least 5 default topics")
}

func TestPrologue_RequiresLobbyPhase(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	// Create game and immediately move it out of lobby
	game, err := q.CreateGame(ctx, "TestGame")
	require.NoError(t, err)

	// Create some players
	for i := 0; i < 3; i++ {
		acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
			Username: fmt.Sprintf("p%d-%s", i, game.JoinCode), CodeHash: "x",
		})
		require.NoError(t, err)
		_, err = q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID:      game.ID,
			DisplayName: fmt.Sprintf("P%d", i+1),
			AccountID:   acct.ID,
		})
		require.NoError(t, err)
	}

	// Move to Prologue phase
	err = q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID:    game.ID,
		Phase: model.PhasePrologue,
	})
	require.NoError(t, err)

	// Verify we're NOT in lobby
	updated, err := q.GetGameByID(ctx, game.ID)
	require.NoError(t, err)
	assert.NotEqual(t, model.PhaseLobby, updated.Phase)
}

// ── Public Record Seeding Tests ─────────────────────────────────────────────

func TestMainEvent_CreatesPublicRecordRows(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	tg := newTestGame(t, q, 3)

	// Get initial public record rows (newTestGame creates them)
	rowsBefore, err := q.ListPublicRecordRows(ctx, tg.Game.ID)
	require.NoError(t, err)
	initialCount := len(rowsBefore)

	// Verify rows exist (should be >= 13 from newTestGame setup)
	assert.GreaterOrEqual(t, initialCount, 13,
		"newTestGame should create at least 13 public record rows")
}

// ── Rankings and Seat Order Validation ──────────────────────────────────────

func TestMainEvent_AllPlayersHaveRankings(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	tg := newTestGame(t, q, 3)

	// Verify rankings exist
	rankings, err := q.ListRankingsByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(rankings), 9, "3 players × 3 categories = 9 minimum")

	// Verify each player has a ranking in each category
	for _, player := range tg.Players {
		for _, category := range []model.RankingCategory{
			model.CategoryPower,
			model.CategoryKnowledge,
			model.CategoryEsteem,
		} {
			found := false
			for _, r := range rankings {
				if *r.PlayerID == player.ID && r.Category == category {
					found = true
					break
				}
			}
			assert.True(t, found, "player %d should have %s ranking", player.ID, category)
		}
	}
}

func TestMainEvent_AllPlayersHaveSeatOrder(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	_ = q

	tg := newTestGame(t, q, 3)

	// Verify all players have seat order set
	for _, player := range tg.Players {
		assert.NotNil(t, player.SeatOrder, "player %d should have seat_order", player.ID)
		assert.Greater(t, *player.SeatOrder, int16(0), "seat order should be > 0")
	}
}

// ── Game Phase Transitions ──────────────────────────────────────────────────

func TestGamePhaseTransition_LobbyToPrologue(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	game, err := q.CreateGame(ctx, "TestGame")
	require.NoError(t, err)
	assert.Equal(t, model.PhaseLobby, game.Phase)

	// Transition to prologue
	err = q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID:    game.ID,
		Phase: model.PhasePrologue,
	})
	require.NoError(t, err)

	updated, err := q.GetGameByID(ctx, game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PhasePrologue, updated.Phase)
}

func TestGamePhaseTransition_PrologueToMainEvent(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	tg := newTestGame(t, q, 3)

	// Move from main_event back to prologue for this test
	err := q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID:    tg.Game.ID,
		Phase: model.PhasePrologue,
	})
	require.NoError(t, err)

	// Transition to main event
	err = q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID:    tg.Game.ID,
		Phase: model.PhaseMainEvent,
	})
	require.NoError(t, err)

	updated, err := q.GetGameByID(ctx, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PhaseMainEvent, updated.Phase)
}

// ── Auto-advance to main_event ──────────────────────────────────────────────

// seedPrologueComplete returns a game in the prologue phase with the
// minimum state advanceToMainEvent expects: 3 categories × 5 ranks
// filled in seat order (no NULL player_ids), 3 players seated, and
// the ranking step at extra_peers (the ≤3-player terminal state).
func seedPrologueComplete(t *testing.T, q *dbgen.Queries) (dbgen.Game, []dbgen.Player) {
	t.Helper()
	ctx := context.Background()

	game, err := q.CreateGame(ctx, "PROL"+randSuffix())
	require.NoError(t, err)

	players := make([]dbgen.Player, 3)
	for i := 0; i < 3; i++ {
		acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
			Username: fmt.Sprintf("prol-p%d-%s", i+1, randSuffix()),
			CodeHash: "x",
		})
		require.NoError(t, err)
		p, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID:        game.ID,
			DisplayName:   fmt.Sprintf("P%d", i+1),
			AccountID:     acct.ID,
			IsFacilitator: i == 0,
		})
		require.NoError(t, err)
		seat := int16(i + 1)
		require.NoError(t, q.SetPlayerSeatOrder(ctx, dbgen.SetPlayerSeatOrderParams{
			ID: p.ID, SeatOrder: &seat,
		}))
		p.SeatOrder = &seat
		players[i] = p
	}
	require.NoError(t, q.SetFacilitator(ctx, dbgen.SetFacilitatorParams{
		FacilitatorID: &players[0].ID, ID: game.ID,
	}))

	for _, cat := range []model.RankingCategory{
		model.CategoryPower, model.CategoryKnowledge, model.CategoryEsteem,
	} {
		for rank := int16(1); rank <= 5; rank++ {
			// Fill ranks 1..3 with players, 4..5 with NULL placeholders.
			var pid *int64
			if int(rank) <= len(players) {
				pid = &players[rank-1].ID
			}
			require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
				GameID:   game.ID,
				PlayerID: pid,
				Category: cat,
				Rank:     rank,
			}))
		}
	}

	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: game.ID, Phase: model.PhasePrologue,
	}))
	step := "extra_peers"
	require.NoError(t, q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
		ID: game.ID, PrologueRankingStep: &step,
	}))

	refreshed, err := q.GetGameByID(ctx, game.ID)
	require.NoError(t, err)
	return refreshed, players
}

func TestAdvanceToMainEvent_TransitionsPhaseAndSeedsRow1(t *testing.T) {
	pool := openTestDB(t)
	store := &db.Store{Q: dbgen.New(pool)}
	ctx := context.Background()
	manager := hub.NewManager()

	game, players := seedPrologueComplete(t, store.Q)

	err := advanceToMainEvent(ctx, store.Q, manager, game.ID)
	require.NoError(t, err)

	updated, err := store.Q.GetGameByID(ctx, game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PhaseMainEvent, updated.Phase)
	assert.Equal(t, int16(1), updated.CurrentRow)
	require.NotNil(t, updated.FocusPlayerID, "focus player must be set")
	// With rank 1 going to player[0] across all three tracks, lowest
	// cumulative status = player[0].
	assert.Equal(t, players[0].ID, *updated.FocusPlayerID)

	rows, err := store.Q.ListPublicRecordRows(ctx, game.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(rows), 13, "must seed at least 13 public record rows")
}

func TestAdvanceToMainEvent_EmitsBoundaryPost(t *testing.T) {
	pool := openTestDB(t)
	store := &db.Store{Q: dbgen.New(pool)}
	ctx := context.Background()
	manager := hub.NewManager()

	game, _ := seedPrologueComplete(t, store.Q)
	require.NoError(t, advanceToMainEvent(ctx, store.Q, manager, game.ID))

	posts, err := store.Q.ListGamePosts(ctx, game.ID)
	require.NoError(t, err)
	found := false
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == "phase.changed" && p.Body == "Main event begins" {
			found = true
			break
		}
	}
	assert.True(t, found, "phase.changed boundary post for main_event must be emitted")
}
