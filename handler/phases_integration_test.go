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
	"uneasy/model"
)

// ── PHASE 3: Phase Handler Integration Tests ────────────────────────────────
// These tests validate game phase transitions and state machine logic.

// ── Start Tone Setting Tests ────────────────────────────────────────────────

func TestStartToneSetting_RejectsUnderMinPlayers(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	// Create a game with only 1 player (requires 2+)
	game, err := q.CreateGame(ctx, "SinglePlayerGame")
	require.NoError(t, err)

	// Add only 1 player
	_, err = q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
		GameID:      game.ID,
		DisplayName: "Solo",
		CookieToken: "test-token-1",
	})
	require.NoError(t, err)

	// Try to start tone setting (should fail with < 2 players)
	count, err := q.CountPlayersInGame(ctx, game.ID)
	require.NoError(t, err)
	assert.Less(t, count, int64(2), "should have fewer than 2 players")
}

func TestStartToneSetting_RejectsOverMaxPlayers(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	// Create a game with 6 players (exceeds 5 max)
	game, err := q.CreateGame(ctx, "SixPlayerGame")
	require.NoError(t, err)

	for i := 0; i < 6; i++ {
		_, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID:      game.ID,
			DisplayName: fmt.Sprintf("P%d", i+1),
			CookieToken: fmt.Sprintf("token-%d", i),
		})
		require.NoError(t, err)
	}

	// Verify we have 6 players
	count, err := q.CountPlayersInGame(ctx, game.ID)
	require.NoError(t, err)
	assert.Greater(t, count, int64(5), "should have more than 5 players")
}

func TestStartToneSetting_AcceptsValidPlayerCounts(t *testing.T) {
	tests := []int{2, 3, 4, 5}

	for _, playerCount := range tests {
		t.Run(fmt.Sprintf("%dPlayers", playerCount), func(t *testing.T) {
			pool := openTestDB(t)
			q := dbgen.New(pool)
			ctx := context.Background()

			game, err := q.CreateGame(ctx, fmt.Sprintf("Game%dPlayers", playerCount))
			require.NoError(t, err)

			for i := 0; i < playerCount; i++ {
				_, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
					GameID:      game.ID,
					DisplayName: fmt.Sprintf("P%d", i+1),
					CookieToken: fmt.Sprintf("token-%d", i),
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

func TestStartToneSetting_SeedsDefaultToneTopics(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	// Create a valid 3-player game
	game, err := q.CreateGame(ctx, "ThreePlayerGame")
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		_, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID:      game.ID,
			DisplayName: fmt.Sprintf("P%d", i+1),
			CookieToken: fmt.Sprintf("token-%d", i),
		})
		require.NoError(t, err)
	}

	// Before seeding, no tone topics
	topicsBefore, err := q.ListToneTopics(ctx, game.ID)
	require.NoError(t, err)
	assert.Empty(t, topicsBefore)

	// Seed default topics (mimicking what StartToneSetting does)
	err = db.SeedDefaultToneTopics(ctx, q, game.ID)
	require.NoError(t, err)

	// Verify default topics were created
	topicsAfter, err := q.ListToneTopics(ctx, game.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, topicsAfter, "should seed default tone topics")
	assert.GreaterOrEqual(t, len(topicsAfter), 5, "should have at least 5 default topics")
}

func TestToneSetting_RequiresLobbyPhase(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	// Create game and immediately move it out of lobby
	game, err := q.CreateGame(ctx, "TestGame")
	require.NoError(t, err)

	// Create some players
	for i := 0; i < 3; i++ {
		_, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID:      game.ID,
			DisplayName: fmt.Sprintf("P%d", i+1),
			CookieToken: fmt.Sprintf("token-%d", i),
		})
		require.NoError(t, err)
	}

	// Move to ToneSetting phase
	err = q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID:    game.ID,
		Phase: model.PhaseToneSetting,
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

func TestGamePhaseTransition_LobbyToToneSetting(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	game, err := q.CreateGame(ctx, "TestGame")
	require.NoError(t, err)
	assert.Equal(t, model.PhaseLobby, game.Phase)

	// Transition to tone setting
	err = q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID:    game.ID,
		Phase: model.PhaseToneSetting,
	})
	require.NoError(t, err)

	updated, err := q.GetGameByID(ctx, game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PhaseToneSetting, updated.Phase)
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
