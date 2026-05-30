//go:build integration

package handler

import (
	"context"
	"net/http"
	"strconv"
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

// TestSpreadRumors_BreakTarget_DestroysAssetOnFinalMarginalium guards the rule
// that tearing an asset's last intact marginalium destroys it ("all 4 gone →
// the asset is destroyed"). The break-target route previously inlined
// TearMarginalia and skipped the destroy check, so the final tear left the
// asset alive. It now routes through breakMarginalia, which destroys.
func TestSpreadRumors_BreakTarget_DestroysAssetOnFinalMarginalium(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	// Target asset owned by another player, carrying all 4 marginalia with the
	// first 3 already torn — so a single break-target tear is the final one.
	targetOwnerIdx := 1
	target := h.seedPeer(targetOwnerIdx, "rumor target")
	var lastM dbgen.Marginalium
	for pos := int16(1); pos <= 4; pos++ {
		m, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID: target, Position: pos, Text: "a damning note",
		})
		require.NoError(t, err)
		if pos < 4 {
			_, err = h.q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{ID: m.ID})
			require.NoError(t, err)
		} else {
			lastM = m
		}
	}

	notes := "Council betrayal rumor"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanSpreadRumors,
		TargetAssetID:    &target,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolve(plan.ID)
	require.NotNil(t, roll, "Spread Rumors creates its roll on resolve")
	h.forceRoll(roll.ID, makeOutcome, 3)
	h.makeChoice(plan.ID, makeOutcome, []string{"break_target"})

	// The preparer tears the final intact marginalium via break-target.
	preparerIdx := -1
	for i, p := range h.tg.Players {
		if p.ID == plan.PreparerID {
			preparerIdx = i
		}
	}
	require.GreaterOrEqual(t, preparerIdx, 0, "preparer must be one of the seeded players")

	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/break-target"
	code, body := h.post(preparerIdx, breakPath, map[string]any{"marginalia_id": lastM.ID})
	require.Equalf(t, http.StatusOK, code, "break-target: %v", body)

	torn, err := h.q.GetMarginaliaByID(ctx, lastM.ID)
	require.NoError(t, err)
	assert.True(t, torn.IsTorn, "the final marginalium should be torn")

	destroyed, err := h.q.GetAssetByID(ctx, target)
	require.NoError(t, err)
	assert.True(t, destroyed.IsDestroyed,
		"tearing the final marginalium via break-target must destroy the asset")
}
