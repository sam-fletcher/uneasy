//go:build integration

package game

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// ─ Test Database Setup ─────────────────────────────────────────────────────

const testDBURLEnv = "TEST_DATABASE_URL"

var (
	gameTestHarnessOnce sync.Once
	gameTestHarnessPool *pgxpool.Pool
	gameTestHarnessErr  error
)

// openGameTestDB returns a pool pointing at the test database, applying
// migrations exactly once per `go test` invocation. Skips the calling
// test if TEST_DATABASE_URL is unset or the DB is unreachable.
func openGameTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv(testDBURLEnv)
	if url == "" {
		t.Skipf("set %s to run integration tests", testDBURLEnv)
	}
	gameTestHarnessOnce.Do(func() {
		pool, err := pgxpool.New(context.Background(), url)
		if err != nil {
			gameTestHarnessErr = fmt.Errorf("connect: %w", err)
			return
		}
		if err := pool.Ping(context.Background()); err != nil {
			gameTestHarnessErr = fmt.Errorf("ping: %w", err)
			return
		}
		if err := db.RunMigrations(url); err != nil {
			gameTestHarnessErr = fmt.Errorf("migrate: %w", err)
			return
		}
		gameTestHarnessPool = pool
	})
	if gameTestHarnessErr != nil {
		t.Skipf("test DB unavailable: %v", gameTestHarnessErr)
	}
	truncateGameTestTables(t, gameTestHarnessPool)
	return gameTestHarnessPool
}

