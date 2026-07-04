//go:build integration

package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/model"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

// createTestPeer is a small helper for scene tests — most tests need a peer
// asset belonging to a specific player and don't care about marginalia.
func createTestPeer(
	t *testing.T,
	q *dbgen.Queries,
	gameID, ownerID int64,
	name string,
	isMain bool,
) dbgen.Asset {
	t.Helper()
	a, err := q.CreateAsset(context.Background(), dbgen.CreateAssetParams{
		GameID:          gameID,
		OwnerID:         ownerID,
		CreatorID:       ownerID,
		AssetType:       model.AssetPeer,
		Name:            name,
		IsMainCharacter: isMain,
	})
	require.NoError(t, err)
	return a
}

func createTestHolding(
	t *testing.T,
	q *dbgen.Queries,
	gameID, ownerID int64,
	name string,
) dbgen.Asset {
	t.Helper()
	a, err := q.CreateAsset(context.Background(), dbgen.CreateAssetParams{
		GameID:    gameID,
		OwnerID:   ownerID,
		CreatorID: ownerID,
		AssetType: model.AssetHolding,
		Name:      name,
	})
	require.NoError(t, err)
	return a
}

// ── Scene CRUD ───────────────────────────────────────────────────────────────

func TestCreateScene_HoldingLocation(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	holding := createTestHolding(t, q, tg.Game.ID, tg.Players[0].ID, "Throne Room")

	scene, err := q.CreateScene(ctx, dbgen.CreateSceneParams{
		GameID:            tg.Game.ID,
		RowNumber:         tg.Game.CurrentRow,
		FocusPlayerID:     tg.Players[0].ID,
		LocationHoldingID: &holding.ID,
		LocationCustom:    nil,
		TimeElapsed:       new(model.TimeHours),
		TimeNote:          nil,
		Prompt:            "",
		ResolvedPlanID:    nil,
	})
	require.NoError(t, err)
	assert.Equal(t, tg.Game.ID, scene.GameID)
	require.NotNil(t, scene.LocationHoldingID)
	assert.Equal(t, holding.ID, *scene.LocationHoldingID)
	assert.Nil(t, scene.LocationCustom)
	require.NotNil(t, scene.TimeElapsed)
	assert.Equal(t, model.TimeHours, *scene.TimeElapsed)
	assert.False(t, scene.EndedAt.Valid)
}

func TestCreateScene_CustomLocation(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	custom := "On the road to nowhere"
	scene, err := q.CreateScene(ctx, dbgen.CreateSceneParams{
		GameID:         tg.Game.ID,
		RowNumber:      tg.Game.CurrentRow,
		FocusPlayerID:  tg.Players[0].ID,
		LocationCustom: &custom,
		TimeElapsed:    new(model.TimeMoments),
	})
	require.NoError(t, err)
	require.NotNil(t, scene.LocationCustom)
	assert.Equal(t, custom, *scene.LocationCustom)
	assert.Nil(t, scene.LocationHoldingID)
}

func TestCreateScene_LocationXorEnforced(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	// Both set → violates CHECK.
	holding := createTestHolding(t, q, tg.Game.ID, tg.Players[0].ID, "Keep")
	custom := "Keep at dawn"
	_, err := q.CreateScene(ctx, dbgen.CreateSceneParams{
		GameID:            tg.Game.ID,
		RowNumber:         tg.Game.CurrentRow,
		FocusPlayerID:     tg.Players[0].ID,
		LocationHoldingID: &holding.ID,
		LocationCustom:    &custom,
		TimeElapsed:       new(model.TimeHours),
	})
	require.Error(t, err, "CHECK should reject both location_holding_id and location_custom set")

	// Neither set → also violates CHECK.
	_, err = q.CreateScene(ctx, dbgen.CreateSceneParams{
		GameID:        tg.Game.ID,
		RowNumber:     tg.Game.CurrentRow,
		FocusPlayerID: tg.Players[0].ID,
		TimeElapsed:   new(model.TimeHours),
	})
	require.Error(t, err, "CHECK should reject neither location set")
}

