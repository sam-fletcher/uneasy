//go:build integration

// handler/dev_delete_game_integration_test.go — proves DeleteGame (backed by the
// ON DELETE CASCADE foreign keys from migration 039) removes a game and every
// one of its child rows in one statement, without touching other games.
package handler

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/gametest"
	"uneasy/model"
)

// countByGame returns SELECT count(*) FROM <table> WHERE game_id = $1.
func countByGame(t *testing.T, pool *pgxpool.Pool, table string, gameID int64) int {
	t.Helper()
	var n int
	// table is a fixed literal from the test, not user input.
	err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM "+table+" WHERE game_id = $1", gameID).Scan(&n)
	require.NoError(t, err)
	return n
}

func TestDeleteGame_CascadesAndIsolates(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	// Game under test, populated across several child tables: seedBase gives it
	// players, rankings, public_record_rows and per-player assets; add a plan too.
	target := newTestGame(t, q, 3,
		gametest.WithPlan(gametest.SeedPlan{
			PreparerIdx: 0, PlanType: model.PlanMakeWar,
			Category: model.CategoryPower, Row: 1, RowOrder: 0,
		}),
	)
	// A second game acts as a control — deleting `target` must not touch it.
	control := newTestGame(t, q, 2)

	// Tables the seed populates; each must be non-empty before, empty after.
	populated := []string{"players", "assets", "rankings", "public_record_rows", "plans"}
	for _, tbl := range populated {
		require.Positivef(t, countByGame(t, pool, tbl, target.Game.ID),
			"%s should be populated before delete", tbl)
	}

	rows, err := q.DeleteGame(ctx, target.Game.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, rows, "exactly one game deleted")

	// The game row itself is gone.
	_, err = q.GetGameByID(ctx, target.Game.ID)
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	// Every child table is empty for the deleted game (cascade reached them).
	for _, tbl := range populated {
		assert.Zerof(t, countByGame(t, pool, tbl, target.Game.ID),
			"%s should be empty after cascade delete", tbl)
	}

	// The control game is untouched.
	_, err = q.GetGameByID(ctx, control.Game.ID)
	require.NoError(t, err)
	assert.Positive(t, countByGame(t, pool, "players", control.Game.ID),
		"control game's players must survive deletion of another game")
	assert.Positive(t, countByGame(t, pool, "assets", control.Game.ID),
		"control game's assets must survive deletion of another game")
}

func TestDeleteGame_MissingIDDeletesNothing(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	// A never-created id deletes zero rows — the handler maps this to 404.
	rows, err := q.DeleteGame(context.Background(), 999_999_999)
	require.NoError(t, err)
	assert.EqualValues(t, 0, rows)
}
