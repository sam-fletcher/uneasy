//go:build integration

package handler

// handler/eligibility_integration_test.go — integration coverage for the
// DB-backed eligibility/ranking helpers (checkPlanEligible, playerHasPeers,
// hasEsteemLockout). Relocated from game/ alongside the functions themselves
// when they moved into the imperative shell; assertions are unchanged.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

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

// ─ checkPlanEligible Tests ─────────────────────────────────────────────────

func TestCheckPlanEligible_AlreadyHasToken(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	// Player 0 (rank 1 = highest) prepares Make Demands (Power category)
	makePlanWithToken(t, q, &tg.Game, &tg.Players[0],
		model.PlanMakeDemands, model.CategoryPower)

	// Player 0 tries to prepare another Make Demands plan → should be rejected
	eligible, msg, err := checkPlanEligible(ctx, q, tg.Game.ID, tg.Players[0].ID,
		model.PlanMakeDemands, model.CategoryPower)

	require.NoError(t, err)
	assert.False(t, eligible)
	assert.Contains(t, msg, "already have this plan prepared")
}

func TestCheckPlanEligible_HigherRankedPlayerHasToken(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	// Player 0 (rank 1 = highest) has token on Make Demands
	makePlanWithToken(t, q, &tg.Game, &tg.Players[0],
		model.PlanMakeDemands, model.CategoryPower)

	// Player 1 (rank 2) tries to prepare Make Demands → should be rejected
	// because a higher-ranked (lower rank number) player has the token
	eligible, msg, err := checkPlanEligible(ctx, q, tg.Game.ID, tg.Players[1].ID,
		model.PlanMakeDemands, model.CategoryPower)

	require.NoError(t, err)
	assert.False(t, eligible)
	assert.Contains(t, msg, "higher-ranked player")
}

func TestCheckPlanEligible_LowerRankedPlayerHasToken(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	// Player 2 (rank 3 = lowest) has token on Make Demands
	makePlanWithToken(t, q, &tg.Game, &tg.Players[2],
		model.PlanMakeDemands, model.CategoryPower)

	// Player 0 (rank 1 = highest) should be eligible because rank 3 is lower
	eligible, msg, err := checkPlanEligible(ctx, q, tg.Game.ID, tg.Players[0].ID,
		model.PlanMakeDemands, model.CategoryPower)

	require.NoError(t, err)
	assert.True(t, eligible, "Player 0 should be eligible despite Player 2 having token; msg: %s", msg)
}

// ─ playerHasPeers Tests ────────────────────────────────────────────────────

func TestPlayerHasPeers_NoPeers(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	// Player 0 has no peer assets initially
	has, err := playerHasPeers(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestPlayerHasPeers_WithPeers(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
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
	has, err := playerHasPeers(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestPlayerHasPeers_DestroyedPeersDoNotCount(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
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
	has, err := playerHasPeers(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestPlayerHasPeers_MultiplePeers(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
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

	has, err := playerHasPeers(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.True(t, has)
}

// ─ hasEsteemLockout Tests ──────────────────────────────────────────────────

func TestHasEsteemLockout_NoPrevPlans(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	// Player 0 has no plans prepared yet
	has, err := hasEsteemLockout(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasEsteemLockout_NonEsteemPlan(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
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
	has, err := hasEsteemLockout(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasEsteemLockout_EsteemPlanWithoutLockout(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
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
	has, err := hasEsteemLockout(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasEsteemLockout_ActiveLockout(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
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
	has, err := hasEsteemLockout(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestHasEsteemLockout_ClearedByNonEsteemPlan(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
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
	has, err := hasEsteemLockout(ctx, q, tg.Game.ID, tg.Players[0].ID)
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
	has, err = hasEsteemLockout(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasEsteemLockout_MultipleEsteemPlans(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
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
	has, err := hasEsteemLockout(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.True(t, has)
}