// truncateGameTestTables wipes every user table in the public schema between tests.
func truncateGameTestTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	rows, err := pool.Query(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_type = 'BASE TABLE'
		  AND table_name <> 'schema_migrations'
	`)
	require.NoError(t, err)
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		err := rows.Scan(&name)
		require.NoError(t, err)
		tables = append(tables, `"`+name+`"`)
	}
	if len(tables) == 0 {
		return
	}

	stmt := "TRUNCATE " + joinCommaGame(tables) + " RESTART IDENTITY CASCADE"
	_, err = pool.Exec(ctx, stmt)
	require.NoError(t, err)
}

func joinCommaGame(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

// gameTestGame mirrors the structure used in handler tests
type gameTestGame struct {
	Game    dbgen.Game
	Players []dbgen.Player
}

// newGameTestGame creates a test game with n players
func newGameTestGame(t *testing.T, q *dbgen.Queries, n int) gameTestGame {
	t.Helper()
	require.GreaterOrEqual(t, n, 2)
	require.LessOrEqual(t, n, 5)
	ctx := context.Background()

	game, err := q.CreateGame(ctx, "TEST"+gameRandSuffix())
	require.NoError(t, err)

	players := make([]dbgen.Player, n)
	for i := 0; i < n; i++ {
		acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
			Username: fmt.Sprintf("p%d-%s", i+1, gameRandSuffix()),
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
		players[i] = p
	}

	require.NoError(t, q.SetFacilitator(ctx, dbgen.SetFacilitatorParams{
		FacilitatorID: &players[0].ID, ID: game.ID,
	}))

	// Power rankings: player[i] gets rank i+1 (1 = highest).
	// Also seed knowledge and esteem so anything that reads any track works.
	for _, cat := range []model.RankingCategory{
		model.CategoryPower, model.CategoryKnowledge, model.CategoryEsteem,
	} {
		for i := 0; i < n; i++ {
			require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
				GameID:   game.ID,
				PlayerID: &players[i].ID,
				Category: cat,
				Rank:     int16(i + 1),
			}))
		}
	}

	require.NoError(t, q.CreatePublicRecordRows(ctx, game.ID))
	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: game.ID, Phase: model.PhaseMainEvent,
	}))
	require.NoError(t, q.SetCurrentRow(ctx, dbgen.SetCurrentRowParams{
		ID: game.ID, CurrentRow: 1,
	}))
	require.NoError(t, q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{
		ID: game.ID, FocusPlayerID: &players[0].ID,
	}))

	refreshed, err := q.GetGameByID(ctx, game.ID)
	require.NoError(t, err)

	return gameTestGame{Game: refreshed, Players: players}
}

func gameRandSuffix() string {
	s, err := db.NewCookieToken()
	if err != nil {
		return "xxxxx"
	}
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// makePlanWithToken creates a Plan plus a matching plan_token in one go.
// Together these represent "this player has prepared a plan of this
// type/category" — the eligibility checks read the tokens on the shield,
// but plan_tokens.plan_id is NOT NULL so we can't insert a token without
// a real plan to point at.
func makePlanWithToken(
	t *testing.T,
	q *dbgen.Queries,
	game *dbgen.Game,
	preparer *dbgen.Player,
	planType model.PlanType,
	category model.RankingCategory,
) {
	t.Helper()
	ctx := context.Background()
	plan, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:        game.ID,
		PlanType:      planType,
		Category:      category,
		PreparerID:    preparer.ID,
		RowNumber:     &game.CurrentRow,
		RowOrder:      0,
		PreparedAtRow: game.CurrentRow,
	})
	require.NoError(t, err)
	_, err = q.CreatePlanToken(ctx, dbgen.CreatePlanTokenParams{
		GameID:   game.ID,
		PlanType: planType,
		PlayerID: preparer.ID,
		PlanID:   plan.ID,
	})
	require.NoError(t, err)
}

// ─ CheckPlanEligible Tests ─────────────────────────────────────────────────

func TestCheckPlanEligible_AlreadyHasToken(t *testing.T) {
	pool := openGameTestDB(t)
	q := dbgen.New(pool)
	tg := newGameTestGame(t, q, 3)
	ctx := context.Background()

	// Player 0 (rank 1 = highest) prepares Make Demands (Power category)
	makePlanWithToken(t, q, &tg.Game, &tg.Players[0],
		model.PlanMakeDemands, model.CategoryPower)

	// Player 0 tries to prepare another Make Demands plan → should be rejected
	eligible, msg, err := CheckPlanEligible(ctx, q, tg.Game.ID, tg.Players[0].ID,
		model.PlanMakeDemands, model.CategoryPower)

	require.NoError(t, err)
	assert.False(t, eligible)
	assert.Contains(t, msg, "already have this plan prepared")
}

func TestCheckPlanEligible_HigherRankedPlayerHasToken(t *testing.T) {
	pool := openGameTestDB(t)
	q := dbgen.New(pool)
	tg := newGameTestGame(t, q, 3)
	ctx := context.Background()

	// Player 0 (rank 1 = highest) has token on Make Demands
	makePlanWithToken(t, q, &tg.Game, &tg.Players[0],
		model.PlanMakeDemands, model.CategoryPower)

	// Player 1 (rank 2) tries to prepare Make Demands → should be rejected
	// because a higher-ranked (lower rank number) player has the token
	eligible, msg, err := CheckPlanEligible(ctx, q, tg.Game.ID, tg.Players[1].ID,
		model.PlanMakeDemands, model.CategoryPower)

	require.NoError(t, err)
	assert.False(t, eligible)
	assert.Contains(t, msg, "higher-ranked player")
}

func TestCheckPlanEligible_LowerRankedPlayerHasToken(t *testing.T) {
	pool := openGameTestDB(t)
	q := dbgen.New(pool)
	tg := newGameTestGame(t, q, 3)
	ctx := context.Background()

	// Player 2 (rank 3 = lowest) has token on Make Demands
	makePlanWithToken(t, q, &tg.Game, &tg.Players[2],
		model.PlanMakeDemands, model.CategoryPower)

	// Player 0 (rank 1 = highest) should be eligible because rank 3 is lower
	eligible, msg, err := CheckPlanEligible(ctx, q, tg.Game.ID, tg.Players[0].ID,
		model.PlanMakeDemands, model.CategoryPower)

	require.NoError(t, err)
	assert.True(t, eligible, "Player 0 should be eligible despite Player 2 having token; msg: %s", msg)
}

// ─ PlayerHasPeers Tests ────────────────────────────────────────────────────

func TestPlayerHasPeers_NoPeers(t *testing.T) {
	pool := openGameTestDB(t)
	q := dbgen.New(pool)
	tg := newGameTestGame(t, q, 2)
	ctx := context.Background()

	// Player 0 has no peer assets initially
	has, err := PlayerHasPeers(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestPlayerHasPeers_WithPeers(t *testing.T) {
	pool := openGameTestDB(t)
	q := dbgen.New(pool)
	tg := newGameTestGame(t, q, 2)
	ctx := context.Background()

	// Player 0 creates a peer asset
	_, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	// Now player 0 should have peers
	has, err := PlayerHasPeers(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestPlayerHasPeers_DestroyedPeersDoNotCount(t *testing.T) {
	pool := openGameTestDB(t)
	q := dbgen.New(pool)
	tg := newGameTestGame(t, q, 2)
	ctx := context.Background()

	// Player 0 creates a peer asset
	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "DeadAlly",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	// Then destroy it
	err = q.DestroyAsset(ctx, asset.ID)
	require.NoError(t, err)

	// Destroyed peers should not count
	has, err := PlayerHasPeers(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestPlayerHasPeers_MultiplePeers(t *testing.T) {
	pool := openGameTestDB(t)
	q := dbgen.New(pool)
	tg := newGameTestGame(t, q, 2)
	ctx := context.Background()

	// Player 0 creates multiple peers
	for i := 0; i < 3; i++ {
		_, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID:          tg.Game.ID,
			OwnerID:         tg.Players[0].ID,
			CreatorID:       tg.Players[0].ID,
			AssetType:       model.AssetPeer,
			Name:            fmt.Sprintf("Ally%d", i),
			IsMainCharacter: false,
		})
		require.NoError(t, err)
	}

	has, err := PlayerHasPeers(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.True(t, has)
}

// ─ HasEsteemLockout Tests ──────────────────────────────────────────────────

func TestHasEsteemLockout_NoPrevPlans(t *testing.T) {
	pool := openGameTestDB(t)
	q := dbgen.New(pool)
	tg := newGameTestGame(t, q, 2)
	ctx := context.Background()

	// Player 0 has no plans prepared yet
	has, err := HasEsteemLockout(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasEsteemLockout_NonEsteemPlan(t *testing.T) {
	pool := openGameTestDB(t)
	q := dbgen.New(pool)
	tg := newGameTestGame(t, q, 2)
	ctx := context.Background()

	// Player 0 prepares a Power plan (Make Demands)
	_, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:        tg.Game.ID,
		PlanType:      model.PlanMakeDemands,
		Category:      model.CategoryPower,
		PreparerID:    tg.Players[0].ID,
		RowNumber:     new(int16(1)),
		RowOrder:      0,
		PreparedAtRow: tg.Game.CurrentRow,
	})
	require.NoError(t, err)

	// No lockout should be active
	has, err := HasEsteemLockout(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasEsteemLockout_EsteemPlanWithoutLockout(t *testing.T) {
	pool := openGameTestDB(t)
	q := dbgen.New(pool)
	tg := newGameTestGame(t, q, 2)
	ctx := context.Background()

	// Player 0 prepares a Spread Propaganda plan (Esteem category)
	// with no custom resolution data (default behavior)
	_, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:        tg.Game.ID,
		PlanType:      model.PlanSpreadPropaganda,
		Category:      model.CategoryEsteem,
		PreparerID:    tg.Players[0].ID,
		RowNumber:     new(int16(1)),
		RowOrder:      0,
		PreparedAtRow: tg.Game.CurrentRow,
	})
	require.NoError(t, err)

	// No lockout
	has, err := HasEsteemLockout(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasEsteemLockout_ActiveLockout(t *testing.T) {
	pool := openGameTestDB(t)
	q := dbgen.New(pool)
	tg := newGameTestGame(t, q, 2)
	ctx := context.Background()

	// Player 0 prepares Spread Propaganda
	plan, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:        tg.Game.ID,
		PlanType:      model.PlanSpreadPropaganda,
		Category:      model.CategoryEsteem,
		PreparerID:    tg.Players[0].ID,
		RowNumber:     new(int16(1)),
		RowOrder:      0,
		PreparedAtRow: tg.Game.CurrentRow,
	})
	require.NoError(t, err)

	// Set resolution data with EsteemLockout = true
	resData := map[string]interface{}{"spread_propaganda": map[string]interface{}{"esteem_lockout": true}}
	resDataBytes, _ := json.Marshal(resData)
	resDataStr := string(resDataBytes)
	err = q.SetPlanResolutionData(ctx, dbgen.SetPlanResolutionDataParams{
		ID:             plan.ID,
		ResolutionData: &resDataStr,
	})
	require.NoError(t, err)

	// Lockout should be active
	has, err := HasEsteemLockout(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestHasEsteemLockout_ClearedByNonEsteemPlan(t *testing.T) {
	pool := openGameTestDB(t)
	q := dbgen.New(pool)
	tg := newGameTestGame(t, q, 2)
	ctx := context.Background()

	// Player 0 prepares Spread Propaganda with EsteemLockout = true
	plan1, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:        tg.Game.ID,
		PlanType:      model.PlanSpreadPropaganda,
		Category:      model.CategoryEsteem,
		PreparerID:    tg.Players[0].ID,
		RowNumber:     new(int16(1)),
		RowOrder:      0,
		PreparedAtRow: tg.Game.CurrentRow,
	})
	require.NoError(t, err)

	resData1 := map[string]interface{}{"spread_propaganda": map[string]interface{}{"esteem_lockout": true}}
	resDataBytes1, _ := json.Marshal(resData1)
	resDataStr1 := string(resDataBytes1)
	err = q.SetPlanResolutionData(ctx, dbgen.SetPlanResolutionDataParams{
		ID:             plan1.ID,
		ResolutionData: &resDataStr1,
	})
	require.NoError(t, err)

	// Lockout is active
	has, err := HasEsteemLockout(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.True(t, has)

	// Player 0 then prepares a non-esteem plan (Power)
	_, err = q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:        tg.Game.ID,
		PlanType:      model.PlanMakeDemands,
		Category:      model.CategoryPower,
		PreparerID:    tg.Players[0].ID,
		RowNumber:     new(int16(2)),
		RowOrder:      0,
		PreparedAtRow: tg.Game.CurrentRow,
	})
	require.NoError(t, err)

	// Now the lockout should be cleared
	has, err = HasEsteemLockout(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasEsteemLockout_MultipleEsteemPlans(t *testing.T) {
	pool := openGameTestDB(t)
	q := dbgen.New(pool)
	tg := newGameTestGame(t, q, 2)
	ctx := context.Background()

	// Player 0 prepares two Spread Propaganda plans with lockout
	for i := 0; i < 2; i++ {
		plan, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
			GameID:        tg.Game.ID,
			PlanType:      model.PlanSpreadPropaganda,
			Category:      model.CategoryEsteem,
			PreparerID:    tg.Players[0].ID,
			RowNumber:     new(int16(i + 1)),
			RowOrder:      0,
			PreparedAtRow: tg.Game.CurrentRow,
		})
		require.NoError(t, err)

		resData := map[string]interface{}{"spread_propaganda": map[string]interface{}{"esteem_lockout": true}}
		resDataBytes, _ := json.Marshal(resData)
		resDataStr := string(resDataBytes)
		err = q.SetPlanResolutionData(ctx, dbgen.SetPlanResolutionDataParams{
			ID:             plan.ID,
			ResolutionData: &resDataStr,
		})
		require.NoError(t, err)
	}

	// Lockout should be active from the most recent plan
	has, err := HasEsteemLockout(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.True(t, has)
}