func TestCreateScene_OnlyOneActivePerGame(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	custom := "Start"
	first, err := q.CreateScene(ctx, dbgen.CreateSceneParams{
		GameID:         tg.Game.ID,
		RowNumber:      tg.Game.CurrentRow,
		FocusPlayerID:  tg.Players[0].ID,
		LocationCustom: &custom,
		TimeElapsed:    new(model.TimeMoments),
	})
	require.NoError(t, err)

	// Second active scene → violates the partial unique index.
	custom2 := "Second"
	_, err = q.CreateScene(ctx, dbgen.CreateSceneParams{
		GameID:         tg.Game.ID,
		RowNumber:      tg.Game.CurrentRow,
		FocusPlayerID:  tg.Players[0].ID,
		LocationCustom: &custom2,
		TimeElapsed:    new(model.TimeMoments),
	})
	require.Error(t, err, "expected unique index violation for second active scene")

	// End the first; a new active scene is now allowed.
	require.NoError(t, q.EndScene(ctx, first.ID))
	_, err = q.CreateScene(ctx, dbgen.CreateSceneParams{
		GameID:         tg.Game.ID,
		RowNumber:      tg.Game.CurrentRow,
		FocusPlayerID:  tg.Players[0].ID,
		LocationCustom: &custom2,
		TimeElapsed:    new(model.TimeMoments),
	})
	require.NoError(t, err, "second scene should be permitted once the first has ended")
}

func TestGetActiveScene_NoneReturnsError(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	_, err := q.GetActiveScene(ctx, tg.Game.ID)
	require.Error(t, err, "expected pgx no-rows error when no active scene")
}

// ── Scene peers + claim ──────────────────────────────────────────────────────

func TestClaimScenePeer_Atomic(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	custom := "Salon"
	scene, err := q.CreateScene(ctx, dbgen.CreateSceneParams{
		GameID:         tg.Game.ID,
		RowNumber:      tg.Game.CurrentRow,
		FocusPlayerID:  tg.Players[0].ID,
		LocationCustom: &custom,
		TimeElapsed:    new(model.TimeMoments),
	})
	require.NoError(t, err)

	// A peer owned by the focus player, NOT their main character → claimable.
	peer := createTestPeer(t, q, tg.Game.ID, tg.Players[0].ID, "Sir Kestrel", false)
	require.NoError(t, q.InsertScenePeer(ctx, dbgen.InsertScenePeerParams{
		SceneID:            scene.ID,
		PeerAssetID:        peer.ID,
		ControllerPlayerID: nil,
	}))

	// First non-focus claim succeeds.
	n, err := q.ClaimScenePeer(ctx, dbgen.ClaimScenePeerParams{
		SceneID:            scene.ID,
		PeerAssetID:        peer.ID,
		ControllerPlayerID: &tg.Players[1].ID,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	// Second claim by anyone else is a no-op (0 rows updated) — locked.
	n, err = q.ClaimScenePeer(ctx, dbgen.ClaimScenePeerParams{
		SceneID:            scene.ID,
		PeerAssetID:        peer.ID,
		ControllerPlayerID: &tg.Players[2].ID,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), n, "claim must not overwrite an existing controller")

	// Verify final controller is the first claimant.
	got, err := q.GetScenePeer(ctx, dbgen.GetScenePeerParams{
		SceneID:     scene.ID,
		PeerAssetID: peer.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, got.ControllerPlayerID)
	assert.Equal(t, tg.Players[1].ID, *got.ControllerPlayerID)
}

// ── Most-recent resolved plan lookup ─────────────────────────────────────────

func TestGetMostRecentResolvedPlanOnRow(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	row := tg.Game.CurrentRow

	// One pending and one resolved plan on this row.
	pending := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanMakeDemands, model.CategoryPower, row)
	resolved := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanProposeDecree, model.CategoryEsteem, row)

	// Mark `resolved` as resolved.
	require.NoError(t, q.SetPlanResult(ctx, dbgen.SetPlanResultParams{
		ID: resolved.ID, Result: ptrStr("make"),
	}))

	got, err := q.GetMostRecentResolvedPlanOnRow(ctx, dbgen.GetMostRecentResolvedPlanOnRowParams{
		GameID:    tg.Game.ID,
		RowNumber: &row,
	})
	require.NoError(t, err)
	assert.Equal(t, resolved.ID, got.ID, "expected resolved plan, not pending plan %d", pending.ID)
	assert.Equal(t, model.PlanProposeDecree, got.PlanType)

	// Sanity: the prompt for that plan type is non-empty.
	assert.NotEmpty(t, game.FollowOnPrompt(got.PlanType))
}

// ptrStr is a tiny *string helper used by SetPlanResult — sqlc emits *string
// for nullable text columns in this codebase.
func ptrStr(s string) *string { return &s }
