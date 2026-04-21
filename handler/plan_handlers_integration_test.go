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

// ── PHASE 2: Plan Handler Integration Tests ─────────────────────────────────
// These tests validate the core business logic of plan handlers by calling
// their ValidatePreparation methods with various inputs.

// ── Make War Tests ───────────────────────────────────────────────────────────

func TestMakeWar_RejectsNoEnemies(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	vc := &game.ValidationContext{
		Q:              q,
		Game:           &tg.Game,
		Player:         &tg.Players[0],
		EnemyPlayerIDs: []int64{},
	}
	_, errMsg := mwHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
	assert.Contains(t, errMsg, "requires at least one")
}

func TestMakeWar_RejectedDuplicateEnemies(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	vc := &game.ValidationContext{
		Q:              q,
		Game:           &tg.Game,
		Player:         &tg.Players[0],
		EnemyPlayerIDs: []int64{tg.Players[1].ID, tg.Players[1].ID},
	}
	_, errMsg := mwHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
	assert.Contains(t, errMsg, "duplicates")
}

func TestMakeWar_AcceptsValidEnemies(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 4)
	ctx := context.Background()

	vc := &game.ValidationContext{
		Q:              q,
		Game:           &tg.Game,
		Player:         &tg.Players[0],
		EnemyPlayerIDs: []int64{tg.Players[1].ID, tg.Players[2].ID},
	}
	_, errMsg := mwHandler{}.ValidatePreparation(ctx, vc)
	assert.Empty(t, errMsg)
}

// ── Propose Duel Tests ───────────────────────────────────────────────────────

func TestProposeDuel_RejectsNoOpponent(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	vc := &game.ValidationContext{
		Q:      q,
		Game:   &tg.Game,
		Player: &tg.Players[0],
	}
	_, errMsg := pduelHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
}

func TestProposeDuel_RejectsSelfAsOpponent(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	opponentID := tg.Players[0].ID
	notes := "Courtyard duel"
	vc := &game.ValidationContext{
		Q:              q,
		Game:           &tg.Game,
		Player:         &tg.Players[0],
		TargetPlayerID: &opponentID,
		Notes:          notes,
	}
	_, errMsg := pduelHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
	assert.Contains(t, errMsg, "yourself")
}

func TestProposeDuel_RejectsNoNotes(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	opponentID := tg.Players[1].ID
	vc := &game.ValidationContext{
		Q:              q,
		Game:           &tg.Game,
		Player:         &tg.Players[0],
		TargetPlayerID: &opponentID,
	}
	_, errMsg := pduelHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
}

func TestProposeDuel_AcceptsValidDuel(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	opponentID := tg.Players[1].ID
	notes := "Courtyard duel at dawn"
	vc := &game.ValidationContext{
		Q:              q,
		Game:           &tg.Game,
		Player:         &tg.Players[0],
		TargetPlayerID: &opponentID,
		Notes:          notes,
	}
	_, errMsg := pduelHandler{}.ValidatePreparation(ctx, vc)
	assert.Empty(t, errMsg)
}

// ── Seek Answers Tests ───────────────────────────────────────────────────────

func TestSeekAnswers_RejectsNoNotes(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	vc := &game.ValidationContext{
		Q:      q,
		Game:   &tg.Game,
		Player: &tg.Players[0],
	}
	_, errMsg := saHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
}

func TestSeekAnswers_AcceptsWithNotes(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	notes := "Research tower origins in archives"
	vc := &game.ValidationContext{
		Q:      q,
		Game:   &tg.Game,
		Player: &tg.Players[0],
		Notes:  notes,
	}
	_, errMsg := saHandler{}.ValidatePreparation(ctx, vc)
	assert.Empty(t, errMsg)
}

// ── Spread Rumors Tests ──────────────────────────────────────────────────────

func TestSpreadRumors_RejectsNoTarget(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	vc := &game.ValidationContext{
		Q:      q,
		Game:   &tg.Game,
		Player: &tg.Players[0],
	}
	_, errMsg := srHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
}

func TestSpreadRumors_RejectsNoNotes(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	// Create target asset
	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	vc := &game.ValidationContext{
		Q:             q,
		Game:          &tg.Game,
		Player:        &tg.Players[0],
		TargetAssetID: &asset.ID,
	}
	_, errMsg := srHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
}

func TestSpreadRumors_AcceptsWithTargetAndNotes(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	// Create target asset
	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	notes := "Council betrayal rumor"
	vc := &game.ValidationContext{
		Q:             q,
		Game:          &tg.Game,
		Player:        &tg.Players[0],
		TargetAssetID: &asset.ID,
		Notes:         notes,
	}
	_, errMsg := srHandler{}.ValidatePreparation(ctx, vc)
	assert.Empty(t, errMsg)
}
