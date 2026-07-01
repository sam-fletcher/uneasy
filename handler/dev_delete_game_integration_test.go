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

// countByID returns SELECT count(*) FROM <table> WHERE <idCol> IN (ids...).
// Used for tables with no game_id column of their own — their only route
// back to the game is through a parent row's id (asset, plan, dice roll,
// duel stake, shake-up spend, secret).
func countByID(t *testing.T, pool *pgxpool.Pool, table, idCol string, ids []int64) int {
	t.Helper()
	if len(ids) == 0 {
		return 0
	}
	var n int
	// table/idCol are fixed literals from the test, not user input.
	err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM "+table+" WHERE "+idCol+" = ANY($1)", ids).Scan(&n)
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

	// Populate tables whose only foreign key path back to the game is
	// through a parent row's id rather than their own game_id column
	// (migration 041 fixed these — they used to have no cascade route to
	// `games` at all, so DeleteGame errored on a foreign key violation as
	// soon as one of them was non-empty; marginalia, created by every
	// Titles-sheet claim in the prologue, was the one real games always hit).
	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: target.Game.ID, OwnerID: target.Players[0].ID, CreatorID: target.Players[0].ID,
		AssetType: model.AssetHolding, Name: "landmine holding",
	})
	require.NoError(t, err)

	_, err = q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: asset.ID, Position: 1, Text: "a note",
	})
	require.NoError(t, err)

	secret, err := q.CreateSecret(ctx, dbgen.CreateSecretParams{
		AssetID: asset.ID, AuthorID: target.Players[0].ID, Text: "a secret",
	})
	require.NoError(t, err)
	require.NoError(t, q.AddSecretVisibility(ctx, dbgen.AddSecretVisibilityParams{
		SecretID: secret.ID, PlayerID: target.Players[1].ID,
	}))

	roll, err := q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
		GameID: target.Game.ID, ActorID: target.Players[0].ID, Difficulty: 3, Stage: "resolved",
	})
	require.NoError(t, err)
	_, err = q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
		RollID: roll.ID, PlayerID: target.Players[0].ID,
	})
	require.NoError(t, err)
	require.NoError(t, q.CreateDifficultyVote(ctx, dbgen.CreateDifficultyVoteParams{
		RollID: roll.ID, PlayerID: target.Players[1].ID, Vote: 1,
	}))

	duelPlan, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID: target.Game.ID, PlanType: model.PlanProposeDuel, Category: model.CategoryPower,
		PreparerID: target.Players[0].ID, RowNumber: intPtr(1), PreparedAtRow: 1,
	})
	require.NoError(t, err)
	stake, err := q.CreateDuelStake(ctx, dbgen.CreateDuelStakeParams{
		PlanID: duelPlan.ID, PlayerID: target.Players[0].ID, AssetID: asset.ID, HiddenDie: 4,
	})
	require.NoError(t, err)
	_, err = q.CreateDuelBout(ctx, dbgen.CreateDuelBoutParams{
		PlanID: duelPlan.ID, BoutNumber: 1,
		DeclarerID: target.Players[0].ID, DeclarerStakeID: stake.ID,
		ResponderID: target.Players[1].ID,
	})
	require.NoError(t, err)

	liaisePlan, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID: target.Game.ID, PlanType: model.PlanClandestinelyLiaise, Category: model.CategoryKnowledge,
		PreparerID: target.Players[0].ID, RowNumber: intPtr(1), PreparedAtRow: 1,
	})
	require.NoError(t, err)
	_, err = q.CreateLiaiseChoice(ctx, dbgen.CreateLiaiseChoiceParams{
		PlanID: liaisePlan.ID, PlayerID: target.Players[0].ID, Choice: "break_peer",
		TargetAssetID: &asset.ID,
	})
	require.NoError(t, err)

	spend, err := q.CreateShakeUpSpend(ctx, dbgen.CreateShakeUpSpendParams{
		GameID: target.Game.ID, PlayerID: target.Players[0].ID,
		Category: "power", OptionKey: "take_holding",
	})
	require.NoError(t, err)
	_, err = q.CreateShakeUpAdjustment(ctx, dbgen.CreateShakeUpAdjustmentParams{
		SpendID: spend.ID, PlayerID: target.Players[1].ID, Adjustment: 1,
	})
	require.NoError(t, err)

	require.Positive(t, countByID(t, pool, "marginalia", "asset_id", []int64{asset.ID}))
	require.Positive(t, countByID(t, pool, "secrets", "asset_id", []int64{asset.ID}))
	require.Positive(t, countByID(t, pool, "secret_visibility", "secret_id", []int64{secret.ID}))
	require.Positive(t, countByID(t, pool, "dice_roll_dice", "roll_id", []int64{roll.ID}))
	require.Positive(t, countByID(t, pool, "difficulty_votes", "roll_id", []int64{roll.ID}))
	require.Positive(t, countByID(t, pool, "duel_staked_assets", "plan_id", []int64{duelPlan.ID}))
	require.Positive(t, countByID(t, pool, "duel_bouts", "plan_id", []int64{duelPlan.ID}))
	require.Positive(t, countByID(t, pool, "liaise_choices", "plan_id", []int64{liaisePlan.ID}))
	require.Positive(t, countByID(t, pool, "shake_up_cost_adjustments", "spend_id", []int64{spend.ID}))

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

	// And every grandchild-only table (migration 041) is empty too.
	assert.Zero(t, countByID(t, pool, "marginalia", "asset_id", []int64{asset.ID}))
	assert.Zero(t, countByID(t, pool, "secrets", "asset_id", []int64{asset.ID}))
	assert.Zero(t, countByID(t, pool, "secret_visibility", "secret_id", []int64{secret.ID}))
	assert.Zero(t, countByID(t, pool, "dice_roll_dice", "roll_id", []int64{roll.ID}))
	assert.Zero(t, countByID(t, pool, "difficulty_votes", "roll_id", []int64{roll.ID}))
	assert.Zero(t, countByID(t, pool, "duel_staked_assets", "plan_id", []int64{duelPlan.ID}))
	assert.Zero(t, countByID(t, pool, "duel_bouts", "plan_id", []int64{duelPlan.ID}))
	assert.Zero(t, countByID(t, pool, "liaise_choices", "plan_id", []int64{liaisePlan.ID}))
	assert.Zero(t, countByID(t, pool, "shake_up_cost_adjustments", "spend_id", []int64{spend.ID}))

	// The control game is untouched.
	_, err = q.GetGameByID(ctx, control.Game.ID)
	require.NoError(t, err)
	assert.Positive(t, countByGame(t, pool, "players", control.Game.ID),
		"control game's players must survive deletion of another game")
	assert.Positive(t, countByGame(t, pool, "assets", control.Game.ID),
		"control game's assets must survive deletion of another game")
}

func intPtr(v int16) *int16 { return &v }

func TestDeleteGame_MissingIDDeletesNothing(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	// A never-created id deletes zero rows — the handler maps this to 404.
	rows, err := q.DeleteGame(context.Background(), 999_999_999)
	require.NoError(t, err)
	assert.EqualValues(t, 0, rows)
}
